/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHandleHealthHappyPath tests that the health endpoint works as expected
func TestHandleHealthHappyPath(t *testing.T) {
	// Create a minimal server for testing
	config := &Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{
		config: config,
		logger: logger,
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Call handler
	server.handleHealth(w, req)

	// Check response code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type: application/json, got %s", contentType)
	}

	// Parse JSON response
	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Check status field
	status, ok := response["status"]
	if !ok {
		t.Error("Response missing 'status' field")
	} else if status != "ok" {
		t.Errorf("Expected status to be 'ok', got %q", status)
	}

	// Check time field
	timeStr, ok := response["time"]
	if !ok {
		t.Error("Response missing 'time' field")
	} else {
		// Try to parse the time string to validate it's in RFC3339 format
		_, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			t.Errorf("Invalid time format: %v", err)
		}
	}
}

// TestHandleHealthTimestampIsUTC tests that the health response timestamp is in UTC
func TestHandleHealthTimestampIsUTC(t *testing.T) {
	// Create a minimal server for testing
	config := &Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{
		config: config,
		logger: logger,
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Call handler
	server.handleHealth(w, req)

	// Parse JSON response
	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Check time field is in UTC (ends with Z)
	timeStr, ok := response["time"]
	if !ok {
		t.Error("Response missing 'time' field")
	} else if !strings.HasSuffix(timeStr, "Z") {
		t.Errorf("Time %q does not appear to be in UTC (should end with Z)", timeStr)
	}
}
