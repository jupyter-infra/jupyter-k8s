/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package main provides the entry point for the authmiddleware service that
// handles JWT-based authentication and authorization for Jupyter-k8s workspaces.
package main

import (
	"os"

	"github.com/jupyter-infra/jupyter-k8s/internal/authmiddleware"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	// Setup logger for controller-runtime
	opts := zap.Options{
		Development: false,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog := ctrl.Log.WithName("setup")

	// Load configuration
	cfg, err := authmiddleware.NewConfig()
	if err != nil {
		setupLog.Error(err, "Failed to load configuration")
		os.Exit(1)
	}

	// Get in-cluster Kubernetes config
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		setupLog.Error(err, "Failed to get in-cluster config")
		os.Exit(1)
	}

	// Use namespace from config (set via NAMESPACE environment variable from Helm)
	if cfg.Namespace == "" {
		setupLog.Error(nil, "NAMESPACE environment variable must be set")
		os.Exit(1)
	}

	setupLog.Info("Configuring manager to watch single namespace", "namespace", cfg.Namespace)

	// Create scheme and add corev1 for Secret informers
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Create manager with namespace-scoped cache
	mgr, err := ctrl.NewManager(k8sConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.MetricsAddr,
		},
		HealthProbeBindAddress: cfg.ProbeAddr,
		LeaderElection:         false, // No leader election needed for stateless services
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				cfg.Namespace: {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "Failed to create manager")
		os.Exit(1)
	}

	// Setup authmiddleware with manager
	if err := authmiddleware.SetupAuthMiddlewareWithManager(mgr, cfg); err != nil {
		setupLog.Error(err, "Failed to setup authmiddleware")
		os.Exit(1)
	}

	// Add health check
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to add health check")
		os.Exit(1)
	}

	// Add readiness check
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to add readiness check")
		os.Exit(1)
	}

	setupLog.Info("Starting authmiddleware manager")

	// Start manager (blocks until signal or error)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager exited with error")
		os.Exit(1)
	}
}
