/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

func TestRequireHardening(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)
	r := NewResolver(NewAllocator(c, c), c)
	for name, sc := range map[string]*workspacev1alpha1.TemplatedPodSecurityContext{
		"neither":         {RunAsUser: tpl(tplUserUID)},
		"no runAsNonRoot": {RunAsUser: tpl(tplUserUID), SupplementalGroupsPolicy: ptr.To(corev1.SupplementalGroupsPolicyStrict)},
		"Merge policy":    {RunAsUser: tpl(tplUserUID), RunAsNonRoot: ptr.To(true), SupplementalGroupsPolicy: ptr.To(corev1.SupplementalGroupsPolicyMerge)},
	} {
		spec := &workspacev1alpha1.WorkspaceTemplateSpec{DefaultPodSecurityContext: sc}
		if err := ValidateTemplates(spec); err == nil {
			t.Errorf("%s: the template webhook must reject it", name)
		}
		if _, err := r.Resolve(ctx, spec, alice, "team-a", false); err == nil {
			t.Errorf("%s: resolution must reject it", name)
		}
	}
	list := &workspacev1alpha1.PosixIdentityList{}
	if err := c.List(ctx, list); err != nil || len(list.Items) != 0 {
		t.Fatalf("a rejected template must not allocate: %d objects (%v)", len(list.Items), err)
	}
	static := &workspacev1alpha1.WorkspaceTemplateSpec{DefaultPodSecurityContext: &workspacev1alpha1.TemplatedPodSecurityContext{
		RunAsUser: ptr.To(intstr.FromInt32(1000)),
	}}
	if err := RequireHardening(static); err != nil {
		t.Errorf("a template without identity templates is not constrained: %v", err)
	}
}

func TestResolveRejectsInvalidValues(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)
	r := NewResolver(NewAllocator(c, c), c)
	for _, text := range []string{"0", "65534", "999", "1048576", "1074790400", "2147483648", tplNamespaceGID} {
		spec := &workspacev1alpha1.WorkspaceTemplateSpec{
			DefaultPodSecurityContext: hardened(&workspacev1alpha1.TemplatedPodSecurityContext{RunAsUser: tpl(text)}),
		}
		if _, err := r.Resolve(ctx, spec, alice, "team-a", false); err == nil {
			t.Errorf("%q: want an error", text)
		}
	}
}

func TestValidateTemplates(t *testing.T) {
	valid := &workspacev1alpha1.WorkspaceTemplateSpec{
		DefaultPodSecurityContext: &workspacev1alpha1.TemplatedPodSecurityContext{
			RunAsNonRoot: ptr.To(true), SupplementalGroupsPolicy: ptr.To(corev1.SupplementalGroupsPolicyStrict),
			RunAsUser:          tpl(tplUserUID),
			SupplementalGroups: []intstr.IntOrString{*tpl(tplNamespaceGID), intstr.FromInt32(2147000123)},
		},
		DefaultContainerSecurityContext: &workspacev1alpha1.TemplatedSecurityContext{RunAsGroup: tpl(tplUserGID)},
	}
	if err := ValidateTemplates(valid); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"{{ .User.Name }}", "{{ .Workspace.Name }}", "{{ .User.Uid", "abc", "{{ .User.Uid }}-1", ""} {
		spec := &workspacev1alpha1.WorkspaceTemplateSpec{
			DefaultPodSecurityContext: hardened(&workspacev1alpha1.TemplatedPodSecurityContext{RunAsUser: tpl(text)}),
		}
		if err := ValidateTemplates(spec); err == nil {
			t.Errorf("%q: want an error", text)
		}
	}
}

func TestIntegerIDsBesideIdentityTemplates(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)
	r := NewResolver(NewAllocator(c, c), c)
	hardened := func(groups ...intstr.IntOrString) *workspacev1alpha1.WorkspaceTemplateSpec {
		return &workspacev1alpha1.WorkspaceTemplateSpec{DefaultPodSecurityContext: &workspacev1alpha1.TemplatedPodSecurityContext{
			RunAsNonRoot: ptr.To(true), SupplementalGroupsPolicy: ptr.To(corev1.SupplementalGroupsPolicyStrict),
			RunAsUser: tpl(tplNamespaceGID), SupplementalGroups: groups,
		}}
	}
	for _, id := range []int32{0, int32(UserRangeMin) + 24, int32(GroupRangeMin), int32(NobodyID)} {
		spec := hardened(*tpl(tplNamespaceGID), intstr.FromInt32(id))
		if err := ValidateTemplates(spec); err == nil {
			t.Errorf("%d: the template webhook must reject it", id)
		}
		if _, err := r.Resolve(ctx, spec, alice, "team-a", false); err == nil {
			t.Errorf("%d: resolution must reject it", id)
		}
	}
	if err := ValidateTemplates(hardened(*tpl(tplNamespaceGID), intstr.FromInt32(2147000123))); err != nil {
		t.Errorf("a volume gid outside the operator ranges must pass: %v", err)
	}
	// Without identity templates, integers keep their meaning from before: 0 is accepted.
	plain := &workspacev1alpha1.WorkspaceTemplateSpec{DefaultPodSecurityContext: &workspacev1alpha1.TemplatedPodSecurityContext{
		SupplementalGroups: []intstr.IntOrString{intstr.FromInt32(0)},
	}}
	if err := ValidateTemplates(plain); err != nil {
		t.Errorf("a template without identity templates must not be checked: %v", err)
	}
}

func TestReferencesUser(t *testing.T) {
	pod := func(user intstr.IntOrString) *workspacev1alpha1.WorkspaceTemplateSpec {
		return &workspacev1alpha1.WorkspaceTemplateSpec{DefaultPodSecurityContext: &workspacev1alpha1.TemplatedPodSecurityContext{
			RunAsUser: &user,
		}}
	}
	for text, want := range map[string]bool{
		tplUserUID:      true,
		tplUserGID:      true,
		tplNamespaceGID: false,
		`{{ if false }}{{ .User.Uid }}{{ else }}{{ .Namespace.Gid }}{{ end }}`: false,
		`{{ .User.Name }}`: true,
	} {
		if got := ReferencesUser(pod(intstr.FromString(text))); got != want {
			t.Errorf("%s: ReferencesUser = %t, want %t", text, got, want)
		}
	}
	if ReferencesUser(pod(intstr.FromInt32(2147000123))) {
		t.Error("a template without identity templates references nothing")
	}
	container := &workspacev1alpha1.WorkspaceTemplateSpec{DefaultContainerSecurityContext: &workspacev1alpha1.TemplatedSecurityContext{
		RunAsUser: tpl(tplUserUID),
	}}
	if !ReferencesUser(container) {
		t.Error("a container default referencing .User counts")
	}
}
