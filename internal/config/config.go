// Package config loads the tdarr-operator controller configuration from the
// environment. All values are injected by the Helm chart.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the runtime configuration for the controller.
type Config struct {
	// ServerURL is the base URL of the Tdarr server web/API port (8265),
	// e.g. http://my-release-tdarr:8265.
	ServerURL string

	// Namespace is the namespace the controller operates in (where node
	// Jobs are created/deleted).
	Namespace string

	// NodeJobTemplatePath points at the rendered node Job manifest that the
	// controller applies when transcode work appears.
	NodeJobTemplatePath string

	// NodeJobName is the fixed name used for the on-demand node Job. Because
	// we scale a single node on demand we only ever create one Job with this
	// name at a time.
	NodeJobName string

	// PollInterval is how often the controller polls the Tdarr server.
	PollInterval time.Duration

	// IdleTimeout is how long the queue and all workers must remain idle
	// before the node Job is torn down. This avoids thrashing the node
	// between back-to-back files.
	IdleTimeout time.Duration

	// TranscodeQueueField / HealthCheckQueueField are the field names in the
	// Tdarr statistics document that hold the pending work counts. They are
	// configurable so the operator can adapt to Tdarr API changes without a
	// rebuild.
	TranscodeQueueField   string
	HealthCheckQueueField string
}

// Load reads the configuration from environment variables, applying defaults
// where appropriate, and validates required fields.
func Load() (*Config, error) {
	c := &Config{
		ServerURL:             os.Getenv("TDARR_SERVER_URL"),
		Namespace:             os.Getenv("NAMESPACE"),
		NodeJobTemplatePath:   getEnv("NODE_JOB_TEMPLATE_PATH", "/etc/tdarr-operator/node-job.yaml"),
		NodeJobName:           getEnv("NODE_JOB_NAME", "tdarr-node"),
		TranscodeQueueField:   getEnv("TRANSCODE_QUEUE_FIELD", "table1Count"),
		HealthCheckQueueField: getEnv("HEALTHCHECK_QUEUE_FIELD", "table4Count"),
	}

	var err error
	if c.PollInterval, err = getDuration("POLL_INTERVAL", 15*time.Second); err != nil {
		return nil, err
	}
	if c.IdleTimeout, err = getDuration("IDLE_TIMEOUT", 120*time.Second); err != nil {
		return nil, err
	}

	if c.ServerURL == "" {
		return nil, fmt.Errorf("TDARR_SERVER_URL is required")
	}
	if c.Namespace == "" {
		return nil, fmt.Errorf("NAMESPACE is required")
	}
	return c, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	// Allow a bare number of seconds for convenience.
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", key, err)
	}
	return d, nil
}
