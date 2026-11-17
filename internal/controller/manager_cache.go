/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// ManagerCacheOptions scopes the manager cache to the pods carrying the workspace label, which every pod
// reader wants and the pod-exec webhook relies on (a pod the cache lacks is not a workspace pod; pinned by
// TestManagerCacheOptions), and to the controller namespace's Secrets when the namespace is known, so the
// JWT secret informer needs no cluster-wide secret permission. Namespaces are scoped to those carrying
// the posix-gid label, the only ones the identity resolution reads: an unlabelled namespace reads as
// missing.
func ManagerCacheOptions(controllerNamespace string) (cache.Options, error) {
	workspacePods, err := labels.Parse(LabelWorkspaceName)
	if err != nil {
		return cache.Options{}, fmt.Errorf("workspace pod selector: %w", err)
	}
	gidNamespaces, err := labels.Parse(workspaceutil.LabelPosixGID)
	if err != nil {
		return cache.Options{}, fmt.Errorf("posix-gid namespace selector: %w", err)
	}
	options := cache.Options{ByObject: map[client.Object]cache.ByObject{
		&corev1.Pod{}:       {Label: workspacePods},
		&corev1.Namespace{}: {Label: gidNamespaces},
	}}
	if controllerNamespace != "" {
		options.ByObject[&corev1.Secret{}] = cache.ByObject{Namespaces: map[string]cache.Config{controllerNamespace: {}}}
	}
	return options, nil
}
