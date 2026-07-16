/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"strings"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Repeated test literals, extracted to satisfy goconst. rayClusterKind/rayClusterHandle name the
// referenced resource's Kind and its template handle; the rest are fixture values reused across specs.
const (
	rayClusterKind    = "RayCluster"
	rayClusterHandle  = "rayCluster"
	testUser          = "alice"
	testClusterName   = "demo"
	testHeadSvc       = "demo-head-svc"
	tagParamExpr      = "{{ .Parameters.tag }}"
	testTeamNamespace = "team-a"
	rayClusterNameKey = "rayClusterName"
)

// rayClusterFixture builds an unstructured RayCluster carrying just the status fields the resolver
// reads via {{ resource ... }}: a head serviceName (testHeadSvc) and a gcs-server endpoint. Both are
// fixed -- every spec resolves the same fixture and asserts against these values.
func rayClusterFixture() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "ray.io", Version: "v1", Kind: rayClusterKind})
	u.SetName(testClusterName)
	u.SetNamespace("ns")
	_ = unstructured.SetNestedMap(u.Object, map[string]interface{}{
		"head":      map[string]interface{}{"serviceName": testHeadSvc},
		"endpoints": map[string]interface{}{"gcs-server": "6379"},
	}, "status")
	return u
}

func TestCaptureKey_CombinesIDAndPath(t *testing.T) {
	got := CaptureKey(rayClusterHandle, "{.status.head.serviceName}")
	want := "rayCluster|{.status.head.serviceName}"
	if got != want {
		t.Fatalf("CaptureKey = %q, want %q", got, want)
	}
}

func TestResolveTemplateExpression_WorkspaceAndParameters(t *testing.T) {
	r := NewIntegrationTemplateResolver(nil) // no {{ resource }} usage -> provider not needed
	data := IntegrationTemplateData{
		Workspace:  IntegrationWorkspaceData{Name: testUser, Namespace: testTeamNamespace},
		Parameters: map[string]string{rayClusterNameKey: testClusterName},
	}
	out, err := r.ResolveTemplateExpression("{{ .Workspace.Namespace }}/{{ .Workspace.Name }}:{{ .Parameters.rayClusterName }}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "team-a/alice:demo" {
		t.Fatalf("ResolveTemplateExpression = %q, want %q", out, "team-a/alice:demo")
	}
}

func TestResolveTemplateExpression_MissingParameterIsError(t *testing.T) {
	r := NewIntegrationTemplateResolver(nil)
	data := IntegrationTemplateData{Parameters: map[string]string{}}
	if _, err := r.ResolveTemplateExpression("{{ .Parameters.absent }}", data); err == nil {
		t.Fatal("expected an error for a missing parameter key (missingkey=error), got nil")
	}
}

func TestResolveTemplateExpression_LiveCaptureRecordsValues(t *testing.T) {
	live := NewLiveResourceValueProvider(map[string]*unstructured.Unstructured{
		rayClusterHandle: rayClusterFixture(),
	})
	r := NewIntegrationTemplateResolver(live)
	data := IntegrationTemplateData{}

	out, err := r.ResolveTemplateExpression(`addr={{ resource "rayCluster" "{.status.head.serviceName}" }}:{{ resource "rayCluster" "{.status.endpoints.gcs-server}" }}`, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "addr=demo-head-svc:6379" {
		t.Fatalf("ResolveTemplateExpression = %q, want %q", out, "addr=demo-head-svc:6379")
	}

	// Every {{ resource }} read must be recorded under CaptureKey so it can be frozen + replayed.
	captured := live.Captured()
	if v := captured[CaptureKey(rayClusterHandle, "{.status.head.serviceName}")]; v != testHeadSvc {
		t.Fatalf("captured head serviceName = %q, want demo-head-svc", v)
	}
	if v := captured[CaptureKey(rayClusterHandle, "{.status.endpoints.gcs-server}")]; v != "6379" {
		t.Fatalf("captured gcs-server = %q, want 6379", v)
	}
}

func TestResolveTemplateExpression_LiveCaptureUnknownIDIsFailClosed(t *testing.T) {
	live := NewLiveResourceValueProvider(map[string]*unstructured.Unstructured{})
	r := NewIntegrationTemplateResolver(live)
	if _, err := r.ResolveTemplateExpression(`{{ resource "missing" "{.status.head.serviceName}" }}`, IntegrationTemplateData{}); err == nil {
		t.Fatal("expected fail-closed error for an undeclared resource id, got nil")
	}
}

func TestResolveTemplateExpression_FrozenReplaySameOutputNoResourceRead(t *testing.T) {
	// A frozen provider seeded with the captured values must reproduce the same rendered string
	// WITHOUT any resource being available -- this is what makes an unchanged-token reconcile
	// drift-proof.
	frozen := NewFrozenResourceValueProvider(map[string]string{
		CaptureKey(rayClusterHandle, "{.status.head.serviceName}"):     testHeadSvc,
		CaptureKey(rayClusterHandle, "{.status.endpoints.gcs-server}"): "6379",
	})
	r := NewIntegrationTemplateResolver(frozen)
	out, err := r.ResolveTemplateExpression(`addr={{ resource "rayCluster" "{.status.head.serviceName}" }}:{{ resource "rayCluster" "{.status.endpoints.gcs-server}" }}`, IntegrationTemplateData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "addr=demo-head-svc:6379" {
		t.Fatalf("frozen replay = %q, want %q", out, "addr=demo-head-svc:6379")
	}
}

func TestResolveTemplateExpression_FrozenReplayMissingKeyIsError(t *testing.T) {
	// A template that references an expression absent from the frozen set must hard-error, forcing a
	// re-resolve (token change) rather than rendering an incomplete pod spec.
	frozen := NewFrozenResourceValueProvider(map[string]string{})
	r := NewIntegrationTemplateResolver(frozen)
	_, err := r.ResolveTemplateExpression(`{{ resource "rayCluster" "{.status.head.serviceName}" }}`, IntegrationTemplateData{})
	if err == nil {
		t.Fatal("expected an error for a frozen key miss, got nil")
	}
	if !strings.Contains(err.Error(), "frozen resolution has no value") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestResolveResourceRef_TemplatedNameAndNamespace(t *testing.T) {
	r := NewIntegrationTemplateResolver(nil)
	ref := &workspacev1alpha1.ResourceRef{
		Name: rayClusterHandle, APIVersion: "ray.io/v1", Kind: rayClusterKind,
		Metadata: workspacev1alpha1.ResourceRefMetadata{
			Name:      "{{ .Parameters.rayClusterName }}",
			Namespace: "{{ .Workspace.Namespace }}",
		},
	}
	data := IntegrationTemplateData{
		Workspace:  IntegrationWorkspaceData{Namespace: testTeamNamespace},
		Parameters: map[string]string{rayClusterNameKey: testClusterName},
	}
	name, ns, err := r.ResolveResourceRef(ref, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != testClusterName || ns != testTeamNamespace {
		t.Fatalf("ResolveResourceRef = (%q,%q), want (demo,team-a)", name, ns)
	}
}

func TestResolveResourceRef_EmptyResolvedNameIsError(t *testing.T) {
	r := NewIntegrationTemplateResolver(nil)
	ref := &workspacev1alpha1.ResourceRef{
		Name: rayClusterHandle, APIVersion: "ray.io/v1", Kind: rayClusterKind,
		Metadata: workspacev1alpha1.ResourceRefMetadata{
			Name:      "{{ .Parameters.rayClusterName }}",
			Namespace: "ns",
		},
	}
	// Parameter resolves to empty -> defense-in-depth empty check must fail.
	data := IntegrationTemplateData{Parameters: map[string]string{rayClusterNameKey: ""}}
	if _, _, err := r.ResolveResourceRef(ref, data); err == nil {
		t.Fatal("expected an error when the resolved name is empty, got nil")
	}
}

// podModsFixture builds a PodModifications exercising every templatable field the resolver contract
// covers: additional + init container image/command/args/workingDir/env/volumeMounts, primary-container
// mergeEnv + volumeMounts, plus a pod-level Volume whose name LOOKS templated (to prove volumes are
// left untouched).
func podModsFixture() *workspacev1alpha1.PodModifications {
	return &workspacev1alpha1.PodModifications{
		AdditionalContainers: []corev1.Container{{
			Name:       "ray-sidecar",
			Image:      "img:{{ .Parameters.tag }}",
			Command:    []string{"start", "{{ .Workspace.Name }}"},
			Args:       []string{"--addr={{ resource \"rayCluster\" \"{.status.head.serviceName}\" }}"},
			WorkingDir: "/work/{{ .Workspace.Namespace }}",
			Env:        []corev1.EnvVar{{Name: "NS", Value: "{{ .Workspace.Namespace }}"}},
			VolumeMounts: []corev1.VolumeMount{{
				Name: "data", MountPath: "/data/{{ .Workspace.Name }}", SubPath: tagParamExpr,
			}},
		}},
		InitContainers: []corev1.Container{{
			Name: "init", Image: "busybox", Args: []string{"echo {{ .Workspace.Name }}"},
		}},
		Volumes: []corev1.Volume{{
			// Intentionally template-looking NAME: the resolver must NOT touch pod-level volumes.
			Name: tagParamExpr,
		}},
		PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
			MergeEnv: []workspacev1alpha1.AccessEnvTemplate{{
				Name: "RAY_ADDRESS", ValueTemplate: "{{ resource \"rayCluster\" \"{.status.head.serviceName}\" }}",
			}},
			VolumeMounts: []corev1.VolumeMount{{
				Name: "shared", MountPath: "/shared/{{ .Workspace.Name }}",
			}},
		},
	}
}

func podModsData() IntegrationTemplateData {
	return IntegrationTemplateData{
		Workspace:  IntegrationWorkspaceData{Name: testUser, Namespace: testTeamNamespace},
		Parameters: map[string]string{"tag": "v1"},
	}
}

func podModsResolver() *IntegrationTemplateResolver {
	return NewIntegrationTemplateResolver(NewLiveResourceValueProvider(map[string]*unstructured.Unstructured{
		rayClusterHandle: rayClusterFixture(),
	}))
}

// TestResolvePodModifications_ResolvesAllTemplatableFields exercises the public entry point end to end
// and asserts each field in the resolved-field contract is rendered.
func TestResolvePodModifications_ResolvesAllTemplatableFields(t *testing.T) {
	out, err := podModsResolver().ResolvePodModifications(podModsFixture(), podModsData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c := out.AdditionalContainers[0]
	if c.Image != "img:v1" {
		t.Fatalf("image = %q, want img:v1", c.Image)
	}
	if c.Command[1] != testUser {
		t.Fatalf("command[1] = %q, want alice", c.Command[1])
	}
	if c.Args[0] != "--addr=demo-head-svc" {
		t.Fatalf("args[0] = %q, want --addr=demo-head-svc", c.Args[0])
	}
	if c.WorkingDir != "/work/team-a" {
		t.Fatalf("workingDir = %q, want /work/team-a", c.WorkingDir)
	}
	if c.Env[0].Value != testTeamNamespace {
		t.Fatalf("env[NS] = %q, want team-a", c.Env[0].Value)
	}
	if c.VolumeMounts[0].MountPath != "/data/alice" || c.VolumeMounts[0].SubPath != "v1" {
		t.Fatalf("volumeMount = (%q,%q), want (/data/alice,v1)", c.VolumeMounts[0].MountPath, c.VolumeMounts[0].SubPath)
	}
	if got := out.InitContainers[0].Args[0]; got != "echo alice" {
		t.Fatalf("initContainer args[0] = %q, want echo alice", got)
	}
	if got := out.PrimaryContainerModifications.MergeEnv[0].ValueTemplate; got != testHeadSvc {
		t.Fatalf("mergeEnv valueTemplate = %q, want demo-head-svc", got)
	}
	if got := out.PrimaryContainerModifications.VolumeMounts[0].MountPath; got != "/shared/alice" {
		t.Fatalf("primary volumeMount mountPath = %q, want /shared/alice", got)
	}
}

// TestResolvePodModifications_DoesNotMutateOriginal locks in the deep-copy contract: the input mods
// must be returned untouched (still carrying {{ }} markers) so the original CR data is never corrupted.
func TestResolvePodModifications_DoesNotMutateOriginal(t *testing.T) {
	in := podModsFixture()
	if _, err := podModsResolver().ResolvePodModifications(in, podModsData()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.AdditionalContainers[0].Image != "img:{{ .Parameters.tag }}" {
		t.Fatalf("original image was mutated: %q", in.AdditionalContainers[0].Image)
	}
	if in.AdditionalContainers[0].Args[0] != "--addr={{ resource \"rayCluster\" \"{.status.head.serviceName}\" }}" {
		t.Fatalf("original args were mutated: %q", in.AdditionalContainers[0].Args[0])
	}
	if in.PrimaryContainerModifications.MergeEnv[0].ValueTemplate != "{{ resource \"rayCluster\" \"{.status.head.serviceName}\" }}" {
		t.Fatalf("original mergeEnv was mutated: %q", in.PrimaryContainerModifications.MergeEnv[0].ValueTemplate)
	}
}

// TestResolvePodModifications_LeavesPodVolumesUntouched locks in the contract that pod-level Volumes
// are intentionally NOT resolved (a Volume source is a structured union, not a free-form template).
func TestResolvePodModifications_LeavesPodVolumesUntouched(t *testing.T) {
	out, err := podModsResolver().ResolvePodModifications(podModsFixture(), podModsData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Volumes[0].Name != tagParamExpr {
		t.Fatalf("pod-level volume name must NOT be resolved, got %q", out.Volumes[0].Name)
	}
}

// TestResolvePodModifications_NilIsNoop verifies a nil input returns (nil, nil).
func TestResolvePodModifications_NilIsNoop(t *testing.T) {
	out, err := NewIntegrationTemplateResolver(nil).ResolvePodModifications(nil, podModsData())
	if err != nil || out != nil {
		t.Fatalf("nil mods must return (nil,nil), got (%v,%v)", out, err)
	}
}

// TestResolvePodModifications_FailClosedPropagatesFieldError verifies a resolution failure deep in a
// container field aborts the whole resolve (fail-closed) with a contextual error, not a partial result.
func TestResolvePodModifications_FailClosedPropagatesFieldError(t *testing.T) {
	mods := &workspacev1alpha1.PodModifications{
		AdditionalContainers: []corev1.Container{{
			Name: "ray-sidecar",
			Args: []string{`{{ resource "missing" "{.status.head.serviceName}" }}`}, // undeclared id
		}},
	}
	live := NewLiveResourceValueProvider(map[string]*unstructured.Unstructured{})
	if _, err := NewIntegrationTemplateResolver(live).ResolvePodModifications(mods, IntegrationTemplateData{}); err == nil {
		t.Fatal("expected a fail-closed error from an undeclared resource id in a container arg, got nil")
	}
}

// TestResolveTemplateExpression_JSONPathArrayIndex verifies [N] array indexing passes through to the
// client-go JSONPath engine untouched.
func TestResolveTemplateExpression_JSONPathArrayIndex(t *testing.T) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "ray.io", Version: "v1", Kind: rayClusterKind})
	_ = unstructured.SetNestedStringSlice(u.Object, []string{"first", "second"}, "status", "hosts")
	r := NewIntegrationTemplateResolver(NewLiveResourceValueProvider(map[string]*unstructured.Unstructured{rayClusterHandle: u}))

	out, err := r.ResolveTemplateExpression(`{{ resource "rayCluster" "{.status.hosts[1]}" }}`, IntegrationTemplateData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "second" {
		t.Fatalf("array-index JSONPath = %q, want second", out)
	}
}

// TestResolveTemplateExpression_JSONPathInvalidAndEmpty verifies both JSONPath error paths surface as
// fail-closed errors: an unparsable path, and a valid path that resolves to nothing.
func TestResolveTemplateExpression_JSONPathInvalidAndEmpty(t *testing.T) {
	live := NewLiveResourceValueProvider(map[string]*unstructured.Unstructured{
		rayClusterHandle: rayClusterFixture(),
	})
	r := NewIntegrationTemplateResolver(live)

	// Unparsable JSONPath (unbalanced brace).
	if _, err := r.ResolveTemplateExpression(`{{ resource "rayCluster" "{.status.head" }}`, IntegrationTemplateData{}); err == nil {
		t.Fatal("expected an invalid-JSONPath error, got nil")
	}
	// Valid path that matches nothing -> empty result is a hard error.
	if _, err := r.ResolveTemplateExpression(`{{ resource "rayCluster" "{.status.absent}" }}`, IntegrationTemplateData{}); err == nil {
		t.Fatal("expected an empty-result JSONPath error, got nil")
	}
}
