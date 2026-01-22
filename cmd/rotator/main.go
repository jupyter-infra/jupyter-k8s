/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package main implements the JWT secret rotator binary.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jupyter-infra/jupyter-k8s/internal/rotator"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Environment variable names
const (
	EnvSecretName      = "SECRET_NAME"
	EnvSecretNamespace = "SECRET_NAMESPACE"
	EnvNumberOfKeys    = "NUMBER_OF_KEYS"
	EnvDryRun          = "DRY_RUN"
)

// Default values
const (
	DefaultSecretName   = "authmiddleware-secrets"
	DefaultNumberOfKeys = 6
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Parse configuration from environment
	secretName := getEnv(EnvSecretName, DefaultSecretName)
	secretNamespace := os.Getenv(EnvSecretNamespace)
	numberOfKeys := getEnvInt(EnvNumberOfKeys, DefaultNumberOfKeys)
	dryRun := getEnvBool(EnvDryRun, false)

	log.Printf("Starting JWT key rotation...")
	log.Printf("  Secret: %s", secretName)
	log.Printf("  Namespace: %s", secretNamespace)
	log.Printf("  Number of keys: %d", numberOfKeys)
	log.Printf("  Dry run: %v", dryRun)

	// Validate namespace is set
	if secretNamespace == "" {
		log.Fatalf("SECRET_NAMESPACE environment variable must be set")
	}

	// Validate configuration
	if numberOfKeys < 1 {
		log.Fatalf("NUMBER_OF_KEYS must be >= 1, got: %d", numberOfKeys)
	}

	// Create Kubernetes client using controller-runtime
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create in-cluster config: %v", err)
	}

	// Create scheme with core types
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		log.Fatalf("Failed to add corev1 to scheme: %v", err)
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Validate secret exists and has valid keys before rotation
	log.Printf("Validating secret %s in namespace %s...", secretName, secretNamespace)
	if err := rotator.ValidateSecret(ctx, k8sClient, secretName, secretNamespace); err != nil {
		log.Printf("Warning: secret validation failed (this is OK for first run): %v", err)
	} else {
		log.Printf("Secret validation passed")
	}

	if dryRun {
		log.Printf("DRY RUN: Would rotate keys in secret %s/%s (numberOfKeys=%d)",
			secretNamespace, secretName, numberOfKeys)
		log.Printf("DRY RUN: Skipping actual rotation")
		os.Exit(0)
	}

	// Perform rotation
	log.Printf("Rotating keys...")
	if err := rotator.RotateSecret(ctx, k8sClient, secretName, secretNamespace, numberOfKeys); err != nil {
		log.Fatalf("Failed to rotate keys: %v", err)
	}

	log.Printf("Key rotation completed successfully")
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt retrieves an integer environment variable or returns a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		intValue, err := strconv.Atoi(value)
		if err != nil {
			log.Fatalf("Invalid value for %s: %s (must be an integer)", key, value)
		}
		return intValue
	}
	return defaultValue
}

// getEnvBool retrieves a boolean environment variable or returns a default value
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			log.Fatalf("Invalid value for %s: %s (must be true or false)", key, value)
		}
		return boolValue
	}
	return defaultValue
}
