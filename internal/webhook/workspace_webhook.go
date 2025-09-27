// Package webhook provides mutating admission webhooks for Workspace resources.
// The WorkspaceMutator automatically adds ownership annotations to track resource creators.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// WorkspaceMutator is a mutating admission webhook that automatically adds a "created-by" annotation
// to Workspace resources when they are created. This annotation tracks which user created the Workspace
// and can be used for ownership-based access control and auditing.
type WorkspaceMutator struct{}

// Handle processes admission requests for Workspace resources and adds a "created-by" annotation
// containing the sanitized username of the requesting user. This enables ownership tracking
// and audit trails for workspace creation.
func (m *WorkspaceMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := log.FromContext(ctx).WithName("workspace-webhook")

	logger.V(1).Info("Processing admission request",
		"kind", req.Kind.Kind,
		"group", req.Kind.Group,
		"namespace", req.Namespace,
		"name", req.Name,
		"user", req.UserInfo.Username)

	if req.Kind.Kind != "Workspace" || req.Kind.Group != "workspaces.jupyter.org" {
		logger.Info("Skipping non-Workspace resource", "kind", req.Kind.Kind, "group", req.Kind.Group)
		return admission.Allowed("Not a workspaces.jupyter.org/Workspace resource")
	}

	if req.Object.Raw == nil {
		logger.Error(nil, "Request object is nil")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("request object is nil"))
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(req.Object.Raw, &obj); err != nil {
		logger.Error(err, "Failed to unmarshal object")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("error unmarshaling object: %v", err))
	}

	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		logger.Error(nil, "Metadata field not found or invalid")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("metadata field not found or invalid"))
	}

	sanitizedUsername := sanitizeUsername(req.UserInfo.Username)
	logger.Info("Adding created-by annotation",
		"workspace", req.Name,
		"namespace", req.Namespace,
		"user", sanitizedUsername)

	var patch string
	if annotations, ok := metadata["annotations"].(map[string]interface{}); ok && annotations != nil {
		patch = `[{"op":"add","path":"/metadata/annotations/created-by","value":"` + sanitizedUsername + `"}]`
		logger.V(1).Info("Adding annotation to existing annotations", "patch", patch)
	} else {
		patch = `[{"op":"add","path":"/metadata/annotations","value":{"created-by":"` + sanitizedUsername + `"}}]`
		logger.V(1).Info("Creating new annotations object", "patch", patch)
	}
	patchType := admissionv1.PatchTypeJSONPatch

	logger.Info("Successfully processed workspace admission request",
		"workspace", req.Name,
		"namespace", req.Namespace,
		"user", sanitizedUsername)

	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			UID:       req.UID,
			Allowed:   true,
			Result:    &metav1.Status{Code: http.StatusOK},
			Patch:     []byte(patch),
			PatchType: &patchType,
		},
	}
}

// InjectDecoder injects the admission decoder. This method is required by the
// admission.DecoderInjector interface but is not used by this webhook implementation.
func (m *WorkspaceMutator) InjectDecoder(d *admission.Decoder) error {
	return nil
}

// sanitizeUsername properly escapes characters for JSON without changing the username
func sanitizeUsername(username string) string {
	// Use Go's JSON marshaling to properly escape the string
	escaped, _ := json.Marshal(username)
	// Remove the surrounding quotes that json.Marshal adds
	return string(escaped[1 : len(escaped)-1])
}
