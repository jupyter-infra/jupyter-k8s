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

package extensionapi

import (
	"fmt"
	"net/http"

	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
)

// handleDiscovery responds with API resource discovery information
func (s *ExtensionServer) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := fmt.Sprintf(`{
		"kind": "APIResourceList",
		"apiVersion": "v1",
		"groupVersion": "%s",
		"resources": [{
			"name": "workspaceconnections",
			"singularName": "workspaceconnection",
			"namespaced": true,
			"kind": "%s",
			"verbs": ["create"]
		}, {
			"name": "connectionaccessreviews",
			"singularName": "connectionaccessreview",
			"namespaced": true,
			"kind": "ConnectionAccessReview",
			"verbs": ["create"]
		}]
	}`, connectionv1alpha1.WorkspaceConnectionAPIVersion, connectionv1alpha1.WorkspaceConnectionKind)

	_, err := w.Write([]byte(response))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write discovery body")
	}
}
