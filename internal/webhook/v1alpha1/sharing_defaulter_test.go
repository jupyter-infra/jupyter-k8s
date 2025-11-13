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

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("setWorkspaceSharingDefaults", func() {
	It("should set Public AccessType when OwnershipType is Public and AccessType is not populated", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: "Public",
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal("Public"))
		Expect(workspace.Spec.AccessType).To(Equal("Public"))
	})

	It("should keep OwnerOnly AccessType when OwnershipType is Public and AccessType is OwnerOnly", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: "Public",
				AccessType:    "OwnerOnly",
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal("Public"))
		Expect(workspace.Spec.AccessType).To(Equal("OwnerOnly"))
	})

	It("should keep Public AccessType when OwnershipType is Public and AccessType is Public", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: "Public",
				AccessType:    "Public",
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal("Public"))
		Expect(workspace.Spec.AccessType).To(Equal("Public"))
	})

	It("should set OwnerOnly AccessType when OwnershipType is OwnerOnly and AccessType is not populated", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: "OwnerOnly",
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal("OwnerOnly"))
		Expect(workspace.Spec.AccessType).To(Equal("OwnerOnly"))
	})

	It("should keep OwnerOnly AccessType when OwnershipType is OwnerOnly and AccessType is OwnerOnly", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: "OwnerOnly",
				AccessType:    "OwnerOnly",
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal("OwnerOnly"))
		Expect(workspace.Spec.AccessType).To(Equal("OwnerOnly"))
	})

	It("should keep Public AccessType when OwnershipType is OwnerOnly and AccessType is Public", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: "OwnerOnly",
				AccessType:    "Public",
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal("OwnerOnly"))
		Expect(workspace.Spec.AccessType).To(Equal("Public"))
	})

	It("should set Public OwnershipType and AccessType when both are not populated", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal("Public"))
		Expect(workspace.Spec.AccessType).To(Equal("Public"))
	})

	It("should set OwnerOnly AccessType when OwnershipType is not populated and AccessType is OwnerOnly", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				AccessType: "OwnerOnly",
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal("Public"))
		Expect(workspace.Spec.AccessType).To(Equal("OwnerOnly"))
	})

	It("should set Public AccessType when OwnershipType is not populated and AccessType is Public", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				AccessType: "Public",
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal("Public"))
		Expect(workspace.Spec.AccessType).To(Equal("Public"))
	})
})
