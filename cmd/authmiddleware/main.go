/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package main provides the entry point for the authmiddleware service that
// handles JWT-based authentication and authorization for Jupyter-k8s workspaces.
package main

import (
	"log/slog"
	"os"

	"github.com/jupyter-infra/jupyter-k8s/internal/authmiddleware"
)

func main() {
	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load configuration
	cfg, err := authmiddleware.NewConfig()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Create JWT handler
	jwtHandler, err := authmiddleware.NewJWTHandler(cfg)
	if err != nil {
		logger.Error("Failed to create JWT handler", "error", err)
		os.Exit(1)
	}

	// Create cookie manager
	cookieManager, err := authmiddleware.NewCookieManager(cfg)
	if err != nil {
		logger.Error("Failed to create cookie manager", "error", err)
		os.Exit(1)
	}

	// Create and start server
	server := authmiddleware.NewServer(cfg, jwtHandler, cookieManager, logger)
	if err := server.Start(); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
