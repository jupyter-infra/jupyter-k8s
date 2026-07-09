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

var _ = Describe("ReadinessProbeDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testTemplateName,
				Namespace: testDefaultNamespace,
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{},
		}
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testWorkspaceName,
			},
			Spec: workspacev1alpha1.WorkspaceSpec{},
		}
	})

	Describe("applyReadinessProbeDefaults", func() {
		It("should apply readiness probe default when workspace has none", func() {
			template.Spec.DefaultReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(8888),
					},
				},
				FailureThreshold: 30,
			}

			applyReadinessProbeDefaults(workspace, template)

			Expect(workspace.Spec.ReadinessProbe).ToNot(BeNil())
			Expect(workspace.Spec.ReadinessProbe.TCPSocket).ToNot(BeNil())
			Expect(workspace.Spec.ReadinessProbe.TCPSocket.Port).To(Equal(intstr.FromInt(8888)))
			Expect(workspace.Spec.ReadinessProbe.FailureThreshold).To(Equal(int32(30)))
		})

		It("should not override an existing readiness probe", func() {
			workspace.Spec.ReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/status",
						Port: intstr.FromInt(8888),
					},
				},
			}
			template.Spec.DefaultReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(8888),
					},
				},
			}

			applyReadinessProbeDefaults(workspace, template)

			Expect(workspace.Spec.ReadinessProbe.HTTPGet).ToNot(BeNil())
			Expect(workspace.Spec.ReadinessProbe.HTTPGet.Path).To(Equal("/api/status"))
			Expect(workspace.Spec.ReadinessProbe.TCPSocket).To(BeNil())
		})

		It("should be a no-op when neither sets a probe", func() {
			applyReadinessProbeDefaults(workspace, template)

			Expect(workspace.Spec.ReadinessProbe).To(BeNil())
		})

		It("should deep-copy the template probe, not alias it", func() {
			template.Spec.DefaultReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(8888),
					},
				},
				FailureThreshold: 30,
			}

			applyReadinessProbeDefaults(workspace, template)

			// Mutate the template's probe after defaulting; workspace must be unaffected.
			template.Spec.DefaultReadinessProbe.FailureThreshold = 99
			template.Spec.DefaultReadinessProbe.TCPSocket.Port = intstr.FromInt(9999)

			Expect(workspace.Spec.ReadinessProbe.FailureThreshold).To(Equal(int32(30)))
			Expect(workspace.Spec.ReadinessProbe.TCPSocket.Port).To(Equal(intstr.FromInt(8888)))
		})
	})
})
