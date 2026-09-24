//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// Workspace Drift Repair: the controller watches the Deployment and Service it owns
// (Owns() in SetupWithManager), so out-of-band changes trigger a reconcile that repairs
// them: a deleted owned resource is recreated, and a mutated pod template is rewritten
// to the workspace-defined spec.
var _ = Describe("Workspace Drift Repair", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "drift"
		workspaceName      = "workspace-drift"
	)

	AfterEach(func() {
		deleteResourcesForDriftTest(workspaceNamespace)
	})

	It("should recreate the Service after an out-of-band deletion", func() {
		By("creating a running workspace")
		createWorkspaceForTest(workspaceName, groupDir, "")

		By("waiting for the workspace to become Available")
		WaitForWorkspaceToReachCondition(
			workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

		serviceName := GetWorkspaceServiceName(workspaceName, workspaceNamespace)

		By("capturing the service UID before deletion")
		oldUID, err := kubectlGet("service", serviceName, workspaceNamespace, "{.metadata.uid}")
		Expect(err).NotTo(HaveOccurred())
		Expect(oldUID).NotTo(BeEmpty())

		By("deleting the service out of band")
		cmd := exec.Command("kubectl", "delete", "service", serviceName,
			"-n", workspaceNamespace, "--wait=true")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("verifying the operator recreates the service")
		Eventually(func(g Gomega) {
			newUID, err := kubectlGet("service", serviceName, workspaceNamespace, "{.metadata.uid}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(newUID).NotTo(BeEmpty())
			g.Expect(newUID).NotTo(Equal(oldUID), "the service must be a new object")
		}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		By("verifying the workspace settles Available with the recreated service")
		WaitForWorkspaceToReachCondition(
			workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)
	})

	It("should revert an out-of-band mutation of the deployment pod template", func() {
		By("creating a running workspace")
		createWorkspaceForTest(workspaceName, groupDir, "")

		By("waiting for the workspace to become Available")
		WaitForWorkspaceToReachCondition(
			workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

		deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
			"{.status.deploymentName}")
		Expect(err).NotTo(HaveOccurred())
		Expect(deploymentName).NotTo(BeEmpty())

		imageJSONPath := fmt.Sprintf("{.spec.template.spec.containers[?(@.name=='%s')].image}",
			controller.PrimaryContainerName)
		originalImage, err := kubectlGet("deployment", deploymentName, workspaceNamespace, imageJSONPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(originalImage).NotTo(BeEmpty())
		generationBefore, err := deploymentGeneration(deploymentName, workspaceNamespace)
		Expect(err).NotTo(HaveOccurred())

		By("mutating the primary container image out of band")
		cmd := exec.Command("kubectl", "set", "image", "deployment/"+deploymentName,
			controller.PrimaryContainerName+"=drifted-image:latest", "-n", workspaceNamespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("verifying the operator reverts the image to the workspace-defined one")
		Eventually(func(g Gomega) {
			image, err := kubectlGet("deployment", deploymentName, workspaceNamespace, imageJSONPath)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(image).To(Equal(originalImage))
			// The mutation and the revert are two spec changes, so the generation advanced twice.
			generation, err := deploymentGeneration(deploymentName, workspaceNamespace)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(generation).To(BeNumerically(">=", generationBefore+2))
		}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		By("verifying the workspace settles Available after the revert")
		WaitForWorkspaceToReachCondition(
			workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)
	})
})

// deleteResourcesForDriftTest removes only the objects this Ordered suite creates, by
// explicit name, so it can never delete unrelated objects sharing the "default" namespace.
func deleteResourcesForDriftTest(workspaceNamespace string) {
	By("cleaning up the drift test workspace")
	cmd := exec.Command("kubectl", "delete", "workspace", "workspace-drift",
		"-n", workspaceNamespace, "--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)
}

// deploymentGeneration returns metadata.generation, which the API server increments on every
// spec change.
func deploymentGeneration(name, namespace string) (int64, error) {
	output, err := kubectlGet("deployment", name, namespace, "{.metadata.generation}")
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(output), 10, 64)
}
