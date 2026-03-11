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

var _ = Describe("Reserved Prefix Validator", func() {

	var workspace *workspacev1alpha1.Workspace

	BeforeEach(func() {
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image: "jupyter/base-notebook:latest",
			},
		}
	})

	Context("validateReservedPrefixOnCreate", func() {
		It("should allow workspace with no reserved prefix labels or annotations", func() {
			workspace.Labels = map[string]string{"team": "data-science"}
			workspace.Annotations = map[string]string{"note": "test"}
			Expect(validateReservedPrefixOnCreate(workspace)).To(Succeed())
		})

		It("should allow workspace with system-managed labels", func() {
			workspace.Labels = map[string]string{
				controller.LabelWorkspaceTemplate:          "my-template",
				controller.LabelWorkspaceTemplateNamespace: "default",
			}
			Expect(validateReservedPrefixOnCreate(workspace)).To(Succeed())
		})

		It("should allow workspace with system-managed annotations", func() {
			workspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy:     "user1",
				controller.AnnotationLastUpdatedBy: "user1",
			}
			Expect(validateReservedPrefixOnCreate(workspace)).To(Succeed())
		})

		It("should reject workspace with unknown reserved prefix label", func() {
			workspace.Labels = map[string]string{
				"workspace.jupyter.org/custom-label": "value",
			}
			err := validateReservedPrefixOnCreate(workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("label 'workspace.jupyter.org/custom-label' uses reserved prefix workspace.jupyter.org/"))
		})

		It("should reject workspace with unknown reserved prefix annotation", func() {
			workspace.Annotations = map[string]string{
				"workspace.jupyter.org/custom-annotation": "value",
			}
			err := validateReservedPrefixOnCreate(workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/custom-annotation' uses reserved prefix workspace.jupyter.org/"))
		})

		It("should allow workspace with nil labels and annotations", func() {
			Expect(validateReservedPrefixOnCreate(workspace)).To(Succeed())
		})
	})

	Context("validateReservedPrefixOnUpdate", func() {
		var oldWorkspace *workspacev1alpha1.Workspace

		BeforeEach(func() {
			oldWorkspace = workspace.DeepCopy()
		})

		It("should allow update with no reserved prefix changes", func() {
			oldWorkspace.Labels = map[string]string{"team": "a"}
			workspace.Labels = map[string]string{"team": "b"}
			Expect(validateReservedPrefixOnUpdate(oldWorkspace, workspace)).To(Succeed())
		})

		It("should reject adding unknown reserved prefix label", func() {
			oldWorkspace.Labels = map[string]string{}
			workspace.Labels = map[string]string{
				"workspace.jupyter.org/custom": "value",
			}
			err := validateReservedPrefixOnUpdate(oldWorkspace, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("label 'workspace.jupyter.org/custom' uses reserved prefix workspace.jupyter.org/"))
		})

		It("should reject adding unknown reserved prefix annotation", func() {
			oldWorkspace.Annotations = map[string]string{}
			workspace.Annotations = map[string]string{
				"workspace.jupyter.org/custom": "value",
			}
			err := validateReservedPrefixOnUpdate(oldWorkspace, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/custom' uses reserved prefix workspace.jupyter.org/"))
		})

		It("should reject changing SetOnCreateOnly annotation", func() {
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			workspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "malicious-user",
			}
			err := validateReservedPrefixOnUpdate(oldWorkspace, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/created-by' is immutable"))
		})

		It("should reject removing SetOnCreateOnly annotation", func() {
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			workspace.Annotations = map[string]string{}
			err := validateReservedPrefixOnUpdate(oldWorkspace, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/created-by' cannot be removed"))
		})

		It("should reject removing SetOnCreateOnly annotation when annotations are nil", func() {
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			workspace.Annotations = nil
			err := validateReservedPrefixOnUpdate(oldWorkspace, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/created-by' cannot be removed"))
		})

		It("should allow changing SetAlways annotation", func() {
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationLastUpdatedBy: "user1",
			}
			workspace.Annotations = map[string]string{
				controller.AnnotationLastUpdatedBy: "user2",
			}
			Expect(validateReservedPrefixOnUpdate(oldWorkspace, workspace)).To(Succeed())
		})

		It("should allow changing SetAlways labels", func() {
			oldWorkspace.Labels = map[string]string{
				controller.LabelWorkspaceTemplate: "template-v1",
			}
			workspace.Labels = map[string]string{
				controller.LabelWorkspaceTemplate: "template-v2",
			}
			Expect(validateReservedPrefixOnUpdate(oldWorkspace, workspace)).To(Succeed())
		})

		It("should allow removing SetAlways labels", func() {
			oldWorkspace.Labels = map[string]string{
				controller.LabelAccessStrategyName: "strategy-1",
			}
			workspace.Labels = map[string]string{}
			Expect(validateReservedPrefixOnUpdate(oldWorkspace, workspace)).To(Succeed())
		})

		It("should reject setting created-by to empty string", func() {
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			workspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "",
			}
			err := validateReservedPrefixOnUpdate(oldWorkspace, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/created-by' is immutable"))
		})

		It("should allow update when preemption-reason annotation is unchanged", func() {
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy:        "user1",
				controller.PreemptionReasonAnnotation: controller.PreemptedReason,
			}
			workspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy:        "user1",
				controller.PreemptionReasonAnnotation: controller.PreemptedReason,
			}
			Expect(validateReservedPrefixOnUpdate(oldWorkspace, workspace)).To(Succeed())
		})
	})
})
