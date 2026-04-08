/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginadapters

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newPodWithUID(uid string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			UID:  types.UID(uid),
		},
	}
}

func TestResolvePodContext_StaticValues(t *testing.T) {
	ctx := map[string]string{
		"region":    "us-east-1",
		"container": "main",
	}
	resolved, err := ResolvePodContext(ctx, newPodWithUID("abc-123"))
	assert.NoError(t, err)
	assert.Equal(t, "us-east-1", resolved["region"])
	assert.Equal(t, "main", resolved["container"])
}

func TestResolvePodContext_ResolvesPodUid(t *testing.T) {
	ctx := map[string]string{
		"podUid": "controller::PodUid()",
		"region": "us-west-2",
	}
	resolved, err := ResolvePodContext(ctx, newPodWithUID("pod-uid-456"))
	assert.NoError(t, err)
	assert.Equal(t, "pod-uid-456", resolved["podUid"])
	assert.Equal(t, "us-west-2", resolved["region"])
}

func TestResolvePodContext_EmptyMap(t *testing.T) {
	resolved, err := ResolvePodContext(map[string]string{}, newPodWithUID("abc"))
	assert.NoError(t, err)
	assert.Empty(t, resolved)
}

func TestResolvePodContext_NilMap(t *testing.T) {
	resolved, err := ResolvePodContext(nil, newPodWithUID("abc"))
	assert.NoError(t, err)
	assert.Nil(t, resolved)
}

func TestResolvePodContext_NilPod_ErrorsOnDynamicValue(t *testing.T) {
	ctx := map[string]string{
		"podUid": "controller::PodUid()",
	}
	_, err := ResolvePodContext(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pod is nil")
}

func TestResolvePodContext_UnknownFunction(t *testing.T) {
	ctx := map[string]string{
		"val": "controller::Unknown()",
	}
	_, err := ResolvePodContext(ctx, newPodWithUID("abc"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown controller function")
}

func TestResolvePodContext_DoesNotMutateInput(t *testing.T) {
	ctx := map[string]string{
		"podUid": "controller::PodUid()",
	}
	_, err := ResolvePodContext(ctx, newPodWithUID("xyz"))
	assert.NoError(t, err)
	assert.Equal(t, "controller::PodUid()", ctx["podUid"])
}
