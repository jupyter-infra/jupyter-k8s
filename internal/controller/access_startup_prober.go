/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// AccessStartupProberInterface allows mocking in tests
type AccessStartupProberInterface interface {
	Probe(
		ctx context.Context,
		workspace *workspacev1alpha1.Workspace,
		accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
		service *corev1.Service,
	) (bool, error)
}

// AccessStartupProber performs HTTP probes to verify access resources are serving traffic.
// It holds a reusable http.Client to avoid repeated allocation of TLS state and connection pools.
type AccessStartupProber struct {
	builder *AccessResourcesBuilder
	client  *http.Client
}

// NewAccessStartupProber creates a new AccessStartupProber with a shared http.Client.
// http.Client is safe for concurrent use and expensive to create (TLS handshake state,
// connection pool), so we allocate one per prober and set timeouts per-request instead.
func NewAccessStartupProber(builder *AccessResourcesBuilder) *AccessStartupProber {
	return &AccessStartupProber{
		builder: builder,
		client: &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Probe performs a single HTTP GET and returns whether the access route is ready.
func (p *AccessStartupProber) Probe(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) (bool, error) {
	logger := logf.FromContext(ctx)
	probe := accessStrategy.Spec.AccessStartupProbe

	if probe == nil || probe.HTTPGet == nil {
		return false, fmt.Errorf("accessStartupProbe.httpGet is required")
	}

	url, err := p.builder.ResolveTemplateURL(
		probe.HTTPGet.URLTemplate, workspace, accessStrategy, service)
	if err != nil {
		return false, fmt.Errorf("failed to resolve probe URL: %w", err)
	}

	timeout := time.Duration(resolveTimeoutSeconds(probe)) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create probe request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		logger.V(1).Info("Access startup probe connection failed", "url", url, "error", err)
		return false, nil
	}
	defer func() { _ = resp.Body.Close() }()

	success := isProbeStatusSuccess(resp.StatusCode, probe.HTTPGet.AdditionalSuccessStatusCodes)
	logger.V(1).Info("Access startup probe response",
		"url", url, "statusCode", resp.StatusCode, "success", success)
	return success, nil
}

func isProbeStatusSuccess(statusCode int, additionalCodes []int) bool {
	if statusCode >= 200 && statusCode < 400 {
		return true
	}
	for _, code := range additionalCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

func resolveTimeoutSeconds(probe *workspacev1alpha1.AccessStartupProbe) int32 {
	if probe.TimeoutSeconds > 0 {
		return probe.TimeoutSeconds
	}
	return DefaultAccessStartupProbeTimeoutSeconds
}

func resolvePeriodSeconds(probe *workspacev1alpha1.AccessStartupProbe) int32 {
	if probe.PeriodSeconds > 0 {
		return probe.PeriodSeconds
	}
	return DefaultAccessStartupProbePeriodSeconds
}

func resolveFailureThreshold(probe *workspacev1alpha1.AccessStartupProbe) int32 {
	if probe.FailureThreshold > 0 {
		return probe.FailureThreshold
	}
	return DefaultAccessStartupProbeFailureThreshold
}
