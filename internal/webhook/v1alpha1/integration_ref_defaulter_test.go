/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("IntegrationRefDefaulter", func() {
	const sharedNS = "jupyter-k8s-shared"

	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	// tmplIn is a "ray-integration" template in the given namespace.
	tmplIn := func(namespace string) *workspacev1alpha1.WorkspaceIntegrationTemplate {
		return &workspacev1alpha1.WorkspaceIntegrationTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testIntegrationName, Namespace: namespace},
		}
	}

	// defaulterWith builds a defaulter over a fake client seeded with the given objects.
	defaulterWith := func(objs ...runtime.Object) *IntegrationRefDefaulter {
		c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
		return NewIntegrationRefDefaulter(c, sharedNS)
	}

	// wsRef builds a team-a workspace with a single integrationTemplateRef carrying the given namespace.
	wsRef := func(refNamespace string) *workspacev1alpha1.Workspace {
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

	It("is a no-op when the workspace has no integrationTemplateRefs", func() {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
		}
		Expect(defaulterWith().ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		Expect(ws.Spec.IntegrationTemplateRefs).To(BeEmpty())
	})

	It("stamps the workspace's own namespace when the template lives there", func() {
		ws := wsRef("") // omitted namespace
		d := defaulterWith(tmplIn(testNamespaceTeamA))
		Expect(d.ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(Equal(testNamespaceTeamA))
	})

	It("falls back to and stamps the shared namespace when the template lives only there", func() {
		ws := wsRef("") // omitted namespace
		d := defaulterWith(tmplIn(sharedNS))
		Expect(d.ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(Equal(sharedNS))
	})

	It("prefers the workspace's own namespace over the shared one when the template exists in both", func() {
		// A same-named template in the user's own namespace shadows the shared one (own-ns tried first).
		ws := wsRef("")
		d := defaulterWith(tmplIn(testNamespaceTeamA), tmplIn(sharedNS))
		Expect(d.ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(Equal(testNamespaceTeamA))
	})

	It("leaves the ref unstamped when the template is absent from both namespaces", func() {
		// Not found anywhere -> stays empty so the validating webhook emits the authoritative rejection.
		ws := wsRef("")
		d := defaulterWith() // empty client
		Expect(d.ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(BeEmpty())
	})

	It("does not touch a ref that already carries an explicit namespace", func() {
		// Even if the explicit namespace is the workspace's own, the defaulter must not re-resolve it;
		// scope enforcement on explicit namespaces is the validating webhook's job.
		ws := wsRef(testNamespaceTeamA)
		d := defaulterWith(tmplIn(sharedNS)) // shared has it, own does not -- must NOT get rewritten to shared
		Expect(d.ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(Equal(testNamespaceTeamA))
	})

	It("is idempotent: a second pass over an already-stamped ref does not re-resolve", func() {
		// After the first stamp the ref carries a namespace, so a re-admission (e.g. a controller metadata
		// update) skips resolution entirely -- which is what prevents a since-deleted template from wedging.
		ws := wsRef("")
		d := defaulterWith(tmplIn(testNamespaceTeamA))
		Expect(d.ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		stamped := ws.Spec.IntegrationTemplateRefs[0].Namespace
		Expect(stamped).To(Equal(testNamespaceTeamA))

		// Second pass with an EMPTY client: if it re-resolved it would fail to find the template and clear
		// nothing (it never clears), but crucially it must not error and must leave the stamp intact.
		d2 := NewIntegrationRefDefaulter(fake.NewClientBuilder().WithScheme(scheme).Build(), sharedNS)
		Expect(d2.ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(Equal(stamped))
	})

	It("stamps each of multiple refs independently", func() {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
			Spec: workspacev1alpha1.WorkspaceSpec{
				IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{
					{Name: testIntegrationName},                        // own ns
					{Name: "ray-integration-shared"},                   // shared ns
					{Name: "ray-integration-explicit", Namespace: "x"}, // untouched
				},
			},
		}
		d := defaulterWith(
			tmplIn(testNamespaceTeamA),
			&workspacev1alpha1.WorkspaceIntegrationTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "ray-integration-shared", Namespace: sharedNS},
			},
		)
		Expect(d.ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(Equal(testNamespaceTeamA))
		Expect(ws.Spec.IntegrationTemplateRefs[1].Namespace).To(Equal(sharedNS))
		Expect(ws.Spec.IntegrationTemplateRefs[2].Namespace).To(Equal("x")) // explicit, unchanged
	})

	It("returns a transient (non-NotFound) read error instead of silently leaving the ref unstamped", func() {
		// A fail-closed mutating webhook must surface an API error rather than store an unresolved ref.
		failing := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return apierrors.NewInternalError(assertErr("boom"))
			},
		}).Build()
		d := NewIntegrationRefDefaulter(failing, sharedNS)
		err := d.ApplyIntegrationRefDefaults(ctx, wsRef(""))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(testIntegrationName))
	})

	It("does not attempt a shared-namespace fallback when no shared namespace is configured", func() {
		// With sharedNamespace unset, only the workspace's own namespace is consulted; a template that
		// exists only in some other namespace is not found, and the ref is left unstamped.
		ws := wsRef("")
		c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tmplIn(sharedNS)).Build()
		d := NewIntegrationRefDefaulter(c, "")
		Expect(d.ApplyIntegrationRefDefaults(ctx, ws)).To(Succeed())
		Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(BeEmpty())
	})
})

// assertErr is a tiny error type so the interceptor can build an InternalError with a message we assert on.
type assertErr string

func (e assertErr) Error() string { return string(e) }
