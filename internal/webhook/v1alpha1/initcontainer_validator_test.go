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

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("InitContainerValidator", func() {
	var template *workspacev1alpha1.WorkspaceTemplate

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "test-template"},
			Spec:       workspacev1alpha1.WorkspaceTemplateSpec{},
		}
	})

	Context("validateInitContainers", func() {
		It("should allow init containers when AllowCustomInitContainers is true", func() {
			allow := true
			template.Spec.AllowCustomInitContainers = &allow
			initContainers := []corev1.Container{
				{Name: "setup", Image: "busybox:latest"},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).To(BeNil())
		})

		It("should reject init containers when AllowCustomInitContainers is nil (default false)", func() {
			template.Spec.AllowCustomInitContainers = nil
			initContainers := []corev1.Container{
				{Name: "setup", Image: "busybox:latest"},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).NotTo(BeNil())
			Expect(violation.Type).To(Equal(ViolationTypeInitContainersNotAllowed))
		})

		It("should reject init containers when AllowCustomInitContainers is false", func() {
			allow := false
			template.Spec.AllowCustomInitContainers = &allow
			initContainers := []corev1.Container{
				{Name: "setup", Image: "busybox:latest"},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).NotTo(BeNil())
			Expect(violation.Type).To(Equal(ViolationTypeInitContainersNotAllowed))
		})

		It("should allow empty init containers regardless of setting", func() {
			allow := false
			template.Spec.AllowCustomInitContainers = &allow
			violation := validateInitContainers(nil, template)
			Expect(violation).To(BeNil())
		})

		It("should allow template default init containers even when custom not allowed", func() {
			template.Spec.DefaultInitContainers = []corev1.Container{
				{Name: "template-init", Image: "busybox:latest", Command: []string{"sh", "-c", "echo hi"}},
			}
			template.Spec.AllowCustomInitContainers = nil
			initContainers := []corev1.Container{
				{Name: "template-init", Image: "busybox:latest", Command: []string{"sh", "-c", "echo hi"}},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).To(BeNil())
		})

		It("should reject when name matches but command differs from template default", func() {
			template.Spec.DefaultInitContainers = []corev1.Container{
				{Name: "template-init", Image: "busybox:latest", Command: []string{"sh", "-c", "echo hi"}},
			}
			template.Spec.AllowCustomInitContainers = nil
			initContainers := []corev1.Container{
				{Name: "template-init", Image: "busybox:latest", Command: []string{"sh", "-c", "echo HACKED"}},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).NotTo(BeNil())
			Expect(violation.Type).To(Equal(ViolationTypeInitContainersNotAllowed))
		})

		It("should reject when name matches but image differs from template default", func() {
			template.Spec.DefaultInitContainers = []corev1.Container{
				{Name: "template-init", Image: "busybox:latest", Command: []string{"echo"}},
			}
			template.Spec.AllowCustomInitContainers = nil
			initContainers := []corev1.Container{
				{Name: "template-init", Image: "evil:latest", Command: []string{"echo"}},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).NotTo(BeNil())
		})

		It("should reject when init containers are reordered from template defaults", func() {
			template.Spec.DefaultInitContainers = []corev1.Container{
				{Name: "init-a", Image: "busybox:latest"},
				{Name: "init-b", Image: "alpine:latest"},
			}
			template.Spec.AllowCustomInitContainers = nil
			initContainers := []corev1.Container{
				{Name: "init-b", Image: "alpine:latest"},
				{Name: "init-a", Image: "busybox:latest"},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).NotTo(BeNil())
			Expect(violation.Type).To(Equal(ViolationTypeInitContainersNotAllowed))
		})

		It("should reject when extra init containers are added beyond defaults", func() {
			template.Spec.DefaultInitContainers = []corev1.Container{
				{Name: "template-init", Image: "busybox:latest"},
			}
			template.Spec.AllowCustomInitContainers = nil
			initContainers := []corev1.Container{
				{Name: "template-init", Image: "busybox:latest"},
				{Name: "extra", Image: "evil:latest"},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).NotTo(BeNil())
		})
	})
})
