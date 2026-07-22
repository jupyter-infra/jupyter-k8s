/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// These tests exercise the WorkspaceIntegrationTemplateCustomValidator webhook WRAPPER (ValidateCreate/
// ValidateUpdate/ValidateDelete), i.e. that admission delegates to validateIntegrationTemplate and maps
// its result to an admission response. The static-validation logic itself is covered exhaustively in
// integration_template_validation_test.go; here we only assert the wrapper wiring.

// invalidTemplate is a template whose sidecar arg references an undeclared resourceRef, so
// validateIntegrationTemplate rejects it. Reused across the wrapper cases.
func invalidTemplate() *workspacev1alpha1.WorkspaceIntegrationTemplate {
	return &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: testIntegrationName, Namespace: "ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
			DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
				PodModifications: &workspacev1alpha1.PodModifications{
					AdditionalContainers: []corev1.Container{{
						Name: "ray-sidecar",
						Args: []string{undeclaredResourceExpr}, // names a handle no resourceRef declares
					}},
				},
			},
		},
	}
}

func TestWITWebhook_ValidateCreate_AcceptsValidTemplate(t *testing.T) {
	v := &WorkspaceIntegrationTemplateCustomValidator{}
	warnings, err := v.ValidateCreate(context.Background(), validTemplate())
	if err != nil {
		t.Fatalf("expected valid template to be admitted, got error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got: %v", warnings)
	}
}

func TestWITWebhook_ValidateCreate_RejectsInvalidTemplate(t *testing.T) {
	v := &WorkspaceIntegrationTemplateCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), invalidTemplate())
	if err == nil {
		t.Fatal("expected an invalid template to be rejected on create, got nil error")
	}
	// The wrapper wraps the underlying error with the template name for operator-friendly messaging.
	if !strings.Contains(err.Error(), testIntegrationName) {
		t.Errorf("expected error to name the template %q, got: %v", testIntegrationName, err)
	}
	if !strings.Contains(err.Error(), "otherCluster") {
		t.Errorf("expected error to surface the underlying validation cause, got: %v", err)
	}
}

func TestWITWebhook_ValidateUpdate_RevalidatesNewTemplate(t *testing.T) {
	v := &WorkspaceIntegrationTemplateCustomValidator{}

	// A previously-valid template edited into an invalid one must be rejected on update: the wrapper
	// validates the NEW object, not the old one.
	if _, err := v.ValidateUpdate(context.Background(), validTemplate(), invalidTemplate()); err == nil {
		t.Fatal("expected update to an invalid template to be rejected, got nil error")
	}

	// A valid new object is admitted regardless of the old object.
	if _, err := v.ValidateUpdate(context.Background(), invalidTemplate(), validTemplate()); err != nil {
		t.Fatalf("expected update to a valid template to be admitted, got error: %v", err)
	}
}

func TestWITWebhook_ValidateDelete_IsNoOp(t *testing.T) {
	v := &WorkspaceIntegrationTemplateCustomValidator{}
	// Even a template that would fail static validation is fine to delete.
	warnings, err := v.ValidateDelete(context.Background(), invalidTemplate())
	if err != nil {
		t.Fatalf("expected delete to be a no-op, got error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings on delete, got: %v", warnings)
	}
}
