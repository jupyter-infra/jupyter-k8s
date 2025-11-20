/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("LifecycleDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-template",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{},
		}
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-workspace",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{},
		}
	})

	Describe("applyLifecycleDefaults", func() {
		It("should apply lifecycle defaults", func() {
			template.Spec.DefaultLifecycle = &corev1.Lifecycle{
				PostStart: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/sh", "-c", "echo started"},
					},
				},
			}

			applyLifecycleDefaults(workspace, template)

			Expect(workspace.Spec.Lifecycle).ToNot(BeNil())
			Expect(workspace.Spec.Lifecycle.PostStart.Exec.Command).To(Equal([]string{"/bin/sh", "-c", "echo started"}))
		})

		It("should not override existing lifecycle", func() {
			workspace.Spec.Lifecycle = &corev1.Lifecycle{
				PostStart: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/bash"},
					},
				},
			}
			template.Spec.DefaultLifecycle = &corev1.Lifecycle{
				PostStart: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/sh"},
					},
				},
			}

			applyLifecycleDefaults(workspace, template)

			Expect(workspace.Spec.Lifecycle.PostStart.Exec.Command).To(Equal([]string{"/bin/bash"}))
		})

		It("should apply idle shutdown defaults", func() {
			template.Spec.DefaultIdleShutdown = &workspacev1alpha1.IdleShutdownSpec{
				Enabled:              true,
				IdleTimeoutInMinutes: 60,
			}

			applyLifecycleDefaults(workspace, template)

			Expect(workspace.Spec.IdleShutdown).ToNot(BeNil())
			Expect(workspace.Spec.IdleShutdown.Enabled).To(BeTrue())
			Expect(workspace.Spec.IdleShutdown.IdleTimeoutInMinutes).To(Equal(60))
		})

		It("should not override existing idle shutdown", func() {
			workspace.Spec.IdleShutdown = &workspacev1alpha1.IdleShutdownSpec{
				Enabled: false,
			}
			template.Spec.DefaultIdleShutdown = &workspacev1alpha1.IdleShutdownSpec{
				Enabled: true,
			}

			applyLifecycleDefaults(workspace, template)

			Expect(workspace.Spec.IdleShutdown.Enabled).To(BeFalse())
		})
	})
})
