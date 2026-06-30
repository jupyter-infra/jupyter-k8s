/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// workspaceintegrationlog is for logging in this package.
var workspaceintegrationlog = logf.Log.WithName("workspaceintegration-resource")

// SetupWorkspaceIntegrationWebhookWithManager registers the mutating webhook for
// WorkspaceIntegration in the manager.
func SetupWorkspaceIntegrationWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&workspacev1alpha1.WorkspaceIntegration{}).
		WithDefaulter(&WorkspaceIntegrationCustomDefaulter{
			client: mgr.GetClient(),
		}).
		Complete()
}

// The WorkspaceIntegration webhook reads the template-referenced resources (e.g. a RayCluster)
// at build time, running under the controller-manager ServiceAccount, so the manager needs
// get;list on those resources. ray.io/rayclusters is the resource the shipped ray-integration
// template references; add further groups here as new integration templates reference new Kinds.
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters,verbs=get;list
// +kubebuilder:webhook:path=/mutate-workspace-jupyter-org-v1alpha1-workspaceintegration,mutating=true,failurePolicy=fail,sideEffects=None,groups=workspace.jupyter.org,resources=workspaceintegrations,verbs=create;update,versions=v1alpha1,name=mworkspaceintegration-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceIntegrationCustomDefaulter resolves a WorkspaceIntegration at its own admission
// (CREATE/UPDATE) and freezes the literal result onto the child's own spec output fields
// (deploymentModifications/statusProbe/shareProcessNamespace), so the workspace controller reads
// only the frozen child and never re-resolves at reconcile time. failurePolicy is Fail because
// this webhook is the sole producer of that output -- see the resolution contract on
// controller.BuildWorkspaceIntegration.
type WorkspaceIntegrationCustomDefaulter struct {
	client client.Client
}

var _ webhook.CustomDefaulter = &WorkspaceIntegrationCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind
// WorkspaceIntegration.
func (d *WorkspaceIntegrationCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	wi, ok := obj.(*workspacev1alpha1.WorkspaceIntegration)
	if !ok {
		return fmt.Errorf("expected a WorkspaceIntegration object but got %T", obj)
	}

	// Defaulters don't run on delete, but guard defensively.
	if !wi.DeletionTimestamp.IsZero() {
		return nil
	}

	template, err := d.fetchTemplate(ctx, wi)
	if err != nil {
		workspaceintegrationlog.Error(err, "Failed to fetch integration template for building",
			"workspaceintegration", wi.GetName(), "template", wi.Spec.TemplateRef.Name)
		return err
	}

	// BuildWorkspaceIntegration resolves the template against the live referenced resources and
	// writes the literal result directly onto wi.Spec (deploymentModifications/shareProcessNamespace/
	// statusProbe). It assigns every output field unconditionally, so any value the submitter put on
	// those fields is overwritten before persist -- the webhook is the sole, authoritative producer.
	if err := controller.BuildWorkspaceIntegration(ctx, d.client, wi, template); err != nil {
		workspaceintegrationlog.Error(err, "Failed to resolve WorkspaceIntegration",
			"workspaceintegration", wi.GetName(), "template", wi.Spec.TemplateRef.Name)
		return fmt.Errorf("failed to resolve WorkspaceIntegration %q: %w", wi.GetName(), err)
	}

	workspaceintegrationlog.Info("Resolved WorkspaceIntegration spec at admission",
		"workspaceintegration", wi.GetName(), "template", wi.Spec.TemplateRef.Name)
	return nil
}

// fetchTemplate retrieves the WorkspaceIntegrationTemplate referenced by the WorkspaceIntegration's
// templateRef. The template namespace defaults to the WorkspaceIntegration's own namespace when
// templateRef.namespace is unset.
func (d *WorkspaceIntegrationCustomDefaulter) fetchTemplate(
	ctx context.Context,
	wi *workspacev1alpha1.WorkspaceIntegration,
) (*workspacev1alpha1.WorkspaceIntegrationTemplate, error) {
	namespace := wi.Spec.TemplateRef.Namespace
	if namespace == "" {
		namespace = wi.Namespace
	}

	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{}
	key := types.NamespacedName{Name: wi.Spec.TemplateRef.Name, Namespace: namespace}
	if err := d.client.Get(ctx, key, template); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("integration template %q not found in namespace %q", wi.Spec.TemplateRef.Name, namespace)
		}
		return nil, fmt.Errorf("failed to get integration template %q in namespace %q: %w", wi.Spec.TemplateRef.Name, namespace, err)
	}
	return template, nil
}
