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

	"github.com/jupyter-infra/jupyter-k8s/internal/awsplugin"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
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
