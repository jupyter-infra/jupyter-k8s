/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

var _ = Describe("MetadataDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "production-template"},
			Spec:       workspacev1alpha1.WorkspaceTemplateSpec{},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: testDisplayName},
		}
	})

	Context("applyMetadataDefaults", func() {
		It("should add template label when labels is nil", func() {
			applyMetadataDefaults(workspace, template)

			Expect(workspace.Labels).NotTo(BeNil())
			Expect(workspace.Labels).To(HaveKeyWithValue(controller.LabelWorkspaceTemplate, "production-template"))
		})

		It("should add template label to existing labels", func() {
			workspace.Labels = map[string]string{
				testExistingKey: "label",
			}

			applyMetadataDefaults(workspace, template)

			Expect(workspace.Labels).To(HaveKeyWithValue(testExistingKey, "label"))
			Expect(workspace.Labels).To(HaveKeyWithValue(controller.LabelWorkspaceTemplate, "production-template"))
		})

		It("should override existing template label", func() {
			workspace.Labels = map[string]string{
				controller.LabelWorkspaceTemplate: "old-template",
			}

			applyMetadataDefaults(workspace, template)

			Expect(workspace.Labels).To(HaveKeyWithValue(controller.LabelWorkspaceTemplate, "production-template"))
		})
	})

	Context("baseLabels", func() {
		It("should add labels from template", func() {
			template.Spec.BaseLabels = []workspacev1alpha1.TemplateLabel{
				{Key: testLabelKeyEnv, Value: testEnvProduction},
				{Key: testLabelKeyTeam, Value: testDataScience},
			}

			applyMetadataDefaults(workspace, template)

			Expect(workspace.Labels).To(HaveKeyWithValue(testLabelKeyEnv, testEnvProduction))
			Expect(workspace.Labels).To(HaveKeyWithValue(testLabelKeyTeam, testDataScience))
		})

		It("should not override existing workspace labels", func() {
			workspace.Labels = map[string]string{
				testLabelKeyEnv: testEnvDevelopment,
			}
			template.Spec.BaseLabels = []workspacev1alpha1.TemplateLabel{
				{Key: testLabelKeyEnv, Value: testEnvProduction},
				{Key: testLabelKeyTeam, Value: testDataScience},
			}

			applyMetadataDefaults(workspace, template)

			Expect(workspace.Labels).To(HaveKeyWithValue(testLabelKeyEnv, testEnvDevelopment))
			Expect(workspace.Labels).To(HaveKeyWithValue(testLabelKeyTeam, testDataScience))
		})

		It("should handle template with no BaseLabels", func() {
			workspace.Labels = map[string]string{
				"custom": "label",
			}

			applyMetadataDefaults(workspace, template)

			Expect(workspace.Labels).To(HaveKeyWithValue("custom", "label"))
			Expect(workspace.Labels).To(HaveKeyWithValue(controller.LabelWorkspaceTemplate, "production-template"))
		})
	})
})
