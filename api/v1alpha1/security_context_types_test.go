/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// identityFields are the fields the templated mirrors type as int-or-string instead of the upstream type.
var identityFields = map[string]bool{"runAsUser": true, "runAsGroup": true, "fsGroup": true, "supplementalGroups": true}

// jsonFields maps the JSON name of each field of a struct type to its Go type.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		fields[name] = t.Field(i).Type
	}
	return fields
}

// TestTemplatedSecurityContextsMirrorCore fails when the upstream security contexts gain, lose or retype
// a field the mirrors do not follow, e.g. after a k8s.io/api bump: update security_context_types.go
// (the type and its Resolve method).
func TestTemplatedSecurityContextsMirrorCore(t *testing.T) {
	for _, pair := range []struct{ mirror, core reflect.Type }{
		{reflect.TypeOf(TemplatedPodSecurityContext{}), reflect.TypeOf(corev1.PodSecurityContext{})},
		{reflect.TypeOf(TemplatedSecurityContext{}), reflect.TypeOf(corev1.SecurityContext{})},
	} {
		mirror, core := jsonFields(pair.mirror), jsonFields(pair.core)
		for name, coreType := range core {
			mirrorType, ok := mirror[name]
			if !ok {
				t.Errorf("%s lacks field %q of %s", pair.mirror.Name(), name, pair.core.Name())
				continue
			}
			if !identityFields[name] && mirrorType != coreType {
				t.Errorf("%s.%s is %s, %s has %s", pair.mirror.Name(), name, mirrorType, pair.core.Name(), coreType)
			}
		}
		for name := range mirror {
			if _, ok := core[name]; !ok {
				t.Errorf("%s has field %q, which %s no longer has", pair.mirror.Name(), name, pair.core.Name())
			}
		}
	}
}

// TestTemplatedSecurityContextsResolveEveryField catches a mirrored field that Resolve forgets to copy:
// a fully populated mirror must resolve to a fully populated upstream struct.
func TestTemplatedSecurityContextsResolveEveryField(t *testing.T) {
	id := intstr.FromInt32(1000)
	pod := &TemplatedPodSecurityContext{
		SELinuxOptions: &corev1.SELinuxOptions{User: "u"}, WindowsOptions: &corev1.WindowsSecurityContextOptions{RunAsUserName: ptr.To("u")},
		RunAsUser: &id, RunAsGroup: &id, RunAsNonRoot: ptr.To(true), SupplementalGroups: []intstr.IntOrString{id},
		SupplementalGroupsPolicy: ptr.To(corev1.SupplementalGroupsPolicyStrict), FSGroup: &id,
		Sysctls: []corev1.Sysctl{{Name: "n", Value: "v"}}, FSGroupChangePolicy: ptr.To(corev1.FSGroupChangeOnRootMismatch),
		SeccompProfile:      &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		AppArmorProfile:     &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeRuntimeDefault},
		SELinuxChangePolicy: ptr.To(corev1.SELinuxChangePolicyRecursive),
	}
	resolvedPod, err := pod.Resolve(StaticID)
	if err != nil {
		t.Fatal(err)
	}
	assertAllFieldsSet(t, reflect.ValueOf(*resolvedPod))

	container := &TemplatedSecurityContext{
		Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}, Privileged: ptr.To(false),
		SELinuxOptions: &corev1.SELinuxOptions{User: "u"}, WindowsOptions: &corev1.WindowsSecurityContextOptions{RunAsUserName: ptr.To("u")},
		RunAsUser: &id, RunAsGroup: &id, RunAsNonRoot: ptr.To(true), ReadOnlyRootFilesystem: ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false), ProcMount: ptr.To(corev1.DefaultProcMount),
		SeccompProfile:  &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		AppArmorProfile: &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeRuntimeDefault},
	}
	resolvedContainer, err := container.Resolve(StaticID)
	if err != nil {
		t.Fatal(err)
	}
	assertAllFieldsSet(t, reflect.ValueOf(*resolvedContainer))
	if !equality.Semantic.DeepEqual(resolvedContainer.Capabilities, container.Capabilities) {
		t.Errorf("capabilities not copied")
	}
}

func assertAllFieldsSet(t *testing.T, v reflect.Value) {
	t.Helper()
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Errorf("Resolve leaves %s.%s unset", v.Type().Name(), v.Type().Field(i).Name)
		}
	}
}

func TestTemplatedSecurityContextHasTemplates(t *testing.T) {
	static, tmpl := intstr.FromInt32(1000), intstr.FromString("{{ .User.Uid }}")
	if (&TemplatedPodSecurityContext{RunAsUser: &static}).HasTemplates() {
		t.Error("an integer is not a template")
	}
	if !(&TemplatedPodSecurityContext{SupplementalGroups: []intstr.IntOrString{static, tmpl}}).HasTemplates() {
		t.Error("a templated supplemental group is a template")
	}
	if !(&TemplatedSecurityContext{RunAsGroup: &tmpl}).HasTemplates() {
		t.Error("a templated runAsGroup is a template")
	}
	if _, err := (&TemplatedPodSecurityContext{FSGroup: &tmpl}).Resolve(StaticID); err == nil {
		t.Error("StaticID must reject a template")
	}
}

// fieldDocs maps each field of a struct type, in a Go source file, to its docstring without marker
// lines (+optional, +listType=..., kubebuilder validations), which legitimately differ.
func fieldDocs(t *testing.T, path, typeName string) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != typeName {
			return true
		}
		for _, field := range spec.Type.(*ast.StructType).Fields.List {
			var lines []string
			for _, line := range strings.Split(field.Doc.Text(), "\n") {
				if !strings.HasPrefix(line, "+") {
					lines = append(lines, line)
				}
			}
			docs[field.Names[0].Name] = strings.TrimSpace(strings.Join(lines, "\n"))
		}
		return false
	})
	if len(docs) == 0 {
		t.Fatalf("type %s not found in %s", typeName, path)
	}
	return docs
}

// TestTemplatedSecurityContextsMirrorCoreDocs fails when a mirrored field's docstring differs from
// upstream's, e.g. after a k8s.io/api bump: copy upstream's docstring into security_context_types.go.
// Identity fields keep upstream's docstring and add a paragraph after it.
func TestTemplatedSecurityContextsMirrorCoreDocs(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "k8s.io/api").Output()
	if err != nil {
		t.Fatalf("locating the k8s.io/api module: %v", err)
	}
	upstreamFile := filepath.Join(strings.TrimSpace(string(out)), "core", "v1", "types.go")
	identityGoFields := map[string]bool{"RunAsUser": true, "RunAsGroup": true, "FSGroup": true, "SupplementalGroups": true}

	for _, pair := range []struct{ mirror, core string }{
		{"TemplatedPodSecurityContext", "PodSecurityContext"},
		{"TemplatedSecurityContext", "SecurityContext"},
	} {
		upstream := fieldDocs(t, upstreamFile, pair.core)
		mirror := fieldDocs(t, "security_context_types.go", pair.mirror)
		for name, want := range upstream {
			got, ok := mirror[name]
			switch {
			case !ok:
				t.Errorf("%s lacks field %s", pair.mirror, name)
			case identityGoFields[name] && !strings.HasPrefix(got, want+"\n\n"):
				t.Errorf("%s.%s must start with upstream's docstring, then a paragraph:\n%s", pair.mirror, name, want)
			case !identityGoFields[name] && got != want:
				t.Errorf("%s.%s docstring differs from upstream's:\n%s", pair.mirror, name, want)
			}
		}
	}
}
