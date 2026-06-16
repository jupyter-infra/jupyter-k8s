/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

func TestNetworkHTTPGetDetector_NotIdle(t *testing.T) {
	recentTime := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	body := fmt.Sprintf(`{"lastActiveTimestamp": "%s"}`, recentTime)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body) //nolint:errcheck
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv)
	detector := NewNetworkHTTPGetDetectorWithClient(srv.Client())

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1", Namespace: "default"},
	}
	idleConfig := &workspacev1alpha1.IdleShutdownSpec{
		IdleTimeoutInMinutes: 30,
		Detection: workspacev1alpha1.IdleDetectionSpec{
			HTTPGet: &workspacev1alpha1.IdleHTTPGetAction{
				HTTPGetAction: corev1.HTTPGetAction{
					Path: "/api/status",
					Port: intstr.FromString(port),
				},
				Transport: "network",
			},
		},
	}

	result, err := detector.CheckIdle(context.Background(), workspace, host, idleConfig)
	assert.NoError(t, err)
	assert.False(t, result.IsIdle)
	assert.True(t, result.ShouldRetry)
}

func TestNetworkHTTPGetDetector_IsIdle(t *testing.T) {
	oldTime := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"lastActiveTimestamp": "%s"}`, oldTime)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body) //nolint:errcheck
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv)
	detector := NewNetworkHTTPGetDetectorWithClient(srv.Client())

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1", Namespace: "default"},
	}
	idleConfig := &workspacev1alpha1.IdleShutdownSpec{
		IdleTimeoutInMinutes: 60,
		Detection: workspacev1alpha1.IdleDetectionSpec{
			HTTPGet: &workspacev1alpha1.IdleHTTPGetAction{
				HTTPGetAction: corev1.HTTPGetAction{
					Path: "/api/status",
					Port: intstr.FromString(port),
				},
				Transport: "network",
			},
		},
	}

	result, err := detector.CheckIdle(context.Background(), workspace, host, idleConfig)
	assert.NoError(t, err)
	assert.True(t, result.IsIdle)
}

func TestNetworkHTTPGetDetector_CustomFieldPath(t *testing.T) {
	recentTime := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	body := fmt.Sprintf(`{"connections": 0, "last_activity": "%s"}`, recentTime)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body) //nolint:errcheck
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv)
	detector := NewNetworkHTTPGetDetectorWithClient(srv.Client())

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1", Namespace: "default"},
	}
	idleConfig := &workspacev1alpha1.IdleShutdownSpec{
		IdleTimeoutInMinutes: 30,
		Detection: workspacev1alpha1.IdleDetectionSpec{
			HTTPGet: &workspacev1alpha1.IdleHTTPGetAction{
				HTTPGetAction: corev1.HTTPGetAction{
					Path: "/api/status",
					Port: intstr.FromString(port),
				},
				Transport: "network",
				LastActivityTimestamp: &workspacev1alpha1.IdleLastActivityTimestampSpec{
					ResponseBodyPath: "last_activity",
					Format:           "RFC3339",
				},
			},
		},
	}

	result, err := detector.CheckIdle(context.Background(), workspace, host, idleConfig)
	assert.NoError(t, err)
	assert.False(t, result.IsIdle)
}

func TestNetworkHTTPGetDetector_UnixTimestamp(t *testing.T) {
	// Recent epoch timestamp
	recentEpoch := fmt.Sprintf("%d", time.Now().Add(-3*time.Minute).Unix())
	body := fmt.Sprintf(`{"ts": %s}`, recentEpoch)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body) //nolint:errcheck
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv)
	detector := NewNetworkHTTPGetDetectorWithClient(srv.Client())

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1", Namespace: "default"},
	}
	idleConfig := &workspacev1alpha1.IdleShutdownSpec{
		IdleTimeoutInMinutes: 30,
		Detection: workspacev1alpha1.IdleDetectionSpec{
			HTTPGet: &workspacev1alpha1.IdleHTTPGetAction{
				HTTPGetAction: corev1.HTTPGetAction{
					Path: "/status",
					Port: intstr.FromString(port),
				},
				Transport: "network",
				LastActivityTimestamp: &workspacev1alpha1.IdleLastActivityTimestampSpec{
					ResponseBodyPath: "ts",
					Format:           "unix",
				},
			},
		},
	}

	result, err := detector.CheckIdle(context.Background(), workspace, host, idleConfig)
	assert.NoError(t, err)
	assert.False(t, result.IsIdle)
}

func TestNetworkHTTPGetDetector_404_PermanentFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv)
	detector := NewNetworkHTTPGetDetectorWithClient(srv.Client())

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1", Namespace: "default"},
	}
	idleConfig := &workspacev1alpha1.IdleShutdownSpec{
		IdleTimeoutInMinutes: 30,
		Detection: workspacev1alpha1.IdleDetectionSpec{
			HTTPGet: &workspacev1alpha1.IdleHTTPGetAction{
				HTTPGetAction: corev1.HTTPGetAction{
					Path: "/api/status",
					Port: intstr.FromString(port),
				},
				Transport: "network",
			},
		},
	}

	result, err := detector.CheckIdle(context.Background(), workspace, host, idleConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint not found")
	assert.False(t, result.IsIdle)
	assert.False(t, result.ShouldRetry)
}

func TestNetworkHTTPGetDetector_ApplicationBasePath(t *testing.T) {
	recentTime := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	body := fmt.Sprintf(`{"lastActiveTimestamp": "%s"}`, recentTime)

	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body) //nolint:errcheck
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv)
	detector := NewNetworkHTTPGetDetectorWithClient(srv.Client())

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1", Namespace: "default"},
		Status: workspacev1alpha1.WorkspaceStatus{
			ApplicationBasePath: "/workspaces/default/ws1/",
		},
	}
	idleConfig := &workspacev1alpha1.IdleShutdownSpec{
		IdleTimeoutInMinutes: 30,
		Detection: workspacev1alpha1.IdleDetectionSpec{
			HTTPGet: &workspacev1alpha1.IdleHTTPGetAction{
				HTTPGetAction: corev1.HTTPGetAction{
					Path: "/api/status",
					Port: intstr.FromString(port),
				},
				Transport: "network",
			},
		},
	}

	result, err := detector.CheckIdle(context.Background(), workspace, host, idleConfig)
	assert.NoError(t, err)
	assert.False(t, result.IsIdle)
	assert.Equal(t, "/workspaces/default/ws1/api/status", receivedPath)
}


func splitHostPort(t *testing.T, srv *httptest.Server) (string, string) {
	t.Helper()
	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host:port: %v", err)
	}
	return host, port
}
