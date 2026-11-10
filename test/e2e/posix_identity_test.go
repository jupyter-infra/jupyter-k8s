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

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// Pod identity: the uid, gid and groups a workspace pod runs with, and recovery of lost identities.
// Admission is in posix_identity_admission_test.go, file access in posix_identity_volume_test.go.
var _ = Describe("POSIX Identity", Ordered, func() {
	const (
		subgroup     = "pod"
		namespace    = "posix-pod-team"
		alice        = "posix-pod-alice"
		bob          = "posix-pod-bob"
		namespaceGID = "2130706500"
		volumeGID    = "2147000123"
	)

	BeforeAll(func() {
		posixApplyBase(subgroup)
	})

	AfterAll(func() {
		cleanupPosixIdentityTest([]string{namespace}, []string{alice, bob})
	})

	Context("Pod identity", func() {
		It("runs a private workspace as its owner, with exactly the team and volume groups", func() {
			Expect(posixCreateAs(subgroup, "alice-private-workspace", alice)).To(Succeed())

			uid := posixWorkspaceUID("alice-private", namespace)
			n, err := strconv.ParseInt(uid, 10, 64)
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(BeNumerically(">=", 1<<20))
			Expect(n).To(BeNumerically("<", 1<<20+1<<30))

			Eventually(func(g Gomega) {
				username, err := posixIdentityUsername(uid)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(username).To(Equal(alice))
				ws := &workspacev1alpha1.Workspace{}
				g.Expect(kubectlGetInto("workspace", "alice-private", namespace, ws)).To(Succeed())
				g.Expect(ws.Labels).To(HaveKeyWithValue(workspaceutil.LabelPosixUID, uid))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			podName := posixReadyPod("alice-private", namespace)

			// Administrators find the pods running as a uid by this label.
			By("checking the pod carries the posix-uid label")
			Eventually(func(g Gomega) {
				pods, err := kubectlGetByLabels("pod", workspaceutil.LabelPosixUID+"="+uid, namespace,
					"{.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.Fields(pods)).To(ContainElement(podName))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			if isUsingFinch() {
				By("skipping exec-based identity checks (Finch has known cgroup access issues)")
				return
			}
			wantGroups := posixSortedIDs(strings.Join([]string{uid, namespaceGID, volumeGID}, " "))

			By("checking the groups the kubelet attached, which Strict limits to the pod spec's")
			expectContainerSupplementalGroups(podName, namespace, controller.PrimaryContainerName, wantGroups...)

			By("checking the identity inside the container")
			Expect(posixExecOutput(podName, namespace, "id", "-u")).To(Equal(uid))
			Expect(posixExecOutput(podName, namespace, "id", "-g")).To(Equal(uid))
			Expect(posixSortedIDs(posixExecOutput(podName, namespace, "id", "-G"))).To(Equal(wantGroups))
		})

		It("keeps a user's uid across workspaces, and gives another user a distinct one", func() {
			Expect(posixCreateAs(subgroup, "alice-second-workspace", alice)).To(Succeed())
			Expect(posixCreateAs(subgroup, "bob-private-workspace", bob)).To(Succeed())
			aliceUID := posixWorkspaceUID("alice-private", namespace)
			Expect(posixWorkspaceUID("alice-second", namespace)).To(Equal(aliceUID))
			Expect(posixWorkspaceUID("bob-private", namespace)).NotTo(Equal(aliceUID))
		})

		It("runs a shared workspace as the namespace", func() {
			Expect(posixCreateAs(subgroup, "alice-shared-workspace", alice)).To(Succeed())
			Expect(posixWorkspaceUID("alice-shared", namespace)).To(Equal(namespaceGID))
			if isUsingFinch() {
				By("skipping exec-based identity checks (Finch has known cgroup access issues)")
				return
			}
			podName := posixReadyPod("alice-shared", namespace)
			Expect(posixExecOutput(podName, namespace, "id", "-u")).To(Equal(namespaceGID))
		})
	})

	Context("Recovery", func() {
		It("recreates a deleted PosixIdentity from the workspace claims", func() {
			uid := posixWorkspaceUID("alice-private", namespace)
			By("deleting alice's PosixIdentity")
			Expect(posixDeleteIdentity("uid-" + uid)).To(Succeed())

			// As alice: only the owner, or an admin group, may change an OwnerOnly workspace.
			By("triggering a reconcile of alice's workspace")
			_, err := utils.Run(exec.Command("kubectl", "annotate", "workspace", "alice-private", "-n", namespace,
				fmt.Sprintf("e2e.jupyter.org/poke=%d", time.Now().UnixNano()), "--overwrite",
				"--as="+alice, "--as-group="+posixUsersGroup))
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				username, err := posixIdentityUsername(uid)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(username).To(Equal(alice))
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})
	})
})
