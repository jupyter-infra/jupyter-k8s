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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

var _ = Describe("Idle Shutdown", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "idle-shutdown"

		workspaceNoAS          = "workspace-idle-no-as"
		workspaceWithAS        = "workspace-idle-with-as"
		workspacePodExec       = "workspace-idle-podexec"
		workspaceLongTimeout   = "workspace-idle-long-timeout"
		workspacePodExecLong   = "workspace-idle-podexec-long"
		workspaceActiveNetwork = "workspace-idle-active-network"
		workspaceActivePodExec = "workspace-idle-active-podexec"
		accessStrategy         = "idle-shutdown-test-strategy"
	)

	BeforeAll(func() {
		By("reconfiguring controller with short idle check interval (5s)")
		cmd := exec.Command("helm", "upgrade", helmReleaseName,
			"dist/chart",
			"--namespace", OperatorNamespace,
			"--reuse-values",
			"--set", "idleShutdown.checkInterval=5s",
			"--wait",
			"--timeout", "3m",
		)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to reconfigure controller with short idle check interval")

		By("waiting for controller rollout to complete")
		cmd = exec.Command("kubectl", "rollout", "status",
			"deployment/jupyter-k8s-controller-manager",
			"-n", OperatorNamespace,
			"--timeout=90s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Controller deployment rollout timed out")

		By("verifying controller is running with short idle check interval")
		Eventually(func(g Gomega) {
			args, err := kubectlGet("deployment", "jupyter-k8s-controller-manager",
				OperatorNamespace, "{.spec.template.spec.containers[0].args}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(args).To(ContainSubstring("--idle-check-interval=5s"))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	})

	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Collecting diagnostics after idle shutdown test failure")
			cmd := exec.Command("kubectl", "get", "workspace", "-n", workspaceNamespace,
				"-o", "wide")
			if output, err := utils.Run(cmd); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "\nWorkspaces:\n%s\n", output)
			}
			allWorkspaces := []string{
				workspaceNoAS, workspaceWithAS, workspacePodExec,
				workspaceLongTimeout, workspacePodExecLong,
				workspaceActiveNetwork, workspaceActivePodExec,
			}
			for _, ws := range allWorkspaces {
				cmd = exec.Command("kubectl", "get", "workspace", ws, "-n", workspaceNamespace,
					"-o", "jsonpath={.status}")
				if output, err := utils.Run(cmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "\nWorkspace %s status: %s\n", ws, output)
				}
			}
			cmd = exec.Command("kubectl", "logs", "-n", OperatorNamespace,
				"-l", "control-plane=controller-manager", "--tail=200")
			if logs, err := utils.Run(cmd); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "\nController logs (last 200 lines):\n%s\n", logs)
			}
			cmd = exec.Command("kubectl", "get", "events", "-n", workspaceNamespace,
				"--sort-by=.lastTimestamp")
			if events, err := utils.Run(cmd); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "\nEvents:\n%s\n", events)
			}
		}
	})

	AfterAll(func() {
		By("cleaning up workspaces")
		cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", workspaceNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=120s")
		_, _ = utils.Run(cmd)

		By("cleaning up access strategies")
		cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", accessStrategy,
			"-n", SharedNamespace, "--ignore-not-found", "--wait=true", "--timeout=30s")
		_, _ = utils.Run(cmd)

		By("restoring controller to default idle check interval (5m)")
		cmd = exec.Command("helm", "upgrade", helmReleaseName,
			"dist/chart",
			"--namespace", OperatorNamespace,
			"--reuse-values",
			"--set", "idleShutdown.checkInterval=5m",
			"--wait",
			"--timeout", "3m",
		)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to restore controller idle check interval")

		By("waiting for controller rollout to complete")
		cmd = exec.Command("kubectl", "rollout", "status",
			"deployment/jupyter-k8s-controller-manager",
			"-n", OperatorNamespace,
			"--timeout=90s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Controller deployment rollout timed out after restore")
	})

	It("should stop idle workspaces via both network and podExec transports", func() {
		// podExec tests are skipped with Finch due to known cgroup exec issues in Kind.
		includePodExec := !isUsingFinch()

		By("creating access strategy with applicationBasePathTemplate and mergeEnv")
		createAccessStrategyForTest("access-strategy-with-base-path", groupDir, "")

		By("creating short-timeout workspaces")
		createWorkspaceForTest(workspaceNoAS, groupDir, "")
		createWorkspaceForTest(workspaceWithAS, groupDir, "")
		if includePodExec {
			createWorkspaceForTest(workspacePodExec, groupDir, "")
		}

		By("waiting for workspaces to become Available")
		WaitForWorkspaceToReachCondition(workspaceNoAS, workspaceNamespace, ConditionTypeAvailable, ConditionTrue)
		WaitForWorkspaceToReachCondition(workspaceWithAS, workspaceNamespace, ConditionTypeAvailable, ConditionTrue)
		if includePodExec {
			WaitForWorkspaceToReachCondition(workspacePodExec, workspaceNamespace, ConditionTypeAvailable, ConditionTrue)
		}

		By("waiting for short-timeout workspaces to be stopped")
		// idleTimeoutInMinutes=1, checkInterval=5s → workspaces stop ~65s after Available.
		stoppedWorkspaces := []string{workspaceNoAS, workspaceWithAS}
		if includePodExec {
			stoppedWorkspaces = append(stoppedWorkspaces, workspacePodExec)
		}
		stoppedPath := fmt.Sprintf("{.status.conditions[?(@.type==\"%s\")].status}", ConditionTypeStopped)
		Eventually(func(g Gomega) {
			for _, ws := range stoppedWorkspaces {
				status, err := kubectlGet("workspace", ws, workspaceNamespace, stoppedPath)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(status).To(Equal(ConditionTrue), "workspace %s should be stopped", ws)
			}
		}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		By("verifying desiredStatus was set to Stopped")
		for _, ws := range stoppedWorkspaces {
			desired, err := kubectlGet("workspace", ws, workspaceNamespace, "{.spec.desiredStatus}")
			Expect(err).NotTo(HaveOccurred())
			Expect(desired).To(Equal("Stopped"), "workspace %s should have desiredStatus=Stopped", ws)
		}

		By("verifying IdleShutdown events were recorded")
		for _, wsName := range stoppedWorkspaces {
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "events",
					"-n", workspaceNamespace,
					"--field-selector", fmt.Sprintf("involvedObject.name=%s,reason=IdleShutdown", wsName),
					"-o", "jsonpath={.items[*].reason}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(output)).To(ContainSubstring("IdleShutdown"))
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		}
	})

	It("should NOT stop workspaces when idle timeout has not been reached", func() {
		includePodExec := !isUsingFinch()

		// Two categories of workspaces that should NOT be stopped:
		// 1. Long timeout (60m) — idle but timeout not reached
		// 2. Active — 1m timeout but kept alive by periodic requests to /api/contents
		longTimeoutWorkspaces := []string{workspaceLongTimeout}
		activeWorkspaces := []string{workspaceActiveNetwork}
		if includePodExec {
			longTimeoutWorkspaces = append(longTimeoutWorkspaces, workspacePodExecLong)
			activeWorkspaces = append(activeWorkspaces, workspaceActivePodExec)
		}
		allNegativeWorkspaces := append(longTimeoutWorkspaces, activeWorkspaces...)

		By("creating long-timeout workspaces")
		createWorkspaceForTest(workspaceLongTimeout, groupDir, "")
		if includePodExec {
			createWorkspaceForTest(workspacePodExecLong, groupDir, "")
		}

		By("creating active workspaces (real JupyterLab, 1m timeout)")
		createWorkspaceForTest(workspaceActiveNetwork, groupDir, "")
		if includePodExec {
			createWorkspaceForTest(workspaceActivePodExec, groupDir, "")
		}

		By("waiting for all negative-case workspaces to become Available")
		for _, ws := range allNegativeWorkspaces {
			WaitForWorkspaceToReachCondition(ws, workspaceNamespace, ConditionTypeAvailable, ConditionTrue)
		}

		By("creating keepalive pod to periodically hit active workspace services")
		// Hitting /api/contents updates JupyterLab's last_activity timestamp,
		// preventing idle shutdown despite the 1m timeout.
		// Uses curlimages/curl pulled from Docker Hub (same as curl-metrics in e2e_test.go).
		keepaliveTargets := fmt.Sprintf(
			"http://workspace-%s-service.%s.svc.cluster.local:8888/api/contents",
			workspaceActiveNetwork, workspaceNamespace)
		if includePodExec {
			keepaliveTargets += fmt.Sprintf(
				" http://workspace-%s-service.%s.svc.cluster.local:8888/api/contents",
				workspaceActivePodExec, workspaceNamespace)
		}
		cmd := exec.Command("kubectl", "run", "idle-keepalive", "--restart=Never",
			"-n", workspaceNamespace,
			"--image=curlimages/curl:latest",
			"--command", "--",
			"sh", "-c", fmt.Sprintf(
				"while true; do for url in %s; do curl -sf $url > /dev/null; done; sleep 10; done",
				keepaliveTargets))
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create keepalive pod")

		// idleTimeoutInMinutes=1, checkInterval=5s. Without the keepalive, a workspace
		// stops at ~71s (verified empirically). 90s gives clear margin past the
		// failure point without adding unnecessary CI runtime.
		By("verifying workspaces stay running for 90s (past the 1m idle timeout)")
		availablePath := fmt.Sprintf("{.status.conditions[?(@.type==\"%s\")].status}", ConditionTypeAvailable)
		Consistently(func(g Gomega) {
			for _, ws := range allNegativeWorkspaces {
				status, err := kubectlGet("workspace", ws, workspaceNamespace, availablePath)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(status).To(Equal(ConditionTrue), "workspace %s should still be Available", ws)

				desired, err := kubectlGet("workspace", ws, workspaceNamespace, "{.spec.desiredStatus}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(desired).To(Equal("Running"), "workspace %s should still be Running", ws)
			}
		}).WithTimeout(90 * time.Second).WithPolling(10 * time.Second).Should(Succeed())

		By("cleaning up keepalive pod")
		cmd = exec.Command("kubectl", "delete", "pod", "idle-keepalive",
			"-n", workspaceNamespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)

		By("verifying NO IdleShutdown events")
		for _, ws := range allNegativeWorkspaces {
			cmd = exec.Command("kubectl", "get", "events",
				"-n", workspaceNamespace,
				"--field-selector", fmt.Sprintf("involvedObject.name=%s,reason=IdleShutdown", ws),
				"-o", "jsonpath={.items[*].reason}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(output)).To(BeEmpty(), "workspace %s should have no IdleShutdown event", ws)
		}
	})
})
