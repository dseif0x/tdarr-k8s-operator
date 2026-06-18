// Command tdarr-operator runs a lightweight controller that deploys alongside
// a Tdarr server and provisions an on-demand transcode node Job whenever the
// server has work to do, removing it again once the work is finished.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/dseif0x/tdarr-k8s-operator/internal/config"
	"github.com/dseif0x/tdarr-k8s-operator/internal/controller"
)

func main() {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	restCfg, err := rest.InClusterConfig()
	if err != nil {
		log.Error("not running in cluster / cannot load in-cluster config", "error", err)
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		log.Error("failed to build kubernetes client", "error", err)
		os.Exit(1)
	}

	ctrl, err := controller.New(cfg, clientset, log)
	if err != nil {
		log.Error("failed to initialise controller", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := ctrl.Run(ctx); err != nil {
		log.Error("controller exited with error", "error", err)
		os.Exit(1)
	}
}
