/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("setWorkspaceSharingDefaults", func() {
	It("should set Public AccessType when OwnershipType is Public and AccessType is not populated", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: testOwnershipPublic,
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipPublic))
		Expect(workspace.Spec.AccessType).To(Equal(testOwnershipPublic))
	})

	It("should keep OwnerOnly AccessType when OwnershipType is Public and AccessType is OwnerOnly", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: testOwnershipPublic,
				AccessType:    testOwnershipOwnerOnly,
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipPublic))
		Expect(workspace.Spec.AccessType).To(Equal(testOwnershipOwnerOnly))
	})

	It("should keep Public AccessType when OwnershipType is Public and AccessType is Public", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: testOwnershipPublic,
				AccessType:    testOwnershipPublic,
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipPublic))
		Expect(workspace.Spec.AccessType).To(Equal(testOwnershipPublic))
	})

	It("should set OwnerOnly AccessType when OwnershipType is OwnerOnly and AccessType is not populated", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: testOwnershipOwnerOnly,
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipOwnerOnly))
		Expect(workspace.Spec.AccessType).To(Equal(testOwnershipOwnerOnly))
	})

	It("should keep OwnerOnly AccessType when OwnershipType is OwnerOnly and AccessType is OwnerOnly", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: testOwnershipOwnerOnly,
				AccessType:    testOwnershipOwnerOnly,
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipOwnerOnly))
		Expect(workspace.Spec.AccessType).To(Equal(testOwnershipOwnerOnly))
	})

	It("should keep Public AccessType when OwnershipType is OwnerOnly and AccessType is Public", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				OwnershipType: testOwnershipOwnerOnly,
				AccessType:    testOwnershipPublic,
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipOwnerOnly))
		Expect(workspace.Spec.AccessType).To(Equal(testOwnershipPublic))
	})

	It("should set Public OwnershipType and AccessType when both are not populated", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipPublic))
		Expect(workspace.Spec.AccessType).To(Equal(testOwnershipPublic))
	})

	It("should set OwnerOnly AccessType when OwnershipType is not populated and AccessType is OwnerOnly", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				AccessType: testOwnershipOwnerOnly,
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipPublic))
		Expect(workspace.Spec.AccessType).To(Equal(testOwnershipOwnerOnly))
	})

	It("should set Public AccessType when OwnershipType is not populated and AccessType is Public", func() {
		workspace := &workspacev1alpha1.Workspace{
			Spec: workspacev1alpha1.WorkspaceSpec{
				AccessType: testOwnershipPublic,
			},
		}
		setWorkspaceSharingDefaults(workspace)
		Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipPublic))
		Expect(workspace.Spec.AccessType).To(Equal(testOwnershipPublic))
	})
})
