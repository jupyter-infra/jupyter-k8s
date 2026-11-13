/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("SecurityDefaulter", func() {
	var (
		workspace *workspacev1alpha1.Workspace
		template  *workspacev1alpha1.WorkspaceTemplate
	)

	BeforeEach(func() {
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testWorkspaceName,
				Namespace: testDefaultNamespace,
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: testWorkspaceDisplayName,
			},
		}

		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: testTemplateName,
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DisplayName:  testTemplateDisplayName,
				DefaultImage: "test-image",
				DefaultPodSecurityContext: &workspacev1alpha1.TemplatedPodSecurityContext{
					FSGroup: intOrStringPtr(1000),
				},
			},
		}
	})

	Context("applySecurityDefaults", func() {
		It("should apply pod security context from template when workspace has none", func() {
			applySecurityDefaults(workspace, template)

			Expect(workspace.Spec.PodSecurityContext).NotTo(BeNil())
			Expect(workspace.Spec.PodSecurityContext.FSGroup).NotTo(BeNil())
			Expect(*workspace.Spec.PodSecurityContext.FSGroup).To(Equal(int64(1000)))
		})

		It("should not override existing pod security context", func() {
			workspace.Spec.PodSecurityContext = &corev1.PodSecurityContext{
				FSGroup: int64Ptr(2000),
			}

			applySecurityDefaults(workspace, template)

			Expect(*workspace.Spec.PodSecurityContext.FSGroup).To(Equal(int64(2000)))
		})

		It("should do nothing when template has no security context", func() {
			template.Spec.DefaultPodSecurityContext = nil

			applySecurityDefaults(workspace, template)

			Expect(workspace.Spec.PodSecurityContext).To(BeNil())
		})

		It("should not override existing container security context", func() {
			privileged := false
			workspace.Spec.ContainerSecurityContext = &corev1.SecurityContext{
				Privileged: &privileged,
			}
			template.Spec.DefaultContainerSecurityContext = &workspacev1alpha1.TemplatedSecurityContext{
				RunAsNonRoot: boolPtr(true),
			}

			applySecurityDefaults(workspace, template)

			Expect(*workspace.Spec.ContainerSecurityContext.Privileged).To(BeFalse())
			Expect(workspace.Spec.ContainerSecurityContext.RunAsNonRoot).To(BeNil())
		})

		It("should apply container security context from template when workspace has none", func() {
			template.Spec.DefaultContainerSecurityContext = &workspacev1alpha1.TemplatedSecurityContext{
				RunAsNonRoot: boolPtr(true),
				RunAsUser:    intOrStringPtr(1000),
			}

			applySecurityDefaults(workspace, template)

			Expect(workspace.Spec.ContainerSecurityContext).NotTo(BeNil())
			Expect(*workspace.Spec.ContainerSecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(*workspace.Spec.ContainerSecurityContext.RunAsUser).To(Equal(int64(1000)))
		})
	})
})

func int64Ptr(i int64) *int64 {
	return &i
}

func intOrStringPtr(i int32) *intstr.IntOrString {
	v := intstr.FromInt32(i)
	return &v
}

func boolPtr(b bool) *bool {
	return &b
}
