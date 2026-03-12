/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleBearerTokenReview_HappyPath(t *testing.T) {
	claims := &jwt.Claims{
		User:      "alice",
		UID:       "alice-uid",
		Groups:    []string{"team-a"},
		Path:      "/workspaces/default/ws",
		Domain:    "example.com",
		TokenType: jwt.TokenTypeBootstrap,
	}
	server := newTestBearerTokenReviewServer(&mockTokenValidator{claims: claims})

	body := `{"spec":{"token":"valid-token"}}`
	req := httptest.NewRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/bearertokenreviews", strings.NewReader(body))
	rr := httptest.NewRecorder()

	server.handleBearerTokenReview(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var review connectionv1alpha1.BearerTokenReview
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &review))
	assert.True(t, review.Status.Authenticated)
	assert.Equal(t, "alice", review.Status.User.Username)
	assert.Equal(t, "alice-uid", review.Status.User.UID)
	assert.Equal(t, []string{"team-a"}, review.Status.User.Groups)
	assert.Equal(t, "/workspaces/default/ws", review.Status.Path)
	assert.Equal(t, "example.com", review.Status.Domain)
	assert.Empty(t, review.Status.Error)
}

func TestHandleBearerTokenReview_InvalidToken(t *testing.T) {
	server := newTestBearerTokenReviewServer(&mockTokenValidator{
		err: fmt.Errorf("signature verification failed"),
	})

	body := `{"spec":{"token":"bad-token"}}`
	req := httptest.NewRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/bearertokenreviews", strings.NewReader(body))
	rr := httptest.NewRecorder()

	server.handleBearerTokenReview(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var review connectionv1alpha1.BearerTokenReview
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &review))
	assert.False(t, review.Status.Authenticated)
	assert.Equal(t, "invalid or expired token", review.Status.Error)
}

func TestHandleBearerTokenReview_MissingToken(t *testing.T) {
	server := newTestBearerTokenReviewServer(&mockTokenValidator{})

	body := `{"spec":{"token":""}}`
	req := httptest.NewRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/bearertokenreviews", strings.NewReader(body))
	rr := httptest.NewRecorder()

	server.handleBearerTokenReview(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var review connectionv1alpha1.BearerTokenReview
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &review))
	assert.False(t, review.Status.Authenticated)
	assert.Equal(t, "token is required", review.Status.Error)
}

func TestHandleBearerTokenReview_WrongTokenType(t *testing.T) {
	claims := &jwt.Claims{
		User:      "alice",
		TokenType: jwt.TokenTypeSession, // Session token, not bootstrap
	}
	server := newTestBearerTokenReviewServer(&mockTokenValidator{claims: claims})

	body := `{"spec":{"token":"session-token"}}`
	req := httptest.NewRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/bearertokenreviews", strings.NewReader(body))
	rr := httptest.NewRecorder()

	server.handleBearerTokenReview(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var review connectionv1alpha1.BearerTokenReview
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &review))
	assert.False(t, review.Status.Authenticated)
	assert.Equal(t, "invalid token type", review.Status.Error)
}

func TestHandleBearerTokenReview_WrongMethod(t *testing.T) {
	server := newTestBearerTokenReviewServer(&mockTokenValidator{})

	req := httptest.NewRequest("GET", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/bearertokenreviews", nil)
	rr := httptest.NewRecorder()

	server.handleBearerTokenReview(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleBearerTokenReview_InvalidJSON(t *testing.T) {
	server := newTestBearerTokenReviewServer(&mockTokenValidator{})

	req := httptest.NewRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/bearertokenreviews", strings.NewReader("not json"))
	rr := httptest.NewRecorder()

	server.handleBearerTokenReview(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func newTestBearerTokenReviewServer(validator jwt.TokenValidator) *ExtensionServer {
	logger := logr.Discard()
	return &ExtensionServer{
		config:         NewConfig(),
		logger:         &logger,
		tokenValidator: validator,
	}
}
