/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// cacheScopes returns the Pod and Secret entries of the cache options; the map is keyed by object pointer,
// so a lookup by a fresh pointer would miss.
func cacheScopes(options cache.Options) (pods, secrets *cache.ByObject) {
	pods, secrets, _ = cacheScopesWithNamespaces(options)
	return pods, secrets
}

func cacheScopesWithNamespaces(options cache.Options) (pods, secrets, namespaces *cache.ByObject) {
	for obj, scope := range options.ByObject {
		switch obj.(type) {
		case *corev1.Pod:
			pods = &scope
		case *corev1.Secret:
			secrets = &scope
		case *corev1.Namespace:
			namespaces = &scope
		}
	}
	return pods, secrets, namespaces
}

// The namespace gid lookup treats a namespace the cache lacks as unlabelled, which holds only while the
// cache keeps exactly the namespaces that carry the posix-gid label.
func TestManagerCacheOptionsScopesNamespacesToPosixGIDLabel(t *testing.T) {
	options, err := ManagerCacheOptions("")
	require.NoError(t, err)
	_, _, namespaces := cacheScopesWithNamespaces(options)
	require.NotNil(t, namespaces)
	assert.True(t, namespaces.Label.Matches(labels.Set{workspaceutil.LabelPosixGID: "2130706500"}))
	assert.False(t, namespaces.Label.Matches(labels.Set{AppLabel: AppLabelValue}))
}

// The pod-exec webhook denies a pod the cache does not hold as a non-workspace pod, which holds only while
// the cache keeps exactly the pods that carry the workspace label.
func TestManagerCacheOptions(t *testing.T) {
	options, err := ManagerCacheOptions("")
	require.NoError(t, err)
	pods, secrets := cacheScopes(options)
	require.NotNil(t, pods, "the cache must scope pods")
	assert.True(t, pods.Label.Matches(labels.Set{LabelWorkspaceName: "ws"}))
	assert.False(t, pods.Label.Matches(labels.Set{AppLabel: AppLabelValue}))
	assert.Nil(t, secrets, "secrets stay unscoped without a controller namespace")

	options, err = ManagerCacheOptions("jupyter-k8s-system")
	require.NoError(t, err)
	pods, secrets = cacheScopes(options)
	require.NotNil(t, pods)
	require.NotNil(t, secrets)
	assert.Contains(t, secrets.Namespaces, "jupyter-k8s-system")
}
