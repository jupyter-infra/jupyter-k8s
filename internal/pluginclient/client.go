/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package pluginclient provides HTTP clients that implement core operator interfaces
// (jwt.SignerFactory, RemoteAccessStrategyInterface) by calling plugin sidecar endpoints.
package pluginclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"
)

// PluginClient is the shared HTTP client for communicating with a plugin sidecar.
type PluginClient struct {
	baseURL     string
	httpClient  *http.Client
	logger      *slog.Logger
	retryCount  int
	retryDelay  time.Duration
	callTimeout time.Duration
}

// NewPluginClient creates a new PluginClient for the given plugin base URL.
func NewPluginClient(baseURL string, logger *slog.Logger) *PluginClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &PluginClient{
		baseURL:     baseURL,
		httpClient:  &http.Client{},
		logger:      logger,
		retryCount:  defaultRetryCount,
		retryDelay:  defaultRetryDelay,
		callTimeout: defaultCallTimeout,
	}
}

// doPost sends a JSON POST request to the plugin and decodes the response.
//
// Retry behavior (see isRetryableError for the full contract):
//   - Retries only on transport-level errors where no response was received
//     (connection refused, reset, EOF). These indicate sidecar unavailability.
//   - Never retries on HTTP responses (including 500/503) — the plugin already
//     processed the request and owns retrying its own cloud provider calls.
//   - Never retries on context cancellation or timeout — respects caller intent.
//
// No per-retry header is needed: since retries only happen on transport failures,
// the plugin never receives a request on failed attempts. By the time a request
// reaches the plugin, it is always the first delivery of that X-Plugin-Request-ID.
//
// Each attempt gets its own per-call timeout derived from callTimeout.
// The caller's context is checked before each retry to allow early exit.
func doPost[Resp any](ctx context.Context, c *PluginClient, path string, reqBody any) (*Resp, int, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("plugin client: marshal request: %w", err)
	}

	callID := plugin.GenerateRequestID()
	logAttrs := []any{
		"path", path,
		"pluginRequestID", callID,
	}
	if originID := plugin.OriginRequestID(ctx); originID != "" {
		logAttrs = append(logAttrs, "originRequestID", originID)
	}

	originalStart := time.Now()
	start := originalStart
	var lastErr error
	var lastDuration time.Duration
	for attempt := range c.retryCount + 1 {
		if attempt > 0 {
			c.logger.WarnContext(ctx, "retrying plugin call",
				append(logAttrs, "attempt", attempt+1, "maxAttempts", c.retryCount+1,
					"lastError", lastErr, "lastDuration", lastDuration)...)

			// Wait for retry delay, but respect the caller's context.
			select {
			case <-ctx.Done():
				return nil, 0, fmt.Errorf("plugin client: terminated before retry %d to %s: %w", attempt, path, ctx.Err())
			case <-time.After(c.retryDelay):
			}
			start = time.Now()
		}

		resp, err := c.doSinglePost(ctx, path, body, callID)

		if err != nil {
			lastDuration = time.Since(start)
			lastErr = err
			if isRetryableError(err) {
				continue
			}
			// Non-retryable transport error (context canceled, timeout, unknown).
			c.logger.ErrorContext(ctx, "plugin call failed (non-retryable)",
				append(logAttrs, "attempt", attempt+1, "duration", lastDuration,
					"totalDuration", time.Since(originalStart), "error", err)...)
			return nil, 0, fmt.Errorf("plugin client: do request to %s: %w", path, err)
		}

		// Got an HTTP response — never retry from here, regardless of status code.
		return handleResponse[Resp](c, ctx, path, resp, logAttrs, time.Since(originalStart))
	}

	// All retries exhausted — the sidecar was unreachable on every attempt.
	c.logger.ErrorContext(ctx, "plugin call failed (retries exhausted)",
		append(logAttrs, "attempts", c.retryCount+1, "totalDuration", time.Since(originalStart), "error", lastErr)...)
	return nil, 0, fmt.Errorf("plugin client: %d attempts to %s failed: %w", c.retryCount+1, path, lastErr)
}

// doSinglePost executes one HTTP POST with a per-call timeout.
func (c *PluginClient) doSinglePost(ctx context.Context, path string, body []byte, callID string) (*http.Response, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.callTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(plugin.HeaderPluginRequestID, callID)
	if originID := plugin.OriginRequestID(ctx); originID != "" {
		httpReq.Header.Set(plugin.HeaderOriginRequestID, originID)
	}

	return c.httpClient.Do(httpReq)
}

// handleResponse reads and decodes an HTTP response. Called only when we got a
// response from the plugin (any status code).
func handleResponse[Resp any](c *PluginClient, ctx context.Context, path string, resp *http.Response, baseLogAttrs []any, totalDuration time.Duration) (*Resp, int, error) {
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("plugin client: read response from %s: %w", path, err)
	}

	logAttrs := append(append([]any{}, baseLogAttrs...), "status", resp.StatusCode, "totalDuration", totalDuration)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp pluginapi.ErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		c.logger.ErrorContext(ctx, "plugin call returned error", append(logAttrs, "error", errResp.Error)...)
		return nil, resp.StatusCode, &plugin.StatusError{Code: resp.StatusCode, Message: errResp.Error}
	}

	c.logger.InfoContext(ctx, "plugin call succeeded", logAttrs...)

	var result Resp
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("plugin client: decode response from %s: %w", path, err)
	}
	return &result, resp.StatusCode, nil
}
