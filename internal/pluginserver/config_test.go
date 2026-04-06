/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyDefaults_ListenAddress(t *testing.T) {
	cfg := ServerConfig{Port: 9090}
	cfg.applyDefaults()
	assert.Equal(t, DefaultListenAddress, cfg.ListenAddress)
}

func TestApplyDefaults_PreservesExplicitListenAddress(t *testing.T) {
	cfg := ServerConfig{Port: 9090, ListenAddress: "0.0.0.0"}
	cfg.applyDefaults()
	assert.Equal(t, "0.0.0.0", cfg.ListenAddress)
}

func TestApplyDefaults_PortDefaultsTo8080(t *testing.T) {
	t.Setenv(EnvPluginPort, "")
	cfg := ServerConfig{}
	cfg.applyDefaults()
	assert.Equal(t, DefaultPort, cfg.Port)
}

func TestApplyDefaults_PortFromEnv(t *testing.T) {
	t.Setenv(EnvPluginPort, "9090")
	cfg := ServerConfig{}
	cfg.applyDefaults()
	assert.Equal(t, 9090, cfg.Port)
}

func TestApplyDefaults_PortExplicitOverridesEnv(t *testing.T) {
	t.Setenv(EnvPluginPort, "9090")
	cfg := ServerConfig{Port: 7070}
	cfg.applyDefaults()
	assert.Equal(t, 7070, cfg.Port)
}

func TestNewServer_NilHandlersDefaultToNotImplemented(t *testing.T) {
	srv := NewPluginServer(ServerConfig{Port: 8080})
	assert.NotNil(t, srv.jwtHandler)
	assert.NotNil(t, srv.remoteAccessHandler)
	assert.IsType(t, NotImplementedJWTHandler{}, srv.jwtHandler)
	assert.IsType(t, NotImplementedRemoteAccessHandler{}, srv.remoteAccessHandler)
}

func TestNewServer_PreservesExplicitHandlers(t *testing.T) {
	jwt := &mockJWTHandler{}
	ra := &mockRemoteAccessHandler{}
	srv := NewPluginServer(ServerConfig{Port: 8080, JWTHandler: jwt, RemoteAccessHandler: ra})
	assert.Same(t, jwt, srv.jwtHandler)
	assert.Same(t, ra, srv.remoteAccessHandler)
}

func TestNewServer_Addr(t *testing.T) {
	srv := NewPluginServer(ServerConfig{Port: 9090})
	assert.Equal(t, "127.0.0.1:9090", srv.httpServer.Addr)
}

func TestNewServer_CustomListenAddress(t *testing.T) {
	srv := NewPluginServer(ServerConfig{Port: 9090, ListenAddress: "0.0.0.0"})
	assert.Equal(t, "0.0.0.0:9090", srv.httpServer.Addr)
}
