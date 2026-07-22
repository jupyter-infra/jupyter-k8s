/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"strings"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// undeclaredResourceExpr is a {{ resource }} expression naming a handle no resourceRef declares; reused
// across specs that assert a resolver-rendered field rejects an undeclared resourceRef.
const undeclaredResourceExpr = `{{ resource "otherCluster" "{.status.head.serviceName}" }}`

// validTemplate is a well-formed template: one resourceRef "rayCluster" and pod-mods that reference
// only that declared handle plus a parameter.
func validTemplate() *workspacev1alpha1.WorkspaceIntegrationTemplate {
	return &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: testIntegrationName, Namespace: "ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
			Parameters: []workspacev1alpha1.IntegrationTemplateParameter{
				{Name: testRayClusterParam},
				{Name: "rayClusterNamespace"},
			},
			ResourceRefs: []workspacev1alpha1.ResourceRef{{
				Name: "rayCluster", APIVersion: "ray.io/v1", Kind: "RayCluster",
				Metadata: workspacev1alpha1.ResourceRefMetadata{
					Name:      "{{ .Parameters.rayClusterName }}",
					Namespace: "{{ .Parameters.rayClusterNamespace }}",
				},
			}},
			DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
				PodModifications: &workspacev1alpha1.PodModifications{
					AdditionalContainers: []corev1.Container{{
						Name: "ray-sidecar",
						Args: []string{`ray start --address={{ resource "rayCluster" "{.status.head.serviceName}" }} --block`},
					}},
				},
			},
		},
	}
}

func TestValidateIntegrationTemplate_Valid(t *testing.T) {
	if err := validateIntegrationTemplate(validTemplate()); err != nil {
		t.Fatalf("valid template should pass, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_BadSyntax(t *testing.T) {
	tmpl := validTemplate()
	// Unterminated action -> text/template parse error.
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].Args = []string{`ray start {{ .Parameters.x `}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "invalid template") {
		t.Fatalf("expected a template syntax error, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_UndeclaredResource(t *testing.T) {
	tmpl := validTemplate()
	// References "otherCluster", which is not a declared resourceRef.
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].Args =
		[]string{`ray start --address={{ resource "otherCluster" "{.status.head.serviceName}" }}`}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "undeclared resourceRef") {
		t.Fatalf("expected undeclared-resourceRef error, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_BadJSONPath(t *testing.T) {
	tmpl := validTemplate()
	// Unbalanced brace in the JSONPath.
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].Args =
		[]string{`ray start --address={{ resource "rayCluster" "{.status.head.serviceName" }}`}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "invalid JSONPath") {
		t.Fatalf("expected invalid-JSONPath error, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_ResourceRefInMetadataRejected(t *testing.T) {
	tmpl := validTemplate()
	// A resourceRef's own metadata.name may reference only .Workspace/.Parameters -- a {{ resource }}
	// here is nonsensical (metadata names the very resource that would have to be fetched first) and
	// must be rejected at admission, not left to fail at resolve time.
	tmpl.Spec.ResourceRefs[0].Metadata.Name = `{{ resource "rayCluster" "{.metadata.name}" }}`
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "may not reference resources") {
		t.Fatalf("expected a resource-not-allowed-in-metadata error, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_UndeclaredParameterRejected(t *testing.T) {
	tmpl := validTemplate()
	// The author references {{ .Parameters.rayClustrName }} (typo) -- not in spec.parameters. This must
	// be rejected at the template author's write, not silently resolve to empty later.
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].Args =
		[]string{`ray start --name={{ .Parameters.rayClustrName }}`}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "rayClustrName") {
		t.Fatalf("expected an undeclared-parameter error naming rayClustrName, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_WorkingDirValidated(t *testing.T) {
	tmpl := validTemplate()
	// workingDir is a resolver-rendered field: an undeclared parameter here must be rejected at admission.
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].WorkingDir =
		`/home/{{ .Parameters.typoDir }}`
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "typoDir") {
		t.Fatalf("expected workingDir to be validated for an undeclared parameter, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_ContainerVolumeMountPathValidated(t *testing.T) {
	tmpl := validTemplate()
	// A container volume mount's mountPath is resolver-rendered and may use {{ resource }} -- an
	// undeclared handle must be rejected at admission.
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].VolumeMounts =
		[]corev1.VolumeMount{{Name: "data", MountPath: `/mnt/{{ resource "otherCluster" "{.metadata.name}" }}`}}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "undeclared resourceRef") {
		t.Fatalf("expected container volumeMount mountPath to be validated, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_VolumeMountSubPathValidated(t *testing.T) {
	tmpl := validTemplate()
	// subPath / subPathExpr are resolver-rendered too; a bad JSONPath in either must be caught.
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].VolumeMounts =
		[]corev1.VolumeMount{{
			Name:        "data",
			SubPath:     `{{ resource "rayCluster" "{.status.head.serviceName" }}`, // unbalanced brace
			SubPathExpr: `{{ .Parameters.rayClusterName }}`,
		}}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "invalid JSONPath") {
		t.Fatalf("expected volumeMount subPath JSONPath to be validated, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_PrimaryContainerVolumeMountValidated(t *testing.T) {
	tmpl := validTemplate()
	// primaryContainerModifications.volumeMounts is resolver-rendered; an undeclared parameter in a
	// mount path must be rejected at admission.
	tmpl.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications =
		&workspacev1alpha1.PrimaryContainerModifications{
			VolumeMounts: []corev1.VolumeMount{{Name: "shared", MountPath: `/mnt/{{ .Parameters.typoMount }}`}},
		}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "typoMount") {
		t.Fatalf("expected primaryContainerModifications volumeMount to be validated, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_WorkspaceFieldReferenceValid(t *testing.T) {
	tmpl := validTemplate()
	// The two real .Workspace fields must render without error (they exist on templateValidationWorkspace).
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].Args =
		[]string{`ray start --owner={{ .Workspace.Name }}.{{ .Workspace.Namespace }}`}
	if err := validateIntegrationTemplate(tmpl); err != nil {
		t.Fatalf("a valid .Workspace.Name/.Namespace reference should pass, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_WorkspaceFieldTypoRejected(t *testing.T) {
	tmpl := validTemplate()
	// A typo'd .Workspace field must be rejected. This works only because the render context's Workspace
	// is a typed struct (see templateValidationData) -- a struct-field miss is a hard text/template error,
	// unlike a map key (missingkey=error only guards maps). Regression guard for that invariant.
	tmpl.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].Args =
		[]string{`ray start --owner={{ .Workspace.Nme }}`}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "Nme") {
		t.Fatalf("expected a .Workspace field typo (Nme) to be rejected, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_InitContainerValidated(t *testing.T) {
	tmpl := validTemplate()
	// initContainers are resolver-rendered exactly like additionalContainers; an undeclared parameter in
	// an init container arg must be rejected at admission too.
	tmpl.Spec.DeploymentModifications.PodModifications.InitContainers = []corev1.Container{{
		Name: "init", Args: []string{`setup {{ .Parameters.typoInit }}`},
	}}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "typoInit") {
		t.Fatalf("expected an init container arg to be validated for an undeclared parameter, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_PrimaryMergeEnvValidated(t *testing.T) {
	tmpl := validTemplate()
	// primaryContainerModifications.mergeEnv[].valueTemplate is resolver-rendered; an undeclared handle in
	// a {{ resource }} there must be rejected.
	tmpl.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications =
		&workspacev1alpha1.PrimaryContainerModifications{
			MergeEnv: []workspacev1alpha1.AccessEnvTemplate{{
				Name: "RAY_ADDRESS", ValueTemplate: undeclaredResourceExpr,
			}},
		}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "undeclared resourceRef") {
		t.Fatalf("expected primaryContainerModifications mergeEnv to be validated, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_MetadataNamespaceRejectsResource(t *testing.T) {
	tmpl := validTemplate()
	// A resourceRef's metadata.namespace, like its name, identifies the resource to fetch and so may not
	// depend on a fetched one -- a {{ resource }} there must be rejected, not only in metadata.name.
	tmpl.Spec.ResourceRefs[0].Metadata.Namespace = `{{ resource "rayCluster" "{.metadata.namespace}" }}`
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "may not reference resources") {
		t.Fatalf("expected metadata.namespace to reject a {{ resource }} expression, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_StatusProbeCommandValidated(t *testing.T) {
	tmpl := validTemplate()
	// The statusProbe exec command is resolver-rendered (its {{ resource }} / {{ .Parameters }} are
	// resolved from the frozen values before exec), so an undeclared handle must be rejected at admission.
	tmpl.Spec.StatusProbe = &workspacev1alpha1.IntegrationStatusProbe{
		Exec: &corev1.ExecAction{Command: []string{
			"ray", "status", "--address",
			undeclaredResourceExpr,
		}},
	}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "undeclared resourceRef") {
		t.Fatalf("expected statusProbe.exec.command to be validated, got: %v", err)
	}
}

func TestValidateIntegrationTemplate_StatusProbeOnlyTemplateValidated(t *testing.T) {
	// A template may declare a statusProbe with NO deploymentModifications; the probe command must still
	// be validated (regression guard for the pod-mods early return skipping probe validation).
	tmpl := validTemplate()
	tmpl.Spec.DeploymentModifications = nil
	tmpl.Spec.StatusProbe = &workspacev1alpha1.IntegrationStatusProbe{
		Exec: &corev1.ExecAction{Command: []string{`{{ .Parameters.typoProbe }}`}},
	}
	err := validateIntegrationTemplate(tmpl)
	if err == nil || !strings.Contains(err.Error(), "typoProbe") {
		t.Fatalf("expected a probe-only template's command to be validated, got: %v", err)
	}
}

// TestWorkspaceIntegrationTemplateCustomValidator covers the admission wrapper: create/update reject an
// invalid template (wrapping the cause with the template name), a valid one passes, and delete is a no-op.
func TestWorkspaceIntegrationTemplateCustomValidator(t *testing.T) {
	v := &WorkspaceIntegrationTemplateCustomValidator{}
	ctx := context.Background()

	invalid := validTemplate()
	invalid.Spec.DeploymentModifications.PodModifications.AdditionalContainers[0].Args =
		[]string{undeclaredResourceExpr}

	t.Run("create rejects an invalid template and names it", func(t *testing.T) {
		_, err := v.ValidateCreate(ctx, invalid)
		if err == nil || !strings.Contains(err.Error(), "invalid WorkspaceIntegrationTemplate") ||
			!strings.Contains(err.Error(), invalid.GetName()) {
			t.Fatalf("expected a wrapped rejection naming the template, got: %v", err)
		}
	})

	t.Run("update rejects an invalid template", func(t *testing.T) {
		if _, err := v.ValidateUpdate(ctx, validTemplate(), invalid); err == nil {
			t.Fatal("expected update of an invalid template to be rejected")
		}
	})

	t.Run("create passes a valid template", func(t *testing.T) {
		if _, err := v.ValidateCreate(ctx, validTemplate()); err != nil {
			t.Fatalf("valid template should pass create, got: %v", err)
		}
	})

	t.Run("delete is a no-op", func(t *testing.T) {
		if _, err := v.ValidateDelete(ctx, invalid); err != nil {
			t.Fatalf("delete must never validate, got: %v", err)
		}
	})
}
