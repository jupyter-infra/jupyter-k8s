package extensionapi

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleDiscovery(t *testing.T) {
	req := httptest.NewRequest("GET", "/apis/connection.workspaces.jupyter.org/v1alpha1", nil)
	w := httptest.NewRecorder()

	handleDiscovery(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "APIResourceList")
	assert.Contains(t, w.Body.String(), "connections")
	assert.Contains(t, w.Body.String(), "create")
}
