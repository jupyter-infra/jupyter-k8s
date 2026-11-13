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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// The POSIX identity specs split across posix_identity_*_test.go. Each file is its own top-level
// container, with its own namespaces, gids and users, because Ginkgo shuffles top-level containers: one
// file's teardown never touches another's objects. Fixtures live in static/posix-identity-<subgroup>.
const (
	posixIdentityGroup = "posix-identity"
	// posixUsersGroup is the group the test users impersonate; each test namespace binds it a Role on
	// workspaces only.
	posixUsersGroup = "posix-identity-e2e-users"
	// posixAdminUser and posixWorkspaceAdminGroup are the identity teardown impersonates to delete the
	// test users' OwnerOnly workspaces. The group is the chart's default CLUSTER_ADMIN_GROUP.
	posixAdminUser           = "posix-identity-e2e-admin"
	posixWorkspaceAdminGroup = "cluster-workspace-admin"
)

// posixApply applies a fixture of the subgroup as the test's admin.
func posixApply(subgroup, filename string) {
	ginkgo.GinkgoHelper()
	path := BuildTestResourcePath(filename, posixIdentityGroup, subgroup)
	_, err := utils.Run(exec.Command("kubectl", "apply", "-f", path))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// posixApplyBase applies the fixtures every subgroup has, its team namespace (with the Role of
// posixUsersGroup) and its private and shared templates, then the given ones.
func posixApplyBase(subgroup string, filenames ...string) {
	ginkgo.GinkgoHelper()
	for _, f := range append([]string{"namespace", "private-template", "shared-template"}, filenames...) {
		posixApply(subgroup, f)
	}
}

// posixCreateAs creates a fixture of the subgroup as a test user.
func posixCreateAs(subgroup, filename, user string) error {
	_, err := posixCreateOutputAs(subgroup, filename, user)
	return err
}

// posixWorkspaceUID reads the uid admission resolved into a workspace. Reads are retried, as in the
// storage tests: the API server can return transient errors on a loaded CI node.
func posixWorkspaceUID(workspace, namespace string) string {
	ginkgo.GinkgoHelper()
	var uid string
	gomega.Eventually(func(g gomega.Gomega) {
		var err error
		uid, err = kubectlGet("workspace", workspace, namespace, "{.spec.podSecurityContext.runAsUser}")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(uid).NotTo(gomega.BeEmpty())
	}, 30*time.Second, 2*time.Second).Should(gomega.Succeed())
	return uid
}

// posixReadyPod waits for the workspace's home claim and the given claims to bind, then for its pod to
// be ready. Every pod-running template here gives workspaces a home volume, which binds only once the
// pod schedules.
func posixReadyPod(workspace, namespace string, claims ...string) string {
	ginkgo.GinkgoHelper()
	return readyWorkspacePod(workspace, namespace, append([]string{controller.GeneratePVCName(workspace)}, claims...)...)
}

// posixExecOutput runs a command in the workspace container and returns its output. kubectl exec can
// fail transiently with OCI runtime errors on a loaded CI node, so failures are retried: commands must
// be idempotent.
func posixExecOutput(podName, namespace string, command ...string) string {
	ginkgo.GinkgoHelper()
	args := append([]string{"exec", podName, "-n", namespace, "-c", controller.PrimaryContainerName, "--"}, command...)
	var output string
	gomega.Eventually(func(g gomega.Gomega) {
		var err error
		output, err = utils.Run(exec.Command("kubectl", args...))
		g.Expect(err).NotTo(gomega.HaveOccurred(), output)
	}, 60*time.Second, 2*time.Second).Should(gomega.Succeed())
	return strings.TrimSpace(output)
}

// posixSortedIDs parses a space-separated list of ids, as `id -G` prints it, into a sorted,
// deduplicated list.
func posixSortedIDs(output string) []int64 {
	ginkgo.GinkgoHelper()
	fields := strings.Fields(output)
	ids := make([]int64, 0, len(fields))
	for _, field := range fields {
		id, err := strconv.ParseInt(field, 10, 64)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), output)
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// posixIdentityUsername returns the username a uid's PosixIdentity records.
func posixIdentityUsername(uid string) (string, error) {
	return kubectlGet("posixidentity", "uid-"+uid, "", "{.spec.username}")
}

// posixIdentitiesOf returns the names of the PosixIdentity objects recording a username, read from the
// API server.
func posixIdentitiesOf(username string) []string {
	ginkgo.GinkgoHelper()
	var names []string
	gomega.Eventually(func(g gomega.Gomega) {
		output, err := kubectlGetAllNamespaces("posixidentity",
			`{range .items[*]}{.metadata.name} {.spec.username}{"\n"}{end}`)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		names = nil
		for _, line := range strings.Split(output, "\n") {
			if fields := strings.Fields(line); len(fields) == 2 && fields[1] == username {
				names = append(names, fields[0])
			}
		}
	}, 30*time.Second, 2*time.Second).Should(gomega.Succeed())
	return names
}

// posixDeleteIdentity deletes a PosixIdentity, removing its finalizer first, as an admin would.
func posixDeleteIdentity(name string) error {
	if _, err := utils.Run(exec.Command("kubectl", "patch", "posixidentity", name,
		"--type=merge", "-p", `{"metadata":{"finalizers":null}}`)); err != nil {
		return err
	}
	_, err := utils.Run(exec.Command("kubectl", "delete", "posixidentity", name, "--ignore-not-found", "--wait=true"))
	return err
}

// cleanupPosixIdentityTest deletes the workspaces, templates and namespaces of a test file, then the
// PosixIdentity objects of its users, which nothing garbage-collects. Errors are ignored: a teardown
// cleans what it can.
func cleanupPosixIdentityTest(namespaces, users []string) {
	// The test users own these OwnerOnly workspaces, and the workspace webhook lets only their owner, the
	// controller or an admin group change or delete them. The kind admin is in no admin group, so
	// teardown impersonates the group the chart names in CLUSTER_ADMIN_GROUP, which the webhook exempts
	// from the owner check, with a binding granting it workspace deletion only (workspace-admin-rbac).
	// A workspace left behind would hold its template's finalizer, and so the namespace, until the
	// delete timeouts.
	ginkgo.By("cleaning up workspaces and templates")
	posixApply("", "workspace-admin-rbac")
	for _, ns := range namespaces {
		if _, err := utils.Run(exec.Command("kubectl", "delete", "workspace", "--all", "-n", ns,
			"--ignore-not-found", "--wait=true", "--timeout=180s",
			"--as="+posixAdminUser, "--as-group="+posixWorkspaceAdminGroup)); err != nil {
			_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "WARNING: deleting the workspaces of %s: %v\n", ns, err)
		}
		if _, err := utils.Run(exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", ns,
			"--ignore-not-found", "--wait=true", "--timeout=60s")); err != nil {
			_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "WARNING: deleting the templates of %s: %v\n", ns, err)
		}
	}
	_, _ = utils.Run(exec.Command("kubectl", "delete", "-f",
		BuildTestResourcePath("workspace-admin-rbac", posixIdentityGroup, ""), "--ignore-not-found"))

	ginkgo.By("cleaning up the PosixIdentity objects of the test users")
	output, _ := kubectlGetAllNamespaces("posixidentity", `{range .items[*]}{.metadata.name} {.spec.username}{"\n"}{end}`)
	for _, line := range strings.Split(output, "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && slices.Contains(users, fields[1]) {
			_ = posixDeleteIdentity(fields[0])
		}
	}

	// Waited for, so that a rerun applying the same namespace does not find it terminating.
	ginkgo.By("deleting the test namespaces")
	for _, ns := range namespaces {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", ns,
			"--ignore-not-found", "--wait=true", "--timeout=180s"))
	}
}

// posixCreateOutputAs creates a fixture of the subgroup as a test user, with extra kubectl create flags
// (for example --dry-run=server or -o jsonpath=...), and returns stdout alone, so that a jsonpath output
// is not mixed with warnings. A failure returns kubectl's stderr in the error, where admission messages
// appear.
func posixCreateOutputAs(subgroup, filename, user string, extra ...string) (string, error) {
	args := append([]string{verbCreate, "-f", BuildTestResourcePath(filename, posixIdentityGroup, subgroup),
		"--as=" + user, "--as-group=" + posixUsersGroup}, extra...)
	cmd := exec.Command("kubectl", args...)
	dir, _ := utils.GetProjectDir()
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "running: kubectl %s\n", strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}
