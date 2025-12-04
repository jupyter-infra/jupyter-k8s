//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

type valueTestCaseForStorageTest struct {
	description string
	jsonPath    string
	expected    string
}

var _ = Describe("Workspace Storage", Ordered, func() {
	const (
		storageClassName     = "rancher-storage-class"
		storageClassFilename = "rancher-storage-class"
		workspaceNamespace   = "default"

		group            = "storage"
		baseSubgroup     = "base"
		externalSubgroup = "external"
		templateSubgroup = "template"

		baseWorkspaceName = "workspace-with-storage"
		externalPvc1Name  = "external-pvc-1"
		templateName      = "storage-template"
	)

	BeforeAll(func() {
		createStorageClassForTest(storageClassFilename)
	})

	AfterAll(func() {
		deleteStorageClassForTest(storageClassName)
	})

	AfterEach(func() {
		deleteResourcesForResourcesTest(workspaceNamespace)
	})

	Context("Primary volume", func() {
		It("should create PVC with correct specifications and default mount path", func() {
			workspaceFilename := baseWorkspaceName
			workspaceName := baseWorkspaceName

			By("creating a workspace with a pvc")
			createWorkspaceForTest(workspaceFilename, group, baseSubgroup)

			By("waiting for the workspace to become Available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			testPvcForStorageTest(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForStorageTest{
					{
						description: "verifying pvc size",
						jsonPath:    "{.spec.resources.requests.storage}",
						expected:    "2Gi",
					},
					{
						description: "verifying pvc access mode",
						jsonPath:    "{.spec.accessModes[0]}",
						expected:    "ReadWriteOnce",
					},
					{
						description: "verifying owner reference",
						jsonPath:    "{.metadata.ownerReferences[0].name}",
						expected:    workspaceName,
					},
					{
						description: "verifying binding status",
						jsonPath:    "{.status.phase}",
						expected:    "Bound",
					},
				},
			)

			VerifyWorkspaceVolumeMount(workspaceName, workspaceNamespace,
				"workspace-storage", "/home/jovyan")
		})

		It("should handle large storage requests", func() {
			workspaceFilename := "workspace-large-storage"
			workspaceName := "workspace-large-storage"

			By("creating a workspace with large pvc")
			createWorkspaceForTest(workspaceFilename, group, baseSubgroup)

			By("waiting for the workspace to become Available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			testPvcForStorageTest(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForStorageTest{
					{
						description: "verifying pvc size",
						jsonPath:    "{.spec.resources.requests.storage}",
						expected:    "10Gi",
					},
				},
			)
		})

		It("should not create PVC when storage is disabled", func() {
			workspaceFilename := "workspace-no-storage"
			workspaceName := "workspace-no-storage"

			By("creating a workspace without storage")
			createWorkspaceForTest(workspaceFilename, group, baseSubgroup)

			By("waiting for the workspace to become Available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying no pvc was created")
			pvcName := controller.GeneratePVCName(workspaceName)
			VerifyResourceDoesNotExist("pvc", pvcName, workspaceNamespace)
		})

		It("should mount PVC with custom path", func() {
			workspaceName := "workspace-custom-mountpath"
			workspaceFilename := "workspace-custom-mountpath"

			By("creating a workspace with custom mount path")
			createWorkspaceForTest(workspaceFilename, group, baseSubgroup)

			By("waiting for the workspace to become Available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			VerifyWorkspaceVolumeMount(workspaceName, workspaceNamespace,
				"workspace-storage", "/home/jovyan/data")
		})

		It("should delete pvc when workspace is deleted", func() {
			workspaceFilename := baseWorkspaceName
			workspaceName := baseWorkspaceName

			By("creating a workspace with a pvc")
			createWorkspaceForTest(workspaceFilename, group, baseSubgroup)

			By("waiting for the workspace to become Available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("deleting the workspace")
			cmd := exec.Command("kubectl", "delete", "workspace", workspaceName, "--wait=false")
			_, deleteErr := utils.Run(cmd)
			Expect(deleteErr).NotTo(HaveOccurred())

			By("verifying the workspace still exists and has a deletion timestamp")
			deleteTimestamp, deleteTimestampErr := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.metadata.deletionTimestamp}")
			Expect(deleteTimestampErr).NotTo(HaveOccurred())
			Expect(deleteTimestamp).NotTo(BeEmpty())

			By("verifying the workspace ultimately gets deleted")
			WaitForResourceToNotExist("workspace", workspaceName, workspaceNamespace, 60*time.Second, 5*time.Second)

			By("verifying the pvc was deleted")
			pvcName := controller.GeneratePVCName(workspaceName)
			VerifyResourceDoesNotExist("pvc", pvcName, workspaceNamespace)
		})

		It("should persist data across pod restart", func() {
			workspaceFilename := baseWorkspaceName
			workspaceName := baseWorkspaceName

			By("creating a workspace with a pvc")
			createWorkspaceForTest(workspaceFilename, group, baseSubgroup)

			By("waiting for the workspace to become Available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying can write to volume")
			VerifyPodCanAccessHomeVolume(workspaceName, workspaceNamespace)

			By("restarting the workspace pod")
			RestartWorkspacePod(workspaceName, workspaceNamespace)

			By("verifying the data was persisted")
			VerifyHomeVolumeDataPersisted(workspaceName, workspaceNamespace)
		})
	})

	Context("External volumes", func() {
		It("should mount external PVCs specified in volumes field", func() {
			externalPvc1Name := externalPvc1Name
			externalPvc1Filename := externalPvc1Name

			externalPvc2Name := "external-pvc-2"
			externalPvc2Filename := "external-pvc-2"

			workspaceFilename := "workspace-with-volumes"
			workspaceName := "workspace-with-volumes"

			By("creating the first external pvc")
			createPvcForTest(externalPvc1Filename, group, externalSubgroup)

			By("creating the second external pvc")
			createPvcForTest(externalPvc2Filename, group, externalSubgroup)

			By("creating a workspace referencing multiple volumes")
			createWorkspaceForTest(workspaceFilename, group, externalSubgroup)

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying first external mount")
			VerifyWorkspaceVolumeMount(workspaceName, workspaceNamespace, "data-volume", "/home/jovyan/data")

			By("verifying second external mount")
			VerifyWorkspaceVolumeMount(workspaceName, workspaceNamespace, "shared-volume", "/home/jovyan/shared")

			By("verifying can write to first external pvc")
			VerifyPodCanAccessExternalVolumes(workspaceName, workspaceNamespace, externalPvc1Name, "/home/jovyan/data")

			By("verifying can write to second external pvc")
			VerifyPodCanAccessExternalVolumes(workspaceName, workspaceNamespace, externalPvc2Name, "/home/jovyan/shared")
		})

		It("should reject workspace referencing PVC owned by another workspace", func() {
			ownerWorkspaceFilename := "owner-workspace"
			ownerWorkspaceName := "owner-workspace"

			rejectedWorkspaceFilename := "volume-ownership-violation-workspace"
			rejectedWorkspaceName := "volume-ownership-violation-workspace"

			By("creating a workspace owning the pvc")
			createWorkspaceForTest(ownerWorkspaceFilename, group, externalSubgroup)

			By("waiting for the owner workspace to become available")
			WaitForWorkspaceToReachCondition(
				ownerWorkspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("attempting to create second workspace that references the first workspace's PVC")
			VerifyCreateWorkspaceRejectedByWebhook(
				rejectedWorkspaceFilename, group, externalSubgroup, rejectedWorkspaceName, workspaceNamespace)
		})

		It("should allow workspace referencing unowned PVC", func() {
			externalPvc1Filename := externalPvc1Name

			workspaceFilename := "volume-ownership-allowed-workspace"
			workspaceName := "volume-ownership-allowed-workspace"

			By("creating a standalone PVC")
			createPvcForTest(externalPvc1Filename, group, externalSubgroup)

			By("creating workspace that references the standalone PVC")
			createWorkspaceForTest(workspaceFilename, group, externalSubgroup)

			By("verifying that the workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)
		})
	})

	Context("Template-based", func() {
		It("should create PVC within template bounds", func() {
			templateFilename := templateName

			workspaceFilename := "workspace-with-template-storage"
			workspaceName := "workspace-with-template-storage"

			By("creating the template")
			createTemplateForTest(templateFilename, group, templateSubgroup)

			By("creating the workspace referencing the template")
			createWorkspaceForTest(workspaceFilename, group, templateSubgroup)

			By("verifying that the workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			testPvcForStorageTest(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForStorageTest{
					{
						description: "verifying pvc size",
						jsonPath:    "{.spec.resources.requests.storage}",
						expected:    "2Gi",
					},
					{
						description: "verifying pvc access mode",
						jsonPath:    "{.spec.accessModes[0]}",
						expected:    "ReadWriteOnce",
					},
					{
						description: "verifying owner reference",
						jsonPath:    "{.metadata.ownerReferences[0].name}",
						expected:    workspaceName,
					},
					{
						description: "verifying binding status",
						jsonPath:    "{.status.phase}",
						expected:    "Bound",
					},
				},
			)
		})

		It("should use template default when storage not specified", func() {
			templateFilename := templateName

			workspaceFilename := "workspace-template-default-storage"
			workspaceName := "workspace-template-default-storage"

			By("creating the template")
			createTemplateForTest(templateFilename, group, templateSubgroup)

			By("creating the workspace referencing the template")
			createWorkspaceForTest(workspaceFilename, group, templateSubgroup)

			By("verifying that the workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			testPvcForStorageTest(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForStorageTest{
					{
						description: "verifying pvc size",
						jsonPath:    "{.spec.resources.requests.storage}",
						expected:    "1Gi",
					},
					{
						description: "verifying pvc access mode",
						jsonPath:    "{.spec.accessModes[0]}",
						expected:    "ReadWriteOnce",
					},
					{
						description: "verifying owner reference",
						jsonPath:    "{.metadata.ownerReferences[0].name}",
						expected:    workspaceName,
					},
					{
						description: "verifying binding status",
						jsonPath:    "{.status.phase}",
						expected:    "Bound",
					},
				},
			)
		})

		It("should reject workspace exceeding template bounds", func() {
			templateFilename := templateName

			workspaceFilename := "workspace-exceed-storage-bounds"
			workspaceName := "workspace-exceed-storage-bounds"

			By("creating the template")
			createTemplateForTest(templateFilename, group, templateSubgroup)

			By("verifying the webhook rejects the workspace creation")
			VerifyCreateWorkspaceRejectedByWebhook(workspaceFilename, group, templateSubgroup, workspaceName, workspaceNamespace)
		})
	})
})

//nolint:unparam
func deleteResourcesForResourcesTest(namespace string) {
	By("cleaning up resources for resources test")

	// Delete all workspaces in the namespace
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", namespace,
		"--ignore-not-found", "--wait=true", "--timeout=180s")
	_, _ = utils.Run(cmd)

	// Delete standalone PVCs that might have been created
	cmd = exec.Command("kubectl", "delete", "pvc", "--all", "-n", namespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	// Delete templates that might have been created
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	// Wait to ensure all resources are fully deleted
	time.Sleep(2 * time.Second)
}

//nolint:unparam
func testPvcForStorageTest(
	workspaceName string,
	workspaceNamespace string,
	testCases []valueTestCaseForStorageTest,
) {
	pvcName := controller.GeneratePVCName(workspaceName)

	// Test each value case
	for _, tc := range testCases {
		By(tc.description)
		output, err := kubectlGet("pvc", pvcName, workspaceNamespace, tc.jsonPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(Equal(tc.expected))
	}
}
