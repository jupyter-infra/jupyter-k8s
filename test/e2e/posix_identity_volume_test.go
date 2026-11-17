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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// File access on one file system shared by two teams, through the uids and gids of POSIX identity.
var _ = Describe("POSIX Identity Volumes", Ordered, func() {
	const (
		subgroup       = "volume"
		namespace      = "posix-vol-team-a"
		otherNamespace = "posix-vol-team-b"
		alice          = "posix-vol-alice"
		bob            = "posix-vol-bob"
		carol          = "posix-vol-carol"
		namespaceGID   = "2130706520"
		otherGID       = "2130706521"
		sharedFSClaim  = "posix-shared-fs"
		sharedFSSeed   = "posix-shared-fs-seed"
		sharedMount    = "/home/jovyan/shared"
		teamADir       = sharedMount + "/namespaces/" + namespace
		teamBDir       = sharedMount + "/namespaces/" + otherNamespace
	)

	var alicePod, bobPod, carolPod, sharedPod string

	BeforeAll(func() {
		posixApplyBase(subgroup, "namespace-team-b", "team-b-private-template", "team-space-template")

		By("creating one file system exposed to both team namespaces")
		posixApply(subgroup, "shared-fs")
		WaitForPVCBinding(sharedFSClaim, namespace)
		WaitForPVCBinding(sharedFSClaim, otherNamespace)

		By("scaffolding the team directories as root, as the deployer does")
		posixApply(subgroup, "shared-fs-seed")
		// The image pull can be slow on a cold CI node; a failed seed stops the wait at once, with its logs.
		Eventually(func(g Gomega) {
			phase, err := kubectlGet("pod", sharedFSSeed, namespace, jsonPathStatusPhase)
			g.Expect(err).NotTo(HaveOccurred())
			if phase == "Failed" {
				logs, _ := utils.Run(exec.Command("kubectl", "logs", sharedFSSeed, "-n", namespace))
				StopTrying("seed pod failed").Wrap(fmt.Errorf("logs: %s", logs)).Now()
			}
			g.Expect(phase).To(Equal("Succeeded"))
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
		_, err := utils.Run(exec.Command("kubectl", "delete", "pod", sharedFSSeed, "-n", namespace,
			"--wait=true", "--timeout=60s"))
		Expect(err).NotTo(HaveOccurred())

		By("creating the workspaces mounting it")
		Expect(posixCreateAs(subgroup, "alice-team-workspace", alice)).To(Succeed())
		Expect(posixCreateAs(subgroup, "bob-team-workspace", bob)).To(Succeed())
		Expect(posixCreateAs(subgroup, "carol-team-workspace", carol)).To(Succeed())
		Expect(posixCreateAs(subgroup, "alice-team-shared-workspace", alice)).To(Succeed())

		if isUsingFinch() {
			return
		}
		alicePod = posixReadyPod("alice-team", namespace, sharedFSClaim)
		bobPod = posixReadyPod("bob-team", namespace, sharedFSClaim)
		carolPod = posixReadyPod("carol-team", otherNamespace, sharedFSClaim)
		sharedPod = posixReadyPod("alice-team-shared", namespace, sharedFSClaim)
	})

	AfterAll(func() {
		cleanupPosixIdentityTest([]string{namespace, otherNamespace}, []string{alice, bob, carol})
		_, _ = utils.Run(exec.Command("kubectl", "delete", "pv", "-l", "jk8s/e2e=posix-identity-test",
			"--ignore-not-found", "--wait=true", "--timeout=60s"))
	})

	BeforeEach(func() {
		if isUsingFinch() {
			Skip("exec-based file access checks (Finch has known cgroup access issues)")
		}
	})

	Context("Team directories", func() {
		It("lets teammates share files in their team directory", func() {
			notes := teamADir + "/notes.txt"
			By("writing a file as alice")
			posixExecOutput(alicePod, namespace, "sh", "-c", fmt.Sprintf("umask 0002 && echo alice > %s", notes))

			By("checking it belongs to alice and, through the setgid directory, to the team")
			Expect(posixExecOutput(alicePod, namespace, "stat", "-c", "%u %g", notes)).To(
				Equal(posixWorkspaceUID("alice-team", namespace) + " " + namespaceGID))

			// Guarded so a retried exec, whose first attempt did run, does not append twice.
			By("appending to it as bob, a teammate")
			posixExecOutput(bobPod, namespace, "sh", "-c", fmt.Sprintf("grep -qx bob %[1]s || echo bob >> %[1]s", notes))
			Expect(posixExecOutput(alicePod, namespace, "cat", notes)).To(Equal("alice\nbob"))
		})

		It("denies a team the directory of another team", func() {
			By("listing team A's directory as carol, from team B")
			expectExecOutput(carolPod, otherNamespace, false, "Permission denied", "ls", teamADir)

			By("writing to her own team's directory")
			posixExecOutput(carolPod, otherNamespace, "sh", "-c",
				fmt.Sprintf("umask 0002 && echo carol > %s/notes.txt", teamBDir))
			Expect(posixExecOutput(carolPod, otherNamespace, "stat", "-c", "%g", teamBDir+"/notes.txt")).To(Equal(otherGID))
		})

		It("writes files as the team from a shared workspace", func() {
			shared := teamADir + "/shared.txt"
			posixExecOutput(sharedPod, namespace, "sh", "-c", fmt.Sprintf("umask 0002 && echo team > %s", shared))
			Expect(posixExecOutput(sharedPod, namespace, "stat", "-c", "%u %g", shared)).To(
				Equal(namespaceGID + " " + namespaceGID))
		})
	})

	Context("Private and team directories", Ordered, func() {
		const teamMount = "/home/jovyan/team"
		var aliceSpacePod, bobSpacePod, aliceDir, bobDir string

		// script writes an executable script at path that prints its owner's name, group-writable so
		// that, in the team directory, teammates may run and change it.
		script := func(podName, path, owner string) {
			GinkgoHelper()
			posixExecOutput(podName, namespace, "sh", "-c", fmt.Sprintf(
				"umask 0002 && printf '#!/bin/sh\\necho ran-%s\\n' > %[2]s && chmod +x %[2]s", owner, path))
		}

		BeforeAll(func() {
			// The template's init container creates each user's private directory, users/<uid>.
			Expect(posixCreateAs(subgroup, "alice-space-workspace", alice)).To(Succeed())
			Expect(posixCreateAs(subgroup, "bob-space-workspace", bob)).To(Succeed())
			if isUsingFinch() {
				return
			}
			aliceSpacePod = posixReadyPod("alice-space", namespace, sharedFSClaim)
			bobSpacePod = posixReadyPod("bob-space", namespace, sharedFSClaim)
			aliceDir = teamMount + "/users/" + posixWorkspaceUID("alice-space", namespace)
			bobDir = teamMount + "/users/" + posixWorkspaceUID("bob-space", namespace)
		})

		It("lets each user read, write and execute in their private directory and the team directory", func() {
			for _, u := range []struct{ pod, dir, name string }{{aliceSpacePod, aliceDir, alice}, {bobSpacePod, bobDir, bob}} {
				By("writing and running a script in " + u.name + "'s private directory")
				script(u.pod, u.dir+"/mine.sh", u.name)
				expectExecOutput(u.pod, namespace, true, "ran-"+u.name, u.dir+"/mine.sh")
				expectExecOutput(u.pod, namespace, true, "ran-"+u.name, "cat", u.dir+"/mine.sh")
			}

			By("writing a script in the team directory as alice")
			teamScript := teamMount + "/team.sh"
			script(aliceSpacePod, teamScript, alice)

			By("reading, running and rewriting it as bob, a teammate")
			expectExecOutput(bobSpacePod, namespace, true, "ran-"+alice, "cat", teamScript)
			expectExecOutput(bobSpacePod, namespace, true, "ran-"+alice, teamScript)
			posixExecOutput(bobSpacePod, namespace, "sh", "-c",
				fmt.Sprintf("printf '#!/bin/sh\\necho ran-%s\\n' > %s", bob, teamScript))

			By("running bob's version as alice")
			expectExecOutput(aliceSpacePod, namespace, true, "ran-"+bob, teamScript)
		})

		It("denies each user the private directory of the other", func() {
			users := []struct{ pod, otherDir, name string }{{bobSpacePod, aliceDir, bob}, {aliceSpacePod, bobDir, alice}}
			for _, u := range users {
				By(u.name + " cannot list, read, run or write in the other's private directory")
				expectExecOutput(u.pod, namespace, false, "Permission denied", "ls", u.otherDir)
				expectExecOutput(u.pod, namespace, false, "Permission denied", "cat", u.otherDir+"/mine.sh")
				expectExecOutput(u.pod, namespace, false, "Permission denied", "sh", "-c", u.otherDir+"/mine.sh")
				expectExecOutput(u.pod, namespace, false, "Permission denied", "touch", u.otherDir+"/intruder")
			}
		})
	})
})
