/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"context"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// Shared fixtures of the package's tests, and the tests spanning allocation, namespace gids and
// resolution together. Tests of a single file live next to it (allocator_test.go, resolver_test.go...).

const (
	alice = "github:Alice"
	bob   = "github:Bob"

	tplUserUID      = "{{ .User.Uid }}"
	tplUserGID      = "{{ .User.Gid }}"
	tplNamespaceGID = "{{ .Namespace.Gid }}"
)

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := workspacev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithIndex(&workspacev1alpha1.PosixIdentity{}, IndexUsernameHash, indexUsernameHash).
		WithIndex(&workspacev1alpha1.Workspace{}, IndexWorkspaceClaimedUID, indexWorkspaceClaimedUID).
		WithIndex(&workspacev1alpha1.Workspace{}, IndexWorkspaceClaimOwner, indexWorkspaceClaimOwner).
		WithIndex(&corev1.Namespace{}, IndexNamespaceGID, indexNamespaceGID).
		Build()
}

func claimingWorkspace(name, owner string, uid int64) *workspacev1alpha1.Workspace {
	return &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: "team-a",
		Annotations: map[string]string{workspaceutil.AnnotationCreatedBy: owner},
		Labels:      map[string]string{workspaceutil.LabelPosixUID: strconv.FormatInt(uid, 10)},
	}}
}

func labelledNamespace(name, gid string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{workspaceutil.LabelPosixGID: gid}}}
}

func mustUID(t *testing.T, identity *workspacev1alpha1.PosixIdentity) int64 {
	t.Helper()
	uid, err := UIDOf(identity)
	if err != nil {
		t.Fatal(err)
	}
	return uid
}

// hardened sets the fields a template with identity templates must set.
func hardened(sc *workspacev1alpha1.TemplatedPodSecurityContext) *workspacev1alpha1.TemplatedPodSecurityContext {
	sc.RunAsNonRoot = ptr.To(true)
	sc.SupplementalGroupsPolicy = ptr.To(corev1.SupplementalGroupsPolicyStrict)
	return sc
}

func tpl(text string) *intstr.IntOrString {
	v := intstr.FromString(text)
	return &v
}

func TestResolvePrivateAndSharedTemplates(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t, labelledNamespace("team-a", "2130706500"))
	r := NewResolver(NewAllocator(c, c), c)

	private := &workspacev1alpha1.WorkspaceTemplateSpec{
		DefaultPodSecurityContext: &workspacev1alpha1.TemplatedPodSecurityContext{
			RunAsNonRoot: ptr.To(true), SupplementalGroupsPolicy: ptr.To(corev1.SupplementalGroupsPolicyStrict),
			RunAsUser: tpl(tplUserUID), RunAsGroup: tpl(tplUserGID), FSGroup: tpl(tplUserGID),
			SupplementalGroups:  []intstr.IntOrString{*tpl(tplNamespaceGID), intstr.FromInt32(2147000123)},
			FSGroupChangePolicy: ptr.To(corev1.FSGroupChangeOnRootMismatch),
		},
	}
	id, err := r.Resolve(ctx, private, alice, "team-a", false)
	if err != nil {
		t.Fatal(err)
	}
	uid := v1Candidates(alice)[0]
	pod := id.Pod
	if id.UID != uid || *pod.RunAsUser != uid || *pod.RunAsGroup != uid || *pod.FSGroup != uid ||
		len(pod.SupplementalGroups) != 2 || pod.SupplementalGroups[0] != 2130706500 || pod.SupplementalGroups[1] != 2147000123 {
		t.Fatalf("unexpected private identity %+v", pod)
	}
	if !*pod.RunAsNonRoot || *pod.SupplementalGroupsPolicy != corev1.SupplementalGroupsPolicyStrict ||
		*pod.FSGroupChangePolicy != corev1.FSGroupChangeOnRootMismatch {
		t.Fatalf("static fields and forced fields missing: %+v", pod)
	}
	if id.Container != nil {
		t.Fatalf("a template without a container default resolves none: %+v", id.Container)
	}

	shared := &workspacev1alpha1.WorkspaceTemplateSpec{
		DefaultPodSecurityContext: &workspacev1alpha1.TemplatedPodSecurityContext{
			RunAsNonRoot: ptr.To(true), SupplementalGroupsPolicy: ptr.To(corev1.SupplementalGroupsPolicyStrict),
			RunAsUser: tpl(tplNamespaceGID), RunAsGroup: tpl(tplNamespaceGID),
		},
	}
	id, err = r.Resolve(ctx, shared, bob, "team-a", false)
	if err != nil {
		t.Fatal(err)
	}
	if id.UID != 0 || *id.Pod.RunAsUser != 2130706500 || id.Pod.FSGroup != nil {
		t.Fatalf("unexpected shared identity %+v", id)
	}
	list := &workspacev1alpha1.PosixIdentityList{}
	if err := c.List(ctx, list); err != nil || len(list.Items) != 1 {
		t.Fatalf("a shared template must not allocate: %d objects (%v)", len(list.Items), err)
	}
}
