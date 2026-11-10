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
