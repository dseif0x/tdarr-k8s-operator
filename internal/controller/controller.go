// Package controller contains the reconcile loop that scales a single Tdarr
// transcode node Job up and down based on the server's work queue.
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/dseif0x/tdarr-k8s-operator/internal/config"
	"github.com/dseif0x/tdarr-k8s-operator/internal/tdarr"
)

// Controller scales the on-demand transcode node.
type Controller struct {
	cfg      *config.Config
	k8s      kubernetes.Interface
	tdarr    *tdarr.Client
	log      *slog.Logger
	tmpl     []byte
	lastBusy time.Time
}

// New constructs a Controller. It eagerly loads and validates the node Job
// template so misconfiguration fails fast at startup.
func New(cfg *config.Config, k8s kubernetes.Interface, log *slog.Logger) (*Controller, error) {
	tmpl, err := os.ReadFile(cfg.NodeJobTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("read node job template %q: %w", cfg.NodeJobTemplatePath, err)
	}
	if _, err := decodeJob(tmpl, cfg.NodeJobName); err != nil {
		return nil, fmt.Errorf("invalid node job template: %w", err)
	}
	return &Controller{
		cfg:      cfg,
		k8s:      k8s,
		tdarr:    tdarr.New(cfg.ServerURL),
		log:      log,
		tmpl:     tmpl,
		lastBusy: time.Now(),
	}, nil
}

// Run executes the reconcile loop until the context is cancelled.
func (c *Controller) Run(ctx context.Context) error {
	c.log.Info("starting reconcile loop",
		"server", c.cfg.ServerURL,
		"pollInterval", c.cfg.PollInterval,
		"idleTimeout", c.cfg.IdleTimeout,
		"namespace", c.cfg.Namespace,
		"nodeJob", c.cfg.NodeJobName,
	)
	ticker := time.NewTicker(c.cfg.PollInterval)
	defer ticker.Stop()

	// Reconcile once immediately so we react without waiting a full interval.
	c.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			c.log.Info("shutting down reconcile loop")
			return nil
		case <-ticker.C:
			c.reconcile(ctx)
		}
	}
}

func (c *Controller) reconcile(ctx context.Context) {
	status, err := c.tdarr.Status(ctx, c.cfg.TranscodeQueueField, c.cfg.HealthCheckQueueField)
	if err != nil {
		// When the server is unreachable we make no changes: scaling up would
		// be pointless and scaling down risks killing an in-flight transcode.
		c.log.Warn("could not query tdarr server; skipping this cycle", "error", err)
		return
	}

	jobExists, err := c.nodeJobExists(ctx)
	if err != nil {
		c.log.Error("could not check node job", "error", err)
		return
	}

	c.log.Debug("queue status",
		"transcodeQueue", status.TranscodeQueue,
		"healthCheckQueue", status.HealthCheckQueue,
		"activeWorkers", status.ActiveWorkers,
		"pending", status.Pending(),
		"nodeJobExists", jobExists,
	)

	if status.Pending() {
		c.lastBusy = time.Now()
		if !jobExists {
			if err := c.createNodeJob(ctx); err != nil {
				c.log.Error("failed to create node job", "error", err)
				return
			}
			c.log.Info("scaled up: created transcode node job",
				"name", c.cfg.NodeJobName,
				"transcodeQueue", status.TranscodeQueue,
				"healthCheckQueue", status.HealthCheckQueue,
			)
		}
		return
	}

	// Nothing pending. Tear the node down only after it has been idle long
	// enough to avoid thrashing between consecutive files.
	if jobExists {
		idleFor := time.Since(c.lastBusy)
		if idleFor < c.cfg.IdleTimeout {
			c.log.Debug("idle but within timeout; keeping node", "idleFor", idleFor.Round(time.Second))
			return
		}
		if err := c.deleteNodeJob(ctx); err != nil {
			c.log.Error("failed to delete node job", "error", err)
			return
		}
		c.log.Info("scaled down: deleted idle transcode node job",
			"name", c.cfg.NodeJobName,
			"idleFor", idleFor.Round(time.Second),
		)
	}
}

func (c *Controller) nodeJobExists(ctx context.Context) (bool, error) {
	_, err := c.k8s.BatchV1().Jobs(c.cfg.Namespace).Get(ctx, c.cfg.NodeJobName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Controller) createNodeJob(ctx context.Context) error {
	job, err := decodeJob(c.tmpl, c.cfg.NodeJobName)
	if err != nil {
		return err
	}
	job.Namespace = c.cfg.Namespace
	_, err = c.k8s.BatchV1().Jobs(c.cfg.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (c *Controller) deleteNodeJob(ctx context.Context) error {
	// Background propagation ensures the Job's pods are garbage collected too.
	policy := metav1.DeletePropagationBackground
	err := c.k8s.BatchV1().Jobs(c.cfg.Namespace).Delete(ctx, c.cfg.NodeJobName, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// decodeJob parses the YAML template into a typed Job and forces the name so
// the controller always manages a single, predictable resource.
func decodeJob(tmpl []byte, name string) (*batchv1.Job, error) {
	var job batchv1.Job
	if err := yaml.Unmarshal(tmpl, &job); err != nil {
		return nil, err
	}
	if job.Kind != "" && job.Kind != "Job" {
		return nil, fmt.Errorf("template kind is %q, expected Job", job.Kind)
	}
	job.Name = name
	job.GenerateName = ""
	return &job, nil
}
