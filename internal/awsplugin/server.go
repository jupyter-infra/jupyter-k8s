/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package awsplugin

import (
	"context"
	"fmt"

	"github.com/jupyter-infra/jupyter-k8s/internal/pluginserver"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// NewServer creates a PluginServer wired with AWS SSM handlers.
func NewServer(ctx context.Context) (*pluginserver.PluginServer, error) {
	// Create SSM client
	ssmClient, err := NewSSMClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSM client: %w", err)
	}

	// Create handlers
	remoteAccessHandler := NewAWSRemoteAccessRoutes(ssmClient)

	return pluginserver.NewPluginServer(pluginserver.ServerConfig{
		RemoteAccessHandler: remoteAccessHandler,
		Logger:              ctrllog.Log.WithName("pluginserver"),
	}), nil
}
