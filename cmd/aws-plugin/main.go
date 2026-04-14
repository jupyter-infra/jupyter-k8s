/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package main provides the entry point for the AWS plugin sidecar.
// The plugin runs as a sidecar container alongside the jupyter-k8s controller,
// handling AWS-specific operations (SSM remote access) via
// a localhost HTTP interface.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jupyter-infra/jupyter-k8s/internal/awsplugin"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	// Health check mode for Kubernetes exec probes.
	// The plugin server binds to 127.0.0.1 so that only the co-located manager
	// container can reach it. This means kubelet HTTP/TCP probes cannot reach it
	// (they connect from the node network). Instead, the helm chart configures an
	// exec probe: kubelet runs ["/aws-plugin", "--healthcheck"] inside this container,
	// which creates an HTTP client that hits the server's /healthz endpoint on
	// localhost — this works because the probe process shares the pod's network.
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		port := os.Getenv("PLUGIN_PORT")
		if port == "" {
			port = "8080"
		}
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", port))
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	ctrllog.SetLogger(zap.New(zap.UseDevMode(false)))
	logger := ctrllog.Log.WithName("aws-plugin")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	server, err := awsplugin.NewServer(ctx)
	if err != nil {
		logger.Error(err, "Failed to create server")
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		logger.Info("Starting AWS plugin server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		logger.Info("Shutting down AWS plugin server")
		if err := server.Shutdown(context.Background()); err != nil {
			logger.Error(err, "Server shutdown failed")
			os.Exit(1)
		}
	case err := <-errCh:
		if err != nil {
			logger.Error(err, "Server exited with error")
			os.Exit(1)
		}
	}
}
