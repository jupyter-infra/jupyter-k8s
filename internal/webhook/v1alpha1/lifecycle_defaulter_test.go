/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("LifecycleDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-template",
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
				Enabled:        true,
				TimeoutMinutes: 60,
			}

			applyLifecycleDefaults(workspace, template)

			Expect(workspace.Spec.IdleShutdown).ToNot(BeNil())
			Expect(workspace.Spec.IdleShutdown.Enabled).To(BeTrue())
			Expect(workspace.Spec.IdleShutdown.TimeoutMinutes).To(Equal(60))
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
