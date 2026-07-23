/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// Repeated test literals, extracted to satisfy goconst.
const (
	testIntegrationName = "ray-integration"
	testRayClusterParam = "rayClusterName"
	testClusterAValue   = "c-a"
)

var _ = Describe("validateWorkspaceIntegrationParameters", func() {
	// A template declaring two required parameters.
	tmpl := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: testIntegrationName, Namespace: "ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
			Parameters: []workspacev1alpha1.IntegrationTemplateParameter{
				{Name: testRayClusterParam},
				{Name: "rayClusterNamespace"},
			},
		},
	}

	refWithParams := func(kv ...string) *workspacev1alpha1.IntegrationTemplateRef {
		ps := make([]workspacev1alpha1.IntegrationParameter, 0, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			ps = append(ps, workspacev1alpha1.IntegrationParameter{Name: kv[i], Value: kv[i+1]})
		}
		return &workspacev1alpha1.IntegrationTemplateRef{Name: testIntegrationName, Parameters: ps}
	}

	It("passes when every declared parameter is supplied", func() {
		ref := refWithParams(testRayClusterParam, testClusterAValue, "rayClusterNamespace", "ns")
		Expect(validateWorkspaceIntegrationParameters(ref, tmpl)).To(Succeed())
	})

	It("rejects a ref missing a declared parameter, naming it", func() {
		err := validateWorkspaceIntegrationParameters(refWithParams(testRayClusterParam, testClusterAValue), tmpl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("rayClusterNamespace"))
	})

	It("rejects when the ref supplies a superset but omits one declared parameter", func() {
		// Supplying an extra, undeclared parameter does not satisfy a missing declared one.
		err := validateWorkspaceIntegrationParameters(refWithParams(testRayClusterParam, testClusterAValue, "extra", "x"), tmpl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("rayClusterNamespace"))
	})

	It("passes when the template declares no parameters", func() {
		bare := &workspacev1alpha1.WorkspaceIntegrationTemplate{}
		Expect(validateWorkspaceIntegrationParameters(refWithParams(), bare)).To(Succeed())
	})
})

var _ = Describe("IntegrationTemplateRefValidator namespace scope", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	// tmplIn is a parameter-less "ray-integration" template in the given namespace.
	tmplIn := func(namespace string) *workspacev1alpha1.WorkspaceIntegrationTemplate {
		return &workspacev1alpha1.WorkspaceIntegrationTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testIntegrationName, Namespace: namespace},
		}
	}

	// newValidator builds a validator over an empty fake client. Namespace-scope rejection happens BEFORE
	// any template read, so the reject cases never consult the client. sharedNamespace is the configured
	// default (shared) namespace an integrationTemplateRef may additionally target.
	newValidator := func(sharedNamespace string) *IntegrationTemplateRefValidator {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		return NewIntegrationTemplateRefValidator(c, sharedNamespace)
	}

	// newValidatorWith builds a validator whose client is seeded with the given templates -- used by the
	// in-scope ALLOW cases, which pass the scope gate and then require the template to actually exist
	// (a missing template is now a rejection, matching the WorkspaceTemplate precedent).
	newValidatorWith := func(sharedNamespace string, objs ...runtime.Object) *IntegrationTemplateRefValidator {
		c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
		return NewIntegrationTemplateRefValidator(c, sharedNamespace)
	}

	// The workspace always lives in team-a; only the ref's target namespace varies across cases.
	wsWithIntegrationNamespace := func(refNamespace string) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
			Spec: workspacev1alpha1.WorkspaceSpec{
				IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{{
					Name:      testIntegrationName,
					Namespace: refNamespace,
				}},
			},
		}
	}

	It("rejects an integrationTemplateRef targeting another team's namespace", func() {
		ws := wsWithIntegrationNamespace("team-b")
		_, err := newValidator("jupyter-k8s-shared").Validate(ctx, ws)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("team-b"))
		Expect(err.Error()).To(ContainSubstring("team-a"))
		Expect(err.Error()).To(ContainSubstring("jupyter-k8s-shared"))
		Expect(err.Error()).To(ContainSubstring(testIntegrationName))
	})

	// These in-scope ALLOW cases seed the referenced template so they pass both the scope gate and the
	// existence check; the scope check is what they exercise (the template just has to exist).
	It("allows an integrationTemplateRef targeting the workspace's own namespace", func() {
		ws := wsWithIntegrationNamespace("team-a")
		_, err := newValidatorWith("jupyter-k8s-shared", tmplIn("team-a")).Validate(ctx, ws)
		Expect(err).NotTo(HaveOccurred())
	})

	It("allows an integrationTemplateRef targeting the shared namespace", func() {
		ws := wsWithIntegrationNamespace("jupyter-k8s-shared")
		_, err := newValidatorWith("jupyter-k8s-shared", tmplIn("jupyter-k8s-shared")).Validate(ctx, ws)
		Expect(err).NotTo(HaveOccurred())
	})

	It("allows an integrationTemplateRef with an empty namespace (defaults to the workspace namespace)", func() {
		ws := wsWithIntegrationNamespace("")
		_, err := newValidatorWith("jupyter-k8s-shared", tmplIn("team-a")).Validate(ctx, ws)
		Expect(err).NotTo(HaveOccurred())
	})

	It("admits an explicit shared-namespace ref when the template lives there", func() {
		// To use a shared-namespace template the user sets ref.Namespace to the shared namespace explicitly;
		// the read then targets exactly that namespace.
		ws := wsWithIntegrationNamespace("jupyter-k8s-shared")
		_, err := newValidatorWith("jupyter-k8s-shared", tmplIn("jupyter-k8s-shared")).Validate(ctx, ws)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects a bare-name ref whose template lives only in the shared namespace (no auto-fallback)", func() {
		// Empty ref namespace resolves to the workspace's own namespace only -- there is no own-ns ->
		// shared-ns fallback. A template present solely in the shared namespace is therefore not found, and
		// the message names the workspace namespace it looked in. The user must set the shared namespace
		// explicitly on the ref.
		ws := wsWithIntegrationNamespace("")
		_, err := newValidatorWith("jupyter-k8s-shared", tmplIn("jupyter-k8s-shared")).Validate(ctx, ws)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
		Expect(err.Error()).To(ContainSubstring("team-a"))
	})

	It("rejects a bare-name ref whose template is absent from the workspace namespace", func() {
		// Empty ref namespace + template nowhere: single read against the workspace namespace misses and the
		// ref is rejected, naming that namespace.
		ws := wsWithIntegrationNamespace("")
		_, err := newValidatorWith("jupyter-k8s-shared").Validate(ctx, ws)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("team-a"))
	})

	It("rejects a cross-namespace integrationTemplateRef even when no shared namespace is configured", func() {
		ws := wsWithIntegrationNamespace("team-b")
		_, err := newValidator("").Validate(ctx, ws)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("team-b"))
		Expect(err.Error()).To(ContainSubstring("team-a"))
	})
})

var _ = Describe("IntegrationTemplateRefValidator end-to-end (with a fetched template)", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	// declaredTemplate is a template in team-a declaring one required parameter.
	declaredTemplate := func() *workspacev1alpha1.WorkspaceIntegrationTemplate {
		return &workspacev1alpha1.WorkspaceIntegrationTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testIntegrationName, Namespace: "team-a"},
			Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
				Parameters: []workspacev1alpha1.IntegrationTemplateParameter{{Name: testRayClusterParam}},
			},
		}
	}

	// validatorWith builds a validator over a fake client seeded with the given objects.
	validatorWith := func(objs ...runtime.Object) *IntegrationTemplateRefValidator {
		c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
		return NewIntegrationTemplateRefValidator(c, "jupyter-k8s-shared")
	}

	wsRef := func(params ...workspacev1alpha1.IntegrationParameter) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
			Spec: workspacev1alpha1.WorkspaceSpec{
				IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{{Name: testIntegrationName, Parameters: params}},
			},
		}
	}

	It("passes when the referenced template exists and every declared parameter is supplied", func() {
		v := validatorWith(declaredTemplate())
		ws := wsRef(workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue})
		warnings, err := v.Validate(ctx, ws)
		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(BeEmpty())
	})

	It("rejects when the referenced template exists but a declared parameter is missing", func() {
		v := validatorWith(declaredTemplate())
		_, err := v.Validate(ctx, wsRef()) // supplies nothing
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(testRayClusterParam))
	})

	It("rejects when the referenced template does not exist", func() {
		v := validatorWith() // empty client -> template not found
		_, err := v.Validate(ctx, wsRef())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
		Expect(err.Error()).To(ContainSubstring(testIntegrationName))
	})

	It("warns about a supplied parameter the template does not declare", func() {
		v := validatorWith(declaredTemplate())
		ws := wsRef(
			workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue}, // declared, satisfied
			workspacev1alpha1.IntegrationParameter{Name: "rayClustrName", Value: "typo"},                // undeclared typo
		)
		warnings, err := v.Validate(ctx, ws)
		Expect(err).NotTo(HaveOccurred()) // undeclared params do not reject
		Expect(warnings).To(HaveLen(1))
		Expect(warnings[0]).To(ContainSubstring("rayClustrName"))
		Expect(warnings[0]).To(ContainSubstring("ignored"))
	})

	It("resolves an omitted ref namespace to the workspace namespace when finding the template", func() {
		// ref.Namespace unset -> defaults to the workspace ns (team-a), where the template lives.
		v := validatorWith(declaredTemplate())
		ws := wsRef(workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue})
		ws.Spec.IntegrationTemplateRefs[0].Namespace = ""
		warnings, err := v.Validate(ctx, ws)
		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(BeEmpty())
	})
})

var _ = Describe("integrationRefsChanged", func() {
	// integrationRefsChanged gates the update-path integration validation: it must run only when the
	// user changes the refs, so a controller-driven metadata update (or an admin's out-of-band template
	// edit) never re-triggers validation against a possibly-deleted template and wedges reconciliation.
	specWithRef := func(refs ...workspacev1alpha1.IntegrationTemplateRef) *workspacev1alpha1.WorkspaceSpec {
		return &workspacev1alpha1.WorkspaceSpec{IntegrationTemplateRefs: refs}
	}
	ref := func(params ...workspacev1alpha1.IntegrationParameter) workspacev1alpha1.IntegrationTemplateRef {
		return workspacev1alpha1.IntegrationTemplateRef{Name: testIntegrationName, Parameters: params}
	}

	It("reports no change for an identical ref set (controller metadata-only update)", func() {
		a := ref(workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue})
		Expect(integrationRefsChanged(specWithRef(a), specWithRef(a))).To(BeFalse())
	})

	It("treats nil and empty ref slices as unchanged (semantic equality)", func() {
		old := &workspacev1alpha1.WorkspaceSpec{IntegrationTemplateRefs: nil}
		new := &workspacev1alpha1.WorkspaceSpec{IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{}}
		Expect(integrationRefsChanged(old, new)).To(BeFalse())
	})

	It("reports a change when a ref is added", func() {
		Expect(integrationRefsChanged(specWithRef(), specWithRef(ref()))).To(BeTrue())
	})

	It("reports a change when a ref's parameters change", func() {
		old := ref(workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue})
		changed := ref(workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: "c-b"})
		Expect(integrationRefsChanged(specWithRef(old), specWithRef(changed))).To(BeTrue())
	})
})
