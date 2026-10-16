/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package v1alpha1 contains API Schema definitions for the workspaces v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=workspace.jupyter.org
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "workspace.jupyter.org", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		GroupVersion,
		&Workspace{},
		&WorkspaceList{},
		&WorkspaceAccessStrategy{},
		&WorkspaceAccessStrategyList{},
		&WorkspaceIntegrationTemplate{},
		&WorkspaceIntegrationTemplateList{},
		&WorkspaceTemplate{},
		&WorkspaceTemplateList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
