/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// integrationTemplateLog logs for the WorkspaceIntegrationTemplate validating webhook.
var integrationTemplateLog = logf.Log.WithName("workspaceintegrationtemplate-webhook")

// SetupWorkspaceIntegrationTemplateWebhookWithManager registers the validating webhook for
// WorkspaceIntegrationTemplate. Validating the template at admission (rather than only at controller
// resolve time) surfaces authoring mistakes -- bad Go template syntax, a {{ resource }} that names an
// undeclared resourceRef, an invalid JSONPath -- to the template AUTHOR immediately, instead of
// silently producing degraded workspaces whose errors only appear later in operator logs.
func SetupWorkspaceIntegrationTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &workspacev1alpha1.WorkspaceIntegrationTemplate{}).
		WithValidator(&WorkspaceIntegrationTemplateCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-workspace-jupyter-org-v1alpha1-workspaceintegrationtemplate,mutating=false,failurePolicy=ignore,sideEffects=None,groups=workspace.jupyter.org,resources=workspaceintegrationtemplates,verbs=create;update,versions=v1alpha1,name=vworkspaceintegrationtemplate-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceIntegrationTemplateCustomValidator statically validates a WorkspaceIntegrationTemplate on
// create and update. failurePolicy=ignore: this validating webhook is OPTIONAL -- it is registered only
// when the ENABLE_WORKSPACE_INTEGRATION_TEMPLATE_WEBHOOK env is not "false" (see cmd/main.go). With
// failurePolicy=fail, disabling the webhook while its ValidatingWebhookConfiguration is still installed
// would route every WIT write to a dead endpoint and reject all WIT create/update cluster-wide.
// failurePolicy=ignore fails OPEN only when the endpoint is unreachable -- a reachable webhook still
// rejects malformed templates outright -- matching the sibling WorkspaceTemplate webhook.
//
// +kubebuilder:object:generate=false
type WorkspaceIntegrationTemplateCustomValidator struct{}

var _ admission.Validator[*workspacev1alpha1.WorkspaceIntegrationTemplate] = &WorkspaceIntegrationTemplateCustomValidator{}

// ValidateCreate rejects a template whose templated fields don't statically validate.
func (v *WorkspaceIntegrationTemplateCustomValidator) ValidateCreate(_ context.Context, tmpl *workspacev1alpha1.WorkspaceIntegrationTemplate) (admission.Warnings, error) {
	return v.validate(tmpl, "create")
}

// ValidateUpdate re-validates on every edit, so a change that breaks a templated field is caught.
func (v *WorkspaceIntegrationTemplateCustomValidator) ValidateUpdate(_ context.Context, _, newTmpl *workspacev1alpha1.WorkspaceIntegrationTemplate) (admission.Warnings, error) {
	return v.validate(newTmpl, "update")
}

// ValidateDelete is a no-op (deletion needs no template validation).
func (v *WorkspaceIntegrationTemplateCustomValidator) ValidateDelete(_ context.Context, _ *workspacev1alpha1.WorkspaceIntegrationTemplate) (admission.Warnings, error) {
	return nil, nil
}

func (v *WorkspaceIntegrationTemplateCustomValidator) validate(tmpl *workspacev1alpha1.WorkspaceIntegrationTemplate, op string) (admission.Warnings, error) {
	if err := validateIntegrationTemplate(tmpl); err != nil {
		integrationTemplateLog.Info("Rejected WorkspaceIntegrationTemplate: static template validation failed",
			"operation", op, "name", tmpl.GetName(), "namespace", tmpl.GetNamespace(), "error", err.Error())
		return nil, fmt.Errorf("invalid WorkspaceIntegrationTemplate %q: %w", tmpl.GetName(), err)
	}
	integrationTemplateLog.V(1).Info("WorkspaceIntegrationTemplate passed static validation",
		"operation", op, "name", tmpl.GetName(), "namespace", tmpl.GetNamespace())
	return nil, nil
}
