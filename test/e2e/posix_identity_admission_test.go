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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/posixidentity"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// Admission of identity-templated workspaces, against the real API server and the operator's cache.
// Workspaces are created stopped: no spec here schedules a pod.
var _ = Describe("POSIX Identity Admission", Ordered, func() {
	const (
		subgroup      = "admission"
		namespace     = "posix-adm-team"
		lateNamespace = "posix-adm-late"
		dupNamespace  = "posix-adm-dup"
		namespaceGID  = "2130706510"
		lateGID       = "2130706511"
		alice         = "posix-adm-alice"
		dryRunUser    = "posix-adm-dryrun"
		seededUser    = "posix-adm-seeded"
		raceRounds    = 3
		jsonPathUID   = "jsonpath={.spec.podSecurityContext.runAsUser}"
	)
	raceUser := func(round int) string { return fmt.Sprintf("posix-adm-race-%d", round) }

	users := []string{alice, dryRunUser, seededUser}
	for round := 1; round <= raceRounds; round++ {
		users = append(users, raceUser(round))
	}

	// patchAs patches a workspace as a test user and returns kubectl's output.
	patchAs := func(workspace, user, patch string) (string, error) {
		return utils.Run(exec.Command("kubectl", "patch", "workspace", workspace, "-n", namespace,
			"--type=merge", "-p", patch, "--as="+user, "--as-group="+posixUsersGroup))
	}

	BeforeAll(func() {
		posixApplyBase(subgroup, "out-of-range-template")
	})

	AfterAll(func() {
		cleanupPosixIdentityTest([]string{namespace, lateNamespace, dupNamespace}, users)
	})

	Context("Rejections", func() {
		It("rejects a template referencing an unknown input", func() {
			_, err := utils.Run(exec.Command("kubectl", "apply", "-f",
				BuildTestResourcePath("invalid-template", posixIdentityGroup, subgroup)))
			Expect(err).To(MatchError(ContainSubstring("defaultPodSecurityContext.runAsUser")))
		})

		It("rejects a workspace picking its own uid", func() {
			Expect(posixCreateAs(subgroup, "custom-uid-workspace", alice)).To(
				MatchError(ContainSubstring("spec.podSecurityContext.runAsUser: must be " + namespaceGID)))
		})

		It("rejects a container picking its own uid, though the template locks no security context", func() {
			Expect(posixCreateAs(subgroup, "custom-container-uid-workspace", alice)).To(
				MatchError(ContainSubstring("spec.containerSecurityContext.runAsUser: must be unset")))
		})

		It("rejects a workspace whose template resolves to a reserved id", func() {
			Expect(posixCreateAs(subgroup, "out-of-range-workspace", alice)).To(
				MatchError(ContainSubstring("resolved id 0")))
		})

		It("rejects an update changing the uid or adding the posix-uid label", func() {
			uid, err := posixCreateOutputAs(subgroup, "alice-updates-workspace", alice, "-o", jsonPathUID)
			Expect(err).NotTo(HaveOccurred())

			By("changing runAsUser")
			_, err = patchAs("alice-updates", alice, `{"spec":{"podSecurityContext":{"runAsUser":2047113}}}`)
			Expect(err).To(MatchError(ContainSubstring("spec.podSecurityContext.runAsUser: must be " + uid)))

			By("adding the posix-uid label to a workspace of a shared template, which carries none")
			_, err = posixCreateOutputAs(subgroup, "alice-shared-updates-workspace", alice)
			Expect(err).NotTo(HaveOccurred())
			_, err = patchAs("alice-shared-updates", alice,
				fmt.Sprintf(`{"metadata":{"labels":{%q:"2047113"}}}`, workspaceutil.LabelPosixUID))
			Expect(err).To(MatchError(ContainSubstring("is set by the operator only")))
		})
	})

	Context("Allocation", func() {
		It("resolves a uid on a dry-run create without allocating it", func() {
			uid, err := posixCreateOutputAs(subgroup, "dry-run-workspace", dryRunUser, "--dry-run=server", "-o", jsonPathUID)
			Expect(err).NotTo(HaveOccurred())
			n, err := strconv.ParseInt(uid, 10, 64)
			Expect(err).NotTo(HaveOccurred())
			Expect(posixidentity.InUserRange(n)).To(BeTrue(), "uid %d", n)
			Expect(posixIdentitiesOf(dryRunUser)).To(BeEmpty())
		})

		// Each round races the first two creates of a new user. Both requests miss the user in the cache
		// and try the same first candidate; the one losing the create reads the winner's object back from
		// the API server. A round that happens to serialize takes the cached path, with the same outcome.
		It("gives one uid to a new user creating two workspaces at once", func() {
			for round := 1; round <= raceRounds; round++ {
				user := raceUser(round)
				By(fmt.Sprintf("racing two first creates of %s", user))
				uids, errs := make([]string, 2), make([]error, 2)
				start := make(chan struct{})
				var wg sync.WaitGroup
				for i := range 2 {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()
						<-start
						fixture := fmt.Sprintf("race-%d-%d-workspace", round, i)
						uids[i], errs[i] = posixCreateOutputAs(subgroup, fixture, user, "-o", jsonPathUID)
					}()
				}
				close(start)
				wg.Wait()

				for _, err := range errs {
					Expect(err).NotTo(HaveOccurred())
				}
				Expect(uids[0]).NotTo(BeEmpty())
				Expect(uids[1]).To(Equal(uids[0]))
				Expect(posixIdentitiesOf(user)).To(ConsistOf("uid-" + uids[0]))
			}
		})

		It("runs a user as the uid of a PosixIdentity an administrator created beforehand", func() {
			const seededUID = posixidentity.UserRangeMin + 4242
			want := strconv.FormatInt(seededUID, 10)
			posixApply(subgroup, "seeded-identity")
			hash, err := kubectlGet("posixidentity", posixidentity.ObjectName(seededUID), "", "{.spec.usernameHash}")
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(Equal(posixidentity.UsernameHash(seededUser)), "seeded-identity.yaml has a stale usernameHash")

			// The operator looks users up in its cache: dry-run until it holds the object, so the real create
			// does not allocate a second uid.
			Eventually(func(g Gomega) {
				uid, err := posixCreateOutputAs(subgroup, "seeded-workspace", seededUser, "--dry-run=server", "-o", jsonPathUID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(uid).To(Equal(want))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			uid, err := posixCreateOutputAs(subgroup, "seeded-workspace", seededUser, "-o", jsonPathUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(uid).To(Equal(want))
			Expect(posixIdentitiesOf(seededUser)).To(ConsistOf(posixidentity.ObjectName(seededUID)))
		})

		It("lets users neither create a PosixIdentity nor anyone change one", func() {
			By("creating one as a user")
			_, err := posixCreateOutputAs(subgroup, "user-identity", alice)
			Expect(err).To(MatchError(ContainSubstring("forbidden")))

			By("changing the spec of one as an administrator")
			uids := posixIdentitiesOf(seededUser)
			Expect(uids).To(HaveLen(1))
			_, err = utils.Run(exec.Command("kubectl", "patch", "posixidentity", uids[0],
				"--type=merge", "-p", `{"spec":{"unixName":"someone-else"}}`))
			Expect(err).To(MatchError(ContainSubstring("spec is immutable")))
		})
	})

	// The operator reads namespaces from a cache scoped to the posix-gid label, and finds a gid's
	// namespaces through a field index: both must follow label changes made after it started.
	Context("Namespace gid", Ordered, func() {
		lateCreate := func() (string, error) {
			return posixCreateOutputAs(subgroup, "late-workspace", alice, "--dry-run=server",
				"-o", "jsonpath={.spec.podSecurityContext.supplementalGroups[0]}")
		}
		label := func(ns string) {
			GinkgoHelper()
			_, err := utils.Run(exec.Command("kubectl", "label", "namespace", ns, "--overwrite",
				workspaceutil.LabelPosixGID+"="+lateGID))
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeAll(func() {
			posixApply(subgroup, "namespace-late")
			posixApply(subgroup, "late-private-template")
		})

		It("rejects a workspace in a namespace without a gid, then admits it once the deployer labels it", func() {
			_, err := lateCreate()
			Expect(err).To(MatchError(ContainSubstring("has no " + workspaceutil.LabelPosixGID + " label")))

			label(lateNamespace)
			Eventually(func(g Gomega) {
				gid, err := lateCreate()
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(gid).To(Equal(lateGID))
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})

		It("rejects a workspace in a namespace whose gid another namespace also carries", func() {
			posixApply(subgroup, "namespace-dup")
			label(dupNamespace)
			Eventually(func(g Gomega) {
				_, err := lateCreate()
				g.Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("namespaces %s, %s share %s=%s",
					dupNamespace, lateNamespace, workspaceutil.LabelPosixGID, lateGID))))
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})
	})
})
