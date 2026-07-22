/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"regexp"
	"strings"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// probeWithCommand builds an Exec statusProbe carrying the given command args.
func probeWithCommand(cmd ...string) *workspacev1alpha1.IntegrationStatusProbe {
	return &workspacev1alpha1.IntegrationStatusProbe{Exec: &corev1.ExecAction{Command: cmd}}
}

// TestResolveStatusProbeCommand_CaptureThenReplay locks in the fix for the probe-resolution bug: the
// statusProbe command carries {{ resource }} expressions just like the sidecar, so it must be rendered
// through the resolver. The capture side (live provider) harvests the keys and the probe side (frozen
// provider) replays them -- rendering the SAME command from both, and never leaving a literal
// "{{ resource ... }}" for exec.
func TestResolveStatusProbeCommand_CaptureThenReplay(t *testing.T) {
	data := IntegrationTemplateData{Parameters: map[string]string{rayClusterNameKey: testClusterName}}
	probeExpr := `{{ resource "rayCluster" "{.status.head.serviceName}" }}:6379`
	probe := probeWithCommand("ray", "status", "--address", probeExpr)

	// Capture direction: live provider renders the command AND records the key it read.
	live := NewLiveResourceValueProvider(map[string]*unstructured.Unstructured{
		rayClusterHandle: rayClusterFixture(),
	})
	captureResolver := NewIntegrationTemplateResolver(live)
	captured, err := resolveStatusProbeCommand(captureResolver, probe, data)
	if err != nil {
		t.Fatalf("capture: unexpected error: %v", err)
	}
	if got := captured.Exec.Command[3]; got != "demo-head-svc:6379" {
		t.Fatalf("capture: command arg = %q, want %q", got, "demo-head-svc:6379")
	}
	key := CaptureKey(rayClusterHandle, "{.status.head.serviceName}")
	if _, ok := live.Captured()[key]; !ok {
		t.Fatalf("capture: probe expression must be recorded under %q so it can be frozen", key)
	}

	// The input probe must not be mutated (a copy is returned).
	if probe.Exec.Command[3] != probeExpr {
		t.Fatalf("capture: input probe was mutated: %q", probe.Exec.Command[3])
	}

	// Replay direction: frozen provider seeded from the captured map renders the same command with no
	// resource available -- proving the probe path no longer execs a literal template.
	frozen := NewFrozenResourceValueProvider(live.Captured())
	replayResolver := NewIntegrationTemplateResolver(frozen)
	replayed, err := resolveStatusProbeCommand(replayResolver, probe, data)
	if err != nil {
		t.Fatalf("replay: unexpected error: %v", err)
	}
	if got := replayed.Exec.Command[3]; got != "demo-head-svc:6379" {
		t.Fatalf("replay: command arg = %q, want %q", got, "demo-head-svc:6379")
	}
}

func TestResolveStatusProbeCommand_NilAndNoExecPassThrough(t *testing.T) {
	r := NewIntegrationTemplateResolver(nil)
	if got, err := resolveStatusProbeCommand(r, nil, IntegrationTemplateData{}); err != nil || got != nil {
		t.Fatalf("nil probe: want (nil,nil), got (%v,%v)", got, err)
	}
	noExec := &workspacev1alpha1.IntegrationStatusProbe{TimeoutSeconds: 5}
	got, err := resolveStatusProbeCommand(r, noExec, IntegrationTemplateData{})
	if err != nil || got != noExec {
		t.Fatalf("no-exec probe: want the probe returned unchanged, got (%v,%v)", got, err)
	}
}

// resourceRefNamePattern is the CRD validation pattern on a resourceRef's name (the id passed to
// CaptureKey). The API server enforces it before any webhook or the controller runs, so CaptureKey
// never sees an id outside this alphabet.
const resourceRefNamePattern = `^[a-z][a-zA-Z0-9-]*$`

// TestCaptureKey_PipeSeparatorIsCollisionFree pins the invariant that makes CaptureKey safe as a map
// key: it joins id and jsonPath with "|", and a valid resourceRef id can never contain "|" (the CRD
// pattern forbids it), so no (id, jsonPath) pair can be spelled two ways that collide. Without this
// guard a future widening of the id alphabet to include "|" would silently let two distinct
// expressions -- e.g. ("a", "|b|c") and ("a|b", "c") -- freeze under the same key and replay the wrong
// value. It is a Go-level invariant (the pattern gates the input), so no envtest is needed.
func TestCaptureKey_PipeSeparatorIsCollisionFree(t *testing.T) {
	idPattern := regexp.MustCompile(resourceRefNamePattern)

	// (a) The CRD alphabet never admits the "|" separator into an id.
	if idPattern.MatchString("ray|cluster") {
		t.Fatalf("resourceRef name pattern %q must reject a pipe -- CaptureKey collision-freedom depends on it", resourceRefNamePattern)
	}
	for _, valid := range []string{rayClusterHandle, "a", "ray-cluster-2"} {
		if !idPattern.MatchString(valid) {
			t.Fatalf("expected %q to be a valid resourceRef id", valid)
		}
		if strings.Contains(CaptureKey(valid, "{.status.head.serviceName}"), valid+"|") {
			continue
		}
		t.Fatalf("CaptureKey(%q, ...) must place the id before the %q separator", valid, "|")
	}

	// (b) Distinct (id, jsonPath) pairs over the valid alphabet yield distinct keys.
	keys := map[string]struct{}{}
	pairs := [][2]string{
		{rayClusterHandle, headServiceNameJSONPath},
		{rayClusterHandle, "{.status.endpoints.gcs}"},
		{"ray-cluster-2", headServiceNameJSONPath},
		{"other", headServiceNameJSONPath},
	}
	for _, p := range pairs {
		k := CaptureKey(p[0], p[1])
		if _, dup := keys[k]; dup {
			t.Fatalf("CaptureKey produced a duplicate key %q for distinct inputs", k)
		}
		keys[k] = struct{}{}
	}

	// (c) The only way ("a", "b|c") and ("a|b", "c") could collide is an id containing "|"; the pattern
	// prevents "a|b" from ever being a valid id, so the collision is unreachable at admission.
	if idPattern.MatchString("a|b") {
		t.Fatalf("an id containing %q would make CaptureKey ambiguous; the CRD pattern must forbid it", "|")
	}
}
