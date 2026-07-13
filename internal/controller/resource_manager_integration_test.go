/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// This envtest exercises integration freeze/replay end to end against a real apiserver: create,
// freeze-on-drift, cluster switch (re-resolve), template edit (re-resolve), and fail-closed on a
// missing referenced resource. It drives EnsureDeploymentExists (the real controller entry point), so
// it covers the freeze-write, the replay-on-build, and the NeedsUpdate comparison together.

func startFreezeEnvtest(t *testing.T) (client.Client, func()) {
	t.Helper()
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		base := filepath.Join("..", "..", "bin", "k8s")
		entries, _ := os.ReadDir(base)
		for _, e := range entries {
			if e.IsDir() && (filepathContainsStr(e.Name(), "linux") || filepathContainsStr(e.Name(), "amd64")) {
				env.BinaryAssetsDirectory = filepath.Join(base, e.Name())
			}
		}
	}
	cfg, err := env.Start()
	if err != nil || cfg == nil {
		t.Skipf("envtest unavailable (%v)", err)
	}
	_ = workspacev1alpha1.AddToScheme(scheme.Scheme)
	_ = apiextensionsv1.AddToScheme(scheme.Scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		_ = env.Stop()
		t.Fatalf("client: %v", err)
	}
	if err := installRayClusterCRDForFreeze(c); err != nil {
		_ = env.Stop()
		t.Fatalf("install RayCluster CRD: %v", err)
	}
	return c, func() { _ = env.Stop() }
}

func filepathContainsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func installRayClusterCRDForFreeze(c client.Client) error {
	preserve := true
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "rayclusters.ray.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: rayGroup,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: rayClusterKind, ListKind: "RayClusterList", Plural: "rayclusters", Singular: "raycluster",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: "v1", Served: true, Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: &preserve},
				},
			}},
		},
	}
	ctx := context.Background()
	if err := c.Create(ctx, crd); err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		got := &apiextensionsv1.CustomResourceDefinition{}
		if err := c.Get(ctx, types.NamespacedName{Name: crd.Name}, got); err == nil {
			for _, cond := range got.Status.Conditions {
				if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
					return nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// makeRayCluster builds a RayCluster in testNamespace (every referenced cluster in these envtests
// lives in the workspace namespace, which is testNamespace).
func makeRayCluster(name, headSvc, gcs string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: rayGroup, Version: "v1", Kind: rayClusterKind})
	u.SetName(name)
	u.SetNamespace(testNamespace)
	_ = unstructured.SetNestedMap(u.Object, map[string]interface{}{
		"head":      map[string]interface{}{"serviceName": headSvc},
		"endpoints": map[string]interface{}{"gcs-server": gcs},
	}, "status")
	return u
}

func freezeTemplate(ns string) *workspacev1alpha1.WorkspaceIntegrationTemplate {
	return &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: rayIntegrationName, Namespace: ns},
		Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
			ShareProcessNamespace: boolPtr(true),
			ResourceRefs: []workspacev1alpha1.ResourceRef{{
				Name: rayClusterHandle, APIVersion: rayAPIVersion, Kind: rayClusterKind,
				Metadata: workspacev1alpha1.ResourceRefMetadata{
					Name:      rayClusterNameExpr,
					Namespace: "{{ .Parameters.rayClusterNamespace }}",
				},
			}},
			DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
				PodModifications: &workspacev1alpha1.PodModifications{
					AdditionalContainers: []corev1.Container{{
						Name:  raySidecarName,
						Image: "public.ecr.aws/sagemaker/sagemaker-distribution:latest-cpu",
						Args:  []string{`ray start --address={{ resource "rayCluster" "{.status.head.serviceName}" }}:{{ resource "rayCluster" "{.status.endpoints.gcs-server}" }} --block`},
					}},
					Volumes: []corev1.Volume{{Name: rayTmpVolume, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
		},
	}
}

func freezeWorkspace(name, ns, cluster string) *workspacev1alpha1.Workspace {
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: workspacev1alpha1.WorkspaceSpec{
			IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{{
				Name: rayIntegrationName,
				Parameters: []workspacev1alpha1.IntegrationParameter{
					{Name: rayClusterNameKey, Value: cluster},
					{Name: rayClusterNamespaceKey, Value: ns},
				},
			}},
		},
	}
	ws.Status.Conditions = []metav1.Condition{{
		Type: ConditionTypeAvailable, Status: metav1.ConditionTrue,
		Reason: testReasonReady, Message: testReadyMessage, LastTransitionTime: metav1.NewTime(time.Unix(0, 0)),
	}}
	return ws
}

func freezeResourceManager(c client.Client) *ResourceManager {
	opts := WorkspaceControllerOptions{ApplicationImagesPullPolicy: corev1.PullIfNotPresent}
	return NewResourceManager(
		c, c.Scheme(),
		NewDeploymentBuilder(c.Scheme(), opts, c),
		NewServiceBuilder(c.Scheme()),
		NewPVCBuilder(c.Scheme()),
		NewAccessResourcesBuilder(),
		NewStatusManager(c),
	)
}

func sidecarArgsOf(d *appsv1.Deployment) string {
	for _, cont := range d.Spec.Template.Spec.Containers {
		if cont.Name == raySidecarName && len(cont.Args) > 0 {
			return cont.Args[0]
		}
	}
	return ""
}

func hasSidecar(d *appsv1.Deployment) bool {
	for _, cont := range d.Spec.Template.Spec.Containers {
		if cont.Name == raySidecarName {
			return true
		}
	}
	return false
}

// freezeHarness bundles the fixed collaborators of the freeze end-to-end scenario so each phase can
// re-reconcile and assert on the resulting Deployment through small helpers (keeping the scenario body
// linear and readable).
type freezeHarness struct {
	t          *testing.T
	ctx        context.Context
	c          client.Client
	rm         *ResourceManager
	ws         *workspacev1alpha1.Workspace
	deployName string
	ns         string
}

// ensure re-runs the reconcile entry point (EnsureDeploymentExists) and returns the resulting
// Deployment, failing the test with the phase label on any error.
func (h *freezeHarness) ensure(label string) *appsv1.Deployment {
	h.t.Helper()
	if _, err := h.rm.EnsureDeploymentExists(h.ctx, h.ws, nil); err != nil {
		h.t.Fatalf("%s ensure: %v", label, err)
	}
	d := &appsv1.Deployment{}
	if err := h.c.Get(h.ctx, types.NamespacedName{Name: h.deployName, Namespace: h.ns}, d); err != nil {
		h.t.Fatalf("%s get deployment: %v", label, err)
	}
	return d
}

// assertSidecarArgs fails the test unless the injected sidecar's args contain want.
func (h *freezeHarness) assertSidecarArgs(d *appsv1.Deployment, want, label string) {
	h.t.Helper()
	if !filepathContainsStr(sidecarArgsOf(d), want) {
		h.t.Fatalf("%s: expected sidecar arg containing %q, got %q", label, want, sidecarArgsOf(d))
	}
}

// setClusterParams repoints the workspace's integration ref at the given cluster (in testNamespace).
func (h *freezeHarness) setClusterParams(cluster string) {
	h.ws.Spec.IntegrationTemplateRefs[0].Parameters = []workspacev1alpha1.IntegrationParameter{
		{Name: rayClusterNameKey, Value: cluster},
		{Name: rayClusterNamespaceKey, Value: h.ns},
	}
}

// parametersHash / templateVersion read the frozen record's two re-resolve triggers.
func (h *freezeHarness) parametersHash() string {
	return findResolvedIntegration(&h.ws.Status, rayIntegrationName).ParametersHash
}

func (h *freezeHarness) templateVersion() string {
	return findResolvedIntegration(&h.ws.Status, rayIntegrationName).ObservedIntegrationTemplateVersion
}

func TestIntegrationFreeze_EndToEnd(t *testing.T) {
	ctx := context.Background()
	c, stop := startFreezeEnvtest(t)
	defer stop()

	ns := testNamespace
	if err := c.Create(ctx, freezeTemplate(ns)); err != nil {
		t.Fatalf("create template: %v", err)
	}
	if err := c.Create(ctx, makeRayCluster("cluster-a", testSvcA, "6379")); err != nil {
		t.Fatalf("create cluster-a: %v", err)
	}
	if err := c.Create(ctx, makeRayCluster(testClusterB, "svc-b", "6380")); err != nil {
		t.Fatalf("create cluster-b: %v", err)
	}

	rm := freezeResourceManager(c)

	// Persist the workspace first so the builder's controller owner reference has a UID.
	ws := freezeWorkspace("f-ws", ns, "cluster-a")
	wsForCreate := ws.DeepCopy()
	if err := c.Create(ctx, wsForCreate); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	ws.ObjectMeta = wsForCreate.ObjectMeta
	// Re-stamp Available (Create strips status; the freeze update path requires it).
	ws.Status = wsForCreate.Status
	ws.Status.Conditions = []metav1.Condition{{
		Type: ConditionTypeAvailable, Status: metav1.ConditionTrue, Reason: testReasonReady, Message: testReadyMessage,
		LastTransitionTime: metav1.NewTime(time.Unix(0, 0)),
	}}
	_ = c.Status().Update(ctx, ws)

	h := &freezeHarness{t: t, ctx: ctx, c: c, rm: rm, ws: ws, deployName: GenerateDeploymentName("f-ws"), ns: ns}

	// ---- CREATE: freeze cluster-a values and inject the sidecar pointing at svc-a. ----
	d := h.ensure("CREATE")
	if !hasSidecar(d) {
		t.Fatalf("CREATE: expected an injected sidecar, got containers=%d", len(d.Spec.Template.Spec.Containers))
	}
	h.assertSidecarArgs(d, "svc-a:6379", "CREATE")
	// The template's shareProcessNamespace must be OR-reduced onto the pod (the workspace container has
	// to share a PID namespace with the injected Ray sidecar).
	if sp := d.Spec.Template.Spec.ShareProcessNamespace; sp == nil || !*sp {
		t.Fatalf("CREATE: expected pod ShareProcessNamespace=true from the template, got %v", sp)
	}
	// Frozen values recorded in status.
	if fr := findResolvedIntegration(&ws.Status, rayIntegrationName); fr == nil || len(fr.Values) != 2 {
		t.Fatalf("CREATE: expected 2 frozen values, got %+v", fr)
	}
	tokenA := h.parametersHash()
	tmplVerA := h.templateVersion()
	if tmplVerA == "" {
		t.Fatalf("CREATE: expected a non-empty observedIntegrationTemplateVersion")
	}
	t.Logf("CREATE PASS: sidecar=%q parametersHash=%s templateVersion=%s", sidecarArgsOf(d), tokenA, tmplVerA)

	// ---- DRIFT: mutate cluster-a's status. Token unchanged -> replay -> sidecar stays svc-a. ----
	clA := makeRayCluster("cluster-a", "svc-a-DRIFTED", "9999")
	clA.SetResourceVersion(rvOf(t, c, "cluster-a", ns))
	if err := c.Update(ctx, clA); err != nil {
		t.Fatalf("drift update: %v", err)
	}
	d = h.ensure("DRIFT")
	h.assertSidecarArgs(d, "svc-a:6379", "DRIFT (expected FROZEN svc-a:6379, drift ignored)")
	t.Logf("DRIFT PASS: sidecar frozen at %q despite cluster-a drift", sidecarArgsOf(d))

	// ---- SWITCH: point at cluster-b. Token changes -> re-resolve -> sidecar svc-b, one sidecar. ----
	h.setClusterParams(testClusterB)
	d = h.ensure("SWITCH")
	h.assertSidecarArgs(d, "svc-b:6380", "SWITCH")
	count := 0
	for _, cont := range d.Spec.Template.Spec.Containers {
		if cont.Name == raySidecarName {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("SWITCH: expected exactly 1 sidecar, got %d", count)
	}
	tokenB := h.parametersHash()
	if tokenB == tokenA {
		t.Fatalf("SWITCH: token must change, stayed %s", tokenB)
	}
	t.Logf("SWITCH PASS: sidecar=%q token %s -> %s", sidecarArgsOf(d), tokenA, tokenB)

	// ---- TEMPLATE-EDIT: params unchanged (still cluster-b), but the admin edits the template (bumps
	// Generation and changes the sidecar arg). observedIntegrationTemplateVersion changes -> re-resolve
	// -> the new arg is rendered, even though parametersHash is identical. This exercises the second
	// re-resolve trigger independently of the parameters trigger; a template edit re-renders the pod
	// (and rolls it), same as a parameter change. ----
	// Re-assert cluster-b params explicitly so this block is self-contained regardless of prior steps.
	h.setClusterParams(testClusterB)
	paramsHashBeforeEdit := h.parametersHash()
	tmplVerBeforeEdit := h.templateVersion()
	tmpl := &workspacev1alpha1.WorkspaceIntegrationTemplate{}
	if err := c.Get(ctx, types.NamespacedName{Name: rayIntegrationName, Namespace: ns}, tmpl); err != nil {
		t.Fatalf("get template for edit: %v", err)
	}
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].Args = []string{
		`ray start --address={{ resource "rayCluster" "{.status.head.serviceName}" }}:{{ resource "rayCluster" "{.status.endpoints.gcs-server}" }} --EDITED --block`,
	}
	if err := c.Update(ctx, tmpl); err != nil {
		t.Fatalf("template edit update: %v", err)
	}
	d = h.ensure("TEMPLATE-EDIT")
	h.assertSidecarArgs(d, "--EDITED", "TEMPLATE-EDIT (expected edited sidecar arg)")
	// Still cluster-b (params unchanged), so the re-resolved head service must be cluster-b's.
	h.assertSidecarArgs(d, "svc-b", "TEMPLATE-EDIT (params unchanged -> svc-b head service)")
	tmplVerAfterEdit := h.templateVersion()
	if tmplVerAfterEdit == tmplVerBeforeEdit {
		t.Fatalf("TEMPLATE-EDIT: observedIntegrationTemplateVersion must change on a template edit, stayed %s", tmplVerAfterEdit)
	}
	if got := h.parametersHash(); got != paramsHashBeforeEdit {
		t.Fatalf("TEMPLATE-EDIT: parametersHash must be unchanged (params identical), was %s now %s", paramsHashBeforeEdit, got)
	}
	t.Logf("TEMPLATE-EDIT PASS: re-resolved on template edit; templateVersion %s -> %s, parametersHash unchanged=%s",
		tmplVerBeforeEdit, tmplVerAfterEdit, paramsHashBeforeEdit)

	// ---- FAIL-CLOSED: switch to a non-existent cluster. Capture fails -> preserve svc-b. ----
	h.setClusterParams("cluster-missing")
	d = h.ensure("FAIL-CLOSED (must not surface an error to caller)")
	h.assertSidecarArgs(d, "svc-b:6380", "FAIL-CLOSED (missing cluster must PRESERVE svc-b sidecar)")
	if got := h.parametersHash(); got != tokenB {
		t.Fatalf("FAIL-CLOSED: token must stay %s (not advance to the failed input), got %s", tokenB, got)
	}
	t.Logf("FAIL-CLOSED PASS: sidecar preserved at %q; token unchanged=%s", sidecarArgsOf(d), tokenB)

	// ---- REPLAY-FAIL-CLOSED: delete the template so replay itself fails. The running (frozen) svc-b
	// sidecar must be PRESERVED, not stripped. This guards the Pass-1 fix: an abort on replay error
	// leaves the running Deployment untouched rather than rebuilding a base-only (sidecar-less) spec.
	// Restore the ref to the still-frozen cluster-b so token == frozen (pure replay path).
	h.setClusterParams(testClusterB)
	tmpl = &workspacev1alpha1.WorkspaceIntegrationTemplate{}
	_ = c.Get(ctx, types.NamespacedName{Name: rayIntegrationName, Namespace: ns}, tmpl)
	if err := c.Delete(ctx, tmpl); err != nil {
		t.Fatalf("delete template: %v", err)
	}
	d = h.ensure("REPLAY-FAIL-CLOSED (must not surface an error to caller)")
	h.assertSidecarArgs(d, "svc-b:6380", "REPLAY-FAIL-CLOSED (deleted template must PRESERVE running svc-b sidecar)")
	t.Logf("REPLAY-FAIL-CLOSED PASS: sidecar preserved at %q despite template deletion", sidecarArgsOf(d))
}

// TestIntegrationFreeze_SharedNamespaceStampedRef verifies that a WorkspaceIntegrationTemplate published
// only in the shared namespace is resolved for a workspace in a different namespace whose ref carries the
// shared namespace in integrationTemplateRefs[].namespace. The mutating webhook (IntegrationRefDefaulter)
// stamps that namespace at admission, so this test seeds the ref the same way and asserts the controller
// reads the template from the stamped namespace.
func TestIntegrationFreeze_SharedNamespaceStampedRef(t *testing.T) {
	ctx := context.Background()
	c, stop := startFreezeEnvtest(t)
	defer stop()

	const sharedNS = "jupyter-k8s-shared"
	wsNS := testNamespace

	// Create the shared namespace and publish the template ONLY there (not in the workspace ns).
	if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sharedNS}}); err != nil {
		t.Fatalf("create shared namespace: %v", err)
	}
	if err := c.Create(ctx, freezeTemplate(sharedNS)); err != nil {
		t.Fatalf("create shared-namespace template: %v", err)
	}
	// The referenced RayCluster lives in the workspace namespace (resource ns defaults to workspace ns).
	if err := c.Create(ctx, makeRayCluster("shared-cluster", "svc-shared", "6379")); err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	rm := freezeResourceManager(c)
	deployName := GenerateDeploymentName("shared-ws")

	// Workspace in the default namespace references the shared-namespace template with the namespace set
	// on the ref, matching what the mutating webhook stamps at admission.
	ws := freezeWorkspace("shared-ws", wsNS, "shared-cluster")
	ws.Spec.IntegrationTemplateRefs[0].Namespace = sharedNS
	wsForCreate := ws.DeepCopy()
	if err := c.Create(ctx, wsForCreate); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	ws.ObjectMeta = wsForCreate.ObjectMeta
	ws.Status = wsForCreate.Status
	ws.Status.Conditions = []metav1.Condition{{
		Type: ConditionTypeAvailable, Status: metav1.ConditionTrue, Reason: testReasonReady, Message: testReadyMessage,
		LastTransitionTime: metav1.NewTime(time.Unix(0, 0)),
	}}
	_ = c.Status().Update(ctx, ws)

	if _, err := rm.EnsureDeploymentExists(ctx, ws, nil); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	d := &appsv1.Deployment{}
	if err := c.Get(ctx, types.NamespacedName{Name: deployName, Namespace: wsNS}, d); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if !hasSidecar(d) || !filepathContainsStr(sidecarArgsOf(d), "svc-shared:6379") {
		t.Fatalf("SHARED-NS STAMPED: expected sidecar resolved from the shared-namespace template targeting svc-shared:6379, got args=%q", sidecarArgsOf(d))
	}
	if fr := findResolvedIntegration(&ws.Status, rayIntegrationName); fr == nil {
		t.Fatalf("SHARED-NS STAMPED: expected a frozen record from the shared-namespace template")
	}
	t.Logf("SHARED-NS STAMPED PASS: template in %q resolved for workspace in %q via stamped ref namespace; sidecar=%q", sharedNS, wsNS, sidecarArgsOf(d))
}

func rvOf(t *testing.T, c client.Client, name, ns string) string {
	t.Helper()
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: rayGroup, Version: "v1", Kind: rayClusterKind})
	if err := c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, u); err != nil {
		t.Fatalf("get rv %s: %v", name, err)
	}
	return u.GetResourceVersion()
}
