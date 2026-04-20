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
		It("should allow init containers when AllowInitContainers is true", func() {
			allow := true
			template.Spec.AllowInitContainers = &allow
			initContainers := []corev1.Container{
				{Name: "setup", Image: "busybox:latest"},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).To(BeNil())
		})

		It("should allow init containers when AllowInitContainers is nil (default true)", func() {
			template.Spec.AllowInitContainers = nil
			initContainers := []corev1.Container{
				{Name: "setup", Image: "busybox:latest"},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).To(BeNil())
		})

		It("should reject init containers when AllowInitContainers is false", func() {
			allow := false
			template.Spec.AllowInitContainers = &allow
			initContainers := []corev1.Container{
				{Name: "setup", Image: "busybox:latest"},
			}
			violation := validateInitContainers(initContainers, template)
			Expect(violation).NotTo(BeNil())
			Expect(violation.Type).To(Equal(ViolationTypeInitContainersNotAllowed))
			Expect(violation.Field).To(Equal("spec.initContainers"))
		})

		It("should allow empty init containers when AllowInitContainers is false", func() {
			allow := false
			template.Spec.AllowInitContainers = &allow
			violation := validateInitContainers(nil, template)
			Expect(violation).To(BeNil())
		})
	})
})
