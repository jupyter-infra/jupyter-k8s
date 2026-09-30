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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// Workspace GPU: the operator's GPU contract on a node advertising fake nvidia.com/gpu capacity
// (helpers_gpu.go): template GPU defaults and placement reach the pod, requests are bounded at
// create and on update, and workspaces without GPU settings get none.
var _ = Describe("Workspace GPU", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "gpu"
		gpuTemplateName    = "gpu-template"
	)

	var (
		gpuNodeName   string
		workspaceName string
	)

	BeforeAll(func() {
		gpuNodeName = setupFakeGPUNode()
		createTemplateForTest(gpuTemplateName, groupDir, "")
	})

	AfterAll(func() {
		deleteGPUTemplate(gpuTemplateName)
		teardownFakeGPUNode(gpuNodeName)
	})

	AfterEach(func() {
		deleteGPUWorkspace(workspaceName, workspaceNamespace)
		workspaceName = ""
	})

	Context("Template propagation", func() {
		It("should propagate template GPU defaults and placement to the pod and schedule it", func() {
			workspaceName = "gpu-default-workspace"

			By("creating a workspace with no resources or scheduling fields")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
				ConditionTypeDeleting:               ConditionFalse,
			})

			By("verifying defaulting copied GPU resources and placement onto the workspace")
			var ws workspacev1alpha1.Workspace
			Expect(kubectlGetInto("workspace", workspaceName, workspaceNamespace, &ws)).To(Succeed())
			Expect(ws.Spec.Resources).NotTo(BeNil(), "defaulting must copy the template defaultResources")
			gpus, ok := gpuQuantity(ws.Spec.Resources.Requests)
			Expect(ok).To(BeTrue(), "defaulting must copy the template GPU request")
			Expect(gpus).To(Equal(int64(1)))
			gpus, ok = gpuQuantity(ws.Spec.Resources.Limits)
			Expect(ok).To(BeTrue(), "defaulting must copy the template GPU limit")
			Expect(gpus).To(Equal(int64(1)))
			Expect(ws.Spec.NodeSelector).To(HaveKeyWithValue(fakeGPUNodeLabel, valueTrue),
				"defaulting must copy the template defaultNodeSelector")
			Expect(hasToleration(ws.Spec.Tolerations, fakeGPUResourceName,
				corev1.TolerationOpExists, corev1.TaintEffectNoSchedule)).To(BeTrue(),
				"defaulting must copy the template defaultTolerations")

			By("verifying GPU resources and placement reached the pod")
			pod := workspacePod(workspaceName, workspaceNamespace)
			primary := containerByName(pod.Spec, controller.PrimaryContainerName)
			Expect(primary).NotTo(BeNil())
			gpus, ok = gpuQuantity(primary.Resources.Requests)
			Expect(ok).To(BeTrue(), "the pod must request the GPU")
			Expect(gpus).To(Equal(int64(1)))
			gpus, ok = gpuQuantity(primary.Resources.Limits)
			Expect(ok).To(BeTrue(), "the pod must limit the GPU")
			Expect(gpus).To(Equal(int64(1)))
			Expect(pod.Spec.NodeSelector).To(HaveKeyWithValue(fakeGPUNodeLabel, valueTrue))
			Expect(hasToleration(pod.Spec.Tolerations, fakeGPUResourceName,
				corev1.TolerationOpExists, corev1.TaintEffectNoSchedule)).To(BeTrue())

			By("verifying the pod scheduled onto the GPU-advertising node")
			Expect(pod.Spec.NodeName).To(Equal(gpuNodeName))
			Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))

			By("verifying the /dev/shm volume is sized to the template's 512Mi memory limit")
			shm := volumeByName(pod.Spec, "workspace-shm")
			Expect(shm).NotTo(BeNil())
			Expect(shm.EmptyDir).NotTo(BeNil())
			Expect(shm.EmptyDir.Medium).To(Equal(corev1.StorageMediumMemory))
			Expect(shm.EmptyDir.SizeLimit.Cmp(resource.MustParse("512Mi"))).To(BeZero())
			Expect(mountByPath(*primary, "/dev/shm")).NotTo(BeNil())
			VerifyShmSize(workspaceName, workspaceNamespace, 512*1024)
		})

		It("should honor a workspace GPU request within template bounds", func() {
			workspaceName = "gpu-override-workspace"

			By("creating a workspace requesting 2 GPUs (the template max)")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("verifying the pod carries the overridden GPU request and limit")
			pod := workspacePod(workspaceName, workspaceNamespace)
			primary := containerByName(pod.Spec, controller.PrimaryContainerName)
			Expect(primary).NotTo(BeNil())
			gpus, ok := gpuQuantity(primary.Resources.Requests)
			Expect(ok).To(BeTrue())
			Expect(gpus).To(Equal(int64(2)), "the workspace override must win over the template default")
			gpus, ok = gpuQuantity(primary.Resources.Limits)
			Expect(ok).To(BeTrue())
			Expect(gpus).To(Equal(int64(2)))

			By("verifying the pod scheduled onto the GPU-advertising node")
			Expect(pod.Spec.NodeName).To(Equal(gpuNodeName))
		})
	})

	Context("Updates", func() {
		It("should restart with the new GPU count on an in-bounds change and reject an out-of-bounds one", func() {
			workspaceName = "gpu-default-workspace"

			By("creating a workspace that takes the template default of 1 GPU")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("raising the workspace GPU request and limit to 2 (the template max)")
			_, err := patchWorkspaceGPU(workspaceName, workspaceNamespace, "2")
			Expect(err).NotTo(HaveOccurred())

			By("verifying the pod is replaced by one requesting 2 GPUs")
			Eventually(func(g Gomega) {
				pods, err := workspacePods(workspaceName, workspaceNamespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pods).To(HaveLen(1), "the previous pod must be gone")
				g.Expect(pods[0].Status.Phase).To(Equal(corev1.PodRunning))
				primary := containerByName(pods[0].Spec, controller.PrimaryContainerName)
				g.Expect(primary).NotTo(BeNil())
				gpus, ok := gpuQuantity(primary.Resources.Requests)
				g.Expect(ok).To(BeTrue())
				g.Expect(gpus).To(Equal(int64(2)))
				gpus, ok = gpuQuantity(primary.Resources.Limits)
				g.Expect(ok).To(BeTrue())
				g.Expect(gpus).To(Equal(int64(2)))
			}).WithTimeout(180 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("attempting to raise the GPU request to 3 (bounds max is 2)")
			output, err := patchWorkspaceGPU(workspaceName, workspaceNamespace, "3")
			Expect(err).To(HaveOccurred(), "webhook should reject a GPU update above the template max")
			Expect(output).To(ContainSubstring(fakeGPUResourceName),
				"the rejection should name the violating GPU resource")

			By("verifying the workspace kept its 2-GPU request")
			request, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.resources.requests.nvidia\\.com/gpu}")
			Expect(err).NotTo(HaveOccurred())
			Expect(request).To(Equal("2"))
		})
	})

	Context("Validation", func() {
		It("should reject a workspace requesting GPUs above the template bound", func() {
			workspaceName = "gpu-exceed-workspace"

			By("attempting to create a workspace requesting 3 GPUs (bounds max is 2)")
			path := BuildTestResourcePath(workspaceName, groupDir, "")
			cmd := exec.Command("kubectl", "apply", "-f", path)
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "webhook should reject a GPU request above the template max")
			Expect(output).To(ContainSubstring(fakeGPUResourceName),
				"the rejection should name the violating GPU resource")

			By("verifying the workspace was not created")
			cmd = exec.Command("kubectl", verbGet, "workspace", workspaceName,
				"-n", workspaceNamespace, "--ignore-not-found")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "workspace should not exist after webhook rejection")
		})
	})

	Context("GPU-off path", func() {
		It("should not inject GPU defaults when a workspace overrides resources with CPU-only values", func() {
			workspaceName = "gpu-cpu-only-workspace"

			By("creating a workspace overriding resources with cpu/memory only")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("verifying the pod has no GPU in requests or limits")
			pod := workspacePod(workspaceName, workspaceNamespace)
			primary := containerByName(pod.Spec, controller.PrimaryContainerName)
			Expect(primary).NotTo(BeNil())
			_, ok := gpuQuantity(primary.Resources.Requests)
			Expect(ok).To(BeFalse(),
				"resource defaulting is all-or-nothing; explicit CPU-only resources must not gain a GPU")
			_, ok = gpuQuantity(primary.Resources.Limits)
			Expect(ok).To(BeFalse())

			By("verifying the override's cpu and memory did reach the pod")
			Expect(primary.Resources.Requests.Cpu().String()).To(Equal("200m"))
			Expect(primary.Resources.Requests.Memory().String()).To(Equal("256Mi"))
			Expect(primary.Resources.Limits.Cpu().String()).To(Equal("500m"))
			Expect(primary.Resources.Limits.Memory().String()).To(Equal("512Mi"))
		})

		It("should leave a workspace without a template free of GPU resources and placement", func() {
			workspaceName = "gpu-no-template-workspace"

			By("creating a workspace with no template reference")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("verifying the deployment pod template has no GPU resources or placement fields")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			// Assert on the deployment, not the pod: pod admission injects cluster-default
			// tolerations (not-ready/unreachable) that would defeat an emptiness check.
			var deploy appsv1.Deployment
			Expect(kubectlGetInto("deployment", deploymentName, workspaceNamespace, &deploy)).To(Succeed())
			podSpec := deploy.Spec.Template.Spec
			Expect(podSpec.NodeSelector).To(BeEmpty())
			Expect(podSpec.Tolerations).To(BeEmpty())
			primary := containerByName(podSpec, controller.PrimaryContainerName)
			Expect(primary).NotTo(BeNil())
			_, ok := gpuQuantity(primary.Resources.Requests)
			Expect(ok).To(BeFalse())
			_, ok = gpuQuantity(primary.Resources.Limits)
			Expect(ok).To(BeFalse())
		})
	})
})
