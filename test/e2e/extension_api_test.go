//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

const (
	extensionAPIGroupDir      = "extension-api"
	extensionAPISubgroupDir   = ""
	extensionAPITestNamespace = "default"
)

var _ = Describe("Extension API", Ordered, func() {
	Context("Setup and registration", func() {

		It("should have extension API service registered and available", func() {
			By("verifying APIService v1alpha1.connection.workspace.jupyter.org is available")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "apiservice", "v1alpha1.connection.workspace.jupyter.org",
					"-o", "jsonpath={.status.conditions[?(@.type=='Available')].status}")
				status, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(status)).To(Equal("True"),
					"APIService v1alpha1.connection.workspace.jupyter.org not available")
			}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		})

		It("should allow authorized user to create ConnectionAccessReview", func() {
			By("creating ConnectionAccessReview without impersonation (admin user)")
			reviewPath := getFixturePath("access-review-basic")
			cmd := exec.Command("kubectl", "create", "-f", reviewPath, "-o", "yaml")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Admin user should be able to create ConnectionAccessReview")

			By("verifying ConnectionAccessReview response contains status fields")
			// ConnectionAccessReview is a review resource - the response is returned immediately
			// with the status populated (similar to SubjectAccessReview)
			Expect(output).To(ContainSubstring("status:"), "Response should contain status section")
			_, _ = fmt.Fprintf(GinkgoWriter, "ConnectionAccessReview created successfully with status:\n%s\n", output)
		})

		It("should deny unauthorized user from creating ConnectionAccessReview", func() {
			By("attempting to create ConnectionAccessReview as unauthorized user via kubectl impersonation")
			reviewPath := getFixturePath("access-review-basic")
			err := createObjectAsUser(reviewPath, "no-connection-access-review-user", []string{})
			Expect(err).To(HaveOccurred(), "Unauthorized user should NOT be able to create ConnectionAccessReview")
			Expect(err.Error()).To(ContainSubstring("forbidden"), "Error should indicate RBAC denial")
		})
	})

	Context("ConnectionAccessReview", func() {
		BeforeAll(func() {
			By("creating RBAC role for workspace creation")
			cmd := exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-creator-role"))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBinding for owner user to create workspaces")
			cmd = exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-creator-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a public workspace for authorization tests")
			createWorkspaceForTest("workspace-public-access", extensionAPIGroupDir, extensionAPISubgroupDir)

			By("creating a private workspace as owner user")
			privateWorkspacePath := getFixturePath("workspace-owner-only-access")
			cmd = exec.Command("kubectl", "create", "-f", privateWorkspacePath,
				"--as=owner-for-access-test-user")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for workspaces to be available")
			WaitForWorkspaceToReachCondition("workspace-public-access", extensionAPITestNamespace, ConditionTypeAvailable, ConditionTrue)
			WaitForWorkspaceToReachCondition("workspace-owner-only-access", extensionAPITestNamespace, ConditionTypeAvailable, ConditionTrue)

			By("creating RBAC role for workspace connection permission")
			cmd = exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-connection-role"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBinding for owner user")
			cmd = exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-connection-owner-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBinding for workspace-users group")
			cmd = exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-connection-group-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			deleteResourcesForExtensionAPITest()
		})

		It("should allow access when user has RBAC permission for workspace connection", func() {
			By("creating ConnectionAccessReview for user with RBAC permission")
			reviewPath := getFixturePath("access-review-rbac-pass")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed")
			Expect(allowed).To(BeTrue(), "User with RBAC permission should be allowed")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "RBAC pass test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should allow access with EKS-style Extra fields for user with RBAC permission", func() {
			By("creating ConnectionAccessReview with EKS Extra fields for user with RBAC permission")
			reviewPath := getFixturePath("access-review-rbac-pass-with-extra")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed with Extra fields present")
			Expect(allowed).To(BeTrue(), "User with RBAC permission and EKS Extra fields should be allowed")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "RBAC pass with EKS Extra test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should deny access when user lacks RBAC permission for workspace connection", func() {
			By("creating ConnectionAccessReview for user without RBAC permission")
			reviewPath := getFixturePath("access-review-rbac-fail")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is denied")
			Expect(allowed).To(BeFalse(), "User without RBAC permission should be denied")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "RBAC fail test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should allow access when user's group has RBAC permission for workspace connection", func() {
			By("creating ConnectionAccessReview for user in authorized group")
			reviewPath := getFixturePath("access-review-group")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed via group membership")
			Expect(allowed).To(BeTrue(), "User in authorized group should be allowed")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "Group RBAC test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should allow access to public workspace for any user with RBAC permission", func() {
			By("creating ConnectionAccessReview for public workspace")
			reviewPath := getFixturePath("access-review-public")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed for public workspace")
			_, _ = fmt.Fprintf(GinkgoWriter, "Public workspace test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
			Expect(allowed).To(BeTrue(), "Any user with RBAC permission should have access to a public workspace")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
		})

		It("should allow access to the owner of a private workspace", func() {
			By("creating ConnectionAccessReview for the owner of the private workspace")
			reviewPath := getFixturePath("access-review-private-owner")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed for the owner user")
			_, _ = fmt.Fprintf(GinkgoWriter, "Private workspace owner test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
			Expect(allowed).To(BeTrue(), "Owner user should have access private workspace")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
		})

		It("should deny access to private workspace for another user than the owner", func() {
			By("creating ConnectionAccessReview for non-owner of a private workspace")
			reviewPath := getFixturePath("access-review-private-non-owner")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is denied for non-owner user")
			Expect(allowed).To(BeFalse(), "Non-owner user should be denied access to private workspace")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "Private workspace non-owner test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should return notFound for non-existent workspace", func() {
			By("creating ConnectionAccessReview for non-existent workspace")
			reviewPath := getFixturePath("access-review-not-found")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace is not found")
			Expect(notFound).To(BeTrue(), "Non-existent workspace should return notFound")
			Expect(allowed).To(BeFalse(), "Access should be denied for non-existent workspace")
			_, _ = fmt.Fprintf(GinkgoWriter, "Workspace not found test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})
	})

	Context("Create Connection", func() {
		BeforeAll(func() {
			By("creating access strategies for connection tests")
			cmd := exec.Command("kubectl", "apply", "-f", getFixturePath("connection-access-strategy"))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("connection-access-strategy-default-handler"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("connection-access-strategy-no-webui"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RBAC for connection tests")
			// Reuse the existing workspace-connection-role (create workspaceconnections permission)
			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("workspace-connection-role"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Reuse workspace-creator-role and bind connection-owner-user to it
			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("workspace-creator-role"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("connection-creator-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("connection-owner-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating public workspace for connection tests")
			createWorkspaceForTest("workspace-connection-public", extensionAPIGroupDir, extensionAPISubgroupDir)

			By("creating workspace with default handler access strategy")
			createWorkspaceForTest("workspace-connection-default-handler", extensionAPIGroupDir, extensionAPISubgroupDir)

			By("creating workspace with no-webui access strategy")
			createWorkspaceForTest("workspace-connection-no-webui", extensionAPIGroupDir, extensionAPISubgroupDir)

			By("creating private workspace as connection-owner-user")
			privatePath := getFixturePath("workspace-connection-private")
			cmd = exec.Command("kubectl", "create", "-f", privatePath, "--as=connection-owner-user")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for workspaces to be available")
			WaitForWorkspaceToReachCondition("workspace-connection-public", extensionAPITestNamespace, ConditionTypeAvailable, ConditionTrue)
			WaitForWorkspaceToReachCondition("workspace-connection-default-handler", extensionAPITestNamespace, ConditionTypeAvailable, ConditionTrue)
			WaitForWorkspaceToReachCondition("workspace-connection-no-webui", extensionAPITestNamespace, ConditionTypeAvailable, ConditionTrue)
			WaitForWorkspaceToReachCondition("workspace-connection-private", extensionAPITestNamespace, ConditionTypeAvailable, ConditionTrue)
		})

		AfterAll(func() {
			deleteResourcesForExtensionAPITest()
		})

		It("should return bearer token URL for public workspace", func() {
			By("creating WorkspaceConnection for public workspace")
			connType, connURL, err := createWorkspaceConnectionAndGetResponse(
				getFixturePath("connection-request-webui"))
			Expect(err).NotTo(HaveOccurred())

			By("verifying response contains web-ui connection type")
			Expect(connType).To(Equal("web-ui"))

			By("verifying connection URL contains a token")
			Expect(connURL).To(ContainSubstring("?token="))
			Expect(connURL).To(ContainSubstring("bearer-auth"))
			_, _ = fmt.Fprintf(GinkgoWriter, "Connection URL: %s\n", connURL)
		})

		It("should return a valid JWT in the bearer token URL", func() {
			By("creating WorkspaceConnection and extracting JWT")
			_, connURL, err := createWorkspaceConnectionAndGetResponse(
				getFixturePath("connection-request-webui"))
			Expect(err).NotTo(HaveOccurred())

			By("parsing token from URL")
			parsed, err := url.Parse(connURL)
			Expect(err).NotTo(HaveOccurred())
			token := parsed.Query().Get("token")
			Expect(token).NotTo(BeEmpty(), "Token should be present in URL query params")

			By("validating JWT structure (header.payload.signature)")
			parts := strings.Split(token, ".")
			Expect(parts).To(HaveLen(3), "JWT should have 3 parts separated by dots")

			By("decoding and validating JWT header")
			headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
			Expect(err).NotTo(HaveOccurred(), "JWT header should be valid base64url")

			var header map[string]interface{}
			err = json.Unmarshal(headerJSON, &header)
			Expect(err).NotTo(HaveOccurred(), "JWT header should be valid JSON")

			Expect(header).To(HaveKey("alg"))
			Expect(header["alg"]).To(Equal("HS384"), "JWT should use HS384 algorithm")
			Expect(header).To(HaveKey("kid"))
			Expect(header["kid"]).NotTo(BeEmpty(), "JWT should have a key ID")
			_, _ = fmt.Fprintf(GinkgoWriter, "JWT header: alg=%s, kid=%s\n", header["alg"], header["kid"])
		})

		It("should work with empty createConnectionHandler (default)", func() {
			By("creating WorkspaceConnection for workspace with default handler")
			connType, connURL, err := createWorkspaceConnectionAndGetResponse(
				getFixturePath("connection-request-default-handler"))
			Expect(err).NotTo(HaveOccurred())

			By("verifying response is valid")
			Expect(connType).To(Equal("web-ui"))
			Expect(connURL).To(ContainSubstring("?token="))
			_, _ = fmt.Fprintf(GinkgoWriter, "Default handler connection URL: %s\n", connURL)
		})

		It("should allow owner to create connection for private workspace", func() {
			By("creating WorkspaceConnection as owner user")
			output, err := createWorkspaceConnectionAsUser(
				getFixturePath("connection-request-private"), "connection-owner-user", []string{})
			Expect(err).NotTo(HaveOccurred(), "Owner should be able to create connection for private workspace")
			Expect(output).To(ContainSubstring("workspaceConnectionUrl"))
			Expect(output).To(ContainSubstring("?token="))
			_, _ = fmt.Fprintf(GinkgoWriter, "Owner connection response:\n%s\n", output)
		})

		It("should deny non-owner connection to private workspace", func() {
			By("creating WorkspaceConnection as non-owner user")
			_, err := createWorkspaceConnectionAsUser(
				getFixturePath("connection-request-private"), "connection-other-user", []string{})
			Expect(err).To(HaveOccurred(), "Non-owner should be denied connection to private workspace")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("Forbidden"),
				ContainSubstring("forbidden"),
				ContainSubstring("not the workspace owner"),
			))
		})

		It("should reject connection when WebUI not enabled", func() {
			By("creating WorkspaceConnection for workspace without bearerAuthURLTemplate")
			_, err := createWorkspaceConnectionAsUser(
				getFixturePath("connection-request-no-webui"), "connection-owner-user", []string{})
			Expect(err).To(HaveOccurred(), "Connection should be rejected when WebUI is not enabled")
			Expect(err.Error()).To(ContainSubstring("web browser access is not enabled"))
		})

		It("should reject connection for non-existent workspace", func() {
			By("creating WorkspaceConnection for non-existent workspace")
			_, err := createWorkspaceConnectionAsUser(
				getFixturePath("connection-request-not-found"), "connection-owner-user", []string{})
			Expect(err).To(HaveOccurred(), "Connection to non-existent workspace should fail")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("not found"),
				ContainSubstring("NotFound"),
			))
		})

		It("should deny unauthorized user from creating connection", func() {
			By("creating WorkspaceConnection as user without RBAC permissions")
			_, err := createWorkspaceConnectionAsUser(
				getFixturePath("connection-request-webui"), "unauthorized-user", []string{})
			Expect(err).To(HaveOccurred(), "Unauthorized user should be denied")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("Forbidden"),
				ContainSubstring("forbidden"),
			))
		})

		It("should use a new signing key after JWT secret rotation", func() {
			By("creating a connection to capture the initial kid")
			_, connURL1, err := createWorkspaceConnectionAndGetResponse(
				getFixturePath("connection-request-webui"))
			Expect(err).NotTo(HaveOccurred())

			kid1, err := extractKidFromConnectionURL(connURL1)
			Expect(err).NotTo(HaveOccurred())
			_, _ = fmt.Fprintf(GinkgoWriter, "Initial kid: %s\n", kid1)

			By("triggering JWT key rotation via CronJob")
			cmd := exec.Command("kubectl", "create", "job",
				"--from=cronjob/jupyter-k8s-jwt-rotator",
				"jwt-rotation-e2e", "-n", OperatorNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for rotation job to complete")
			cmd = exec.Command("kubectl", "wait", "job/jwt-rotation-e2e",
				"-n", OperatorNamespace,
				"--for=condition=complete", "--timeout=60s")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Rotation job did not complete")

			By("waiting for controller to pick up rotated key and start signing with new kid")
			// The controller's informer detects the secret change, then newKeyUseDelay
			// (default 5s) must elapse before the new key is used for signing.
			Eventually(func(g Gomega) {
				_, connURL2, err := createWorkspaceConnectionAndGetResponse(
					getFixturePath("connection-request-webui"))
				g.Expect(err).NotTo(HaveOccurred())

				kid2, err := extractKidFromConnectionURL(connURL2)
				g.Expect(err).NotTo(HaveOccurred())

				_, _ = fmt.Fprintf(GinkgoWriter, "Polling kid: %s (waiting for != %s)\n", kid2, kid1)
				g.Expect(kid2).NotTo(Equal(kid1), "Expected new kid after rotation")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("cleaning up rotation job")
			cmd = exec.Command("kubectl", "delete", "job", "jwt-rotation-e2e",
				"-n", OperatorNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})

	Context("Create BearerTokenReview", func() {
		BeforeAll(func() {
			By("creating access strategy for bearer token review tests")
			cmd := exec.Command("kubectl", "apply", "-f", getFixturePath("bearer-review-access-strategy"))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RBAC roles for bearer token review and connection creation")
			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("bearer-review-role"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("bearer-review-connection-role"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBindings for bearer-review-user")
			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("bearer-review-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", getFixturePath("bearer-review-connection-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace for bearer token review tests")
			createWorkspaceForTest("workspace-bearer-review", extensionAPIGroupDir, extensionAPISubgroupDir)

			By("waiting for workspace to be available")
			WaitForWorkspaceToReachCondition("workspace-bearer-review", extensionAPITestNamespace, ConditionTypeAvailable, ConditionTrue)
		})

		AfterAll(func() {
			deleteResourcesForExtensionAPITest()
		})

		It("should verify a valid bearer token and return correct claims", func() {
			By("creating a WorkspaceConnection to obtain a bearer token")
			_, connURL, err := createWorkspaceConnectionAndGetResponse(
				getFixturePath("bearer-review-connection-request"))
			Expect(err).NotTo(HaveOccurred())

			token, err := extractTokenFromConnectionURL(connURL)
			Expect(err).NotTo(HaveOccurred())

			By("creating BearerTokenReview with the valid token")
			result, err := createBearerTokenReview(token, "")
			Expect(err).NotTo(HaveOccurred())

			By("verifying the response contains correct authenticated identity")
			Expect(result.Authenticated).To(BeTrue(), "Valid bearer token should be authenticated")
			Expect(result.Username).NotTo(BeEmpty(), "Username should be populated")
			Expect(result.Path).To(ContainSubstring("workspace-bearer-review"),
				"Path should reference the workspace")
			Expect(result.Domain).To(Equal("example.com"), "Domain should match access strategy template")
			Expect(result.Error).To(BeEmpty(), "Error should be empty for valid token")
			_, _ = fmt.Fprintf(GinkgoWriter,
				"BearerTokenReview happy path: authenticated=%v, user=%s, path=%s, domain=%s\n",
				result.Authenticated, result.Username, result.Path, result.Domain)
		})

		It("should reject an invalid token", func() {
			By("creating BearerTokenReview with a garbage token")
			result, err := createBearerTokenReview("invalid-token-string", "")
			Expect(err).NotTo(HaveOccurred())

			By("verifying the token is not authenticated")
			Expect(result.Authenticated).To(BeFalse(), "Invalid token should not be authenticated")
			Expect(result.Error).To(Equal("invalid or expired token"))
		})

		It("should deny user without RBAC permission to create BearerTokenReview", func() {
			By("creating BearerTokenReview as user with no bearertokenreviews RBAC")
			_, err := createBearerTokenReview("any-token", "no-bearer-review-user")
			Expect(err).To(HaveOccurred(), "User without RBAC should be denied")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("Forbidden"),
				ContainSubstring("forbidden"),
			))
		})

		It("should allow authorized user to create BearerTokenReview via RBAC", func() {
			By("obtaining a bearer token")
			_, connURL, err := createWorkspaceConnectionAndGetResponse(
				getFixturePath("bearer-review-connection-request"))
			Expect(err).NotTo(HaveOccurred())

			token, err := extractTokenFromConnectionURL(connURL)
			Expect(err).NotTo(HaveOccurred())

			By("creating BearerTokenReview as bearer-review-user (has bearertokenreviews RBAC)")
			result, err := createBearerTokenReview(token, "bearer-review-user")
			Expect(err).NotTo(HaveOccurred())

			By("verifying the authorized user gets a valid response")
			Expect(result.Authenticated).To(BeTrue(), "Authorized user should get authenticated response")
			Expect(result.Username).NotTo(BeEmpty())
			_, _ = fmt.Fprintf(GinkgoWriter,
				"BearerTokenReview RBAC pass: user=%s, path=%s\n", result.Username, result.Path)
		})

		It("should still verify tokens after JWT key rotation", func() {
			By("obtaining a bearer token before rotation")
			_, connURL1, err := createWorkspaceConnectionAndGetResponse(
				getFixturePath("bearer-review-connection-request"))
			Expect(err).NotTo(HaveOccurred())

			preRotationToken, err := extractTokenFromConnectionURL(connURL1)
			Expect(err).NotTo(HaveOccurred())

			preRotationKid, err := extractKidFromConnectionURL(connURL1)
			Expect(err).NotTo(HaveOccurred())
			_, _ = fmt.Fprintf(GinkgoWriter, "Pre-rotation kid: %s\n", preRotationKid)

			By("triggering JWT key rotation")
			cmd := exec.Command("kubectl", "create", "job",
				"--from=cronjob/jupyter-k8s-jwt-rotator",
				"jwt-rotation-bearer-review-e2e", "-n", OperatorNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "wait", "job/jwt-rotation-bearer-review-e2e",
				"-n", OperatorNamespace,
				"--for=condition=complete", "--timeout=60s")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Rotation job did not complete")

			By("waiting for controller to start signing with new key")
			var postRotationToken string
			Eventually(func(g Gomega) {
				_, connURL2, err := createWorkspaceConnectionAndGetResponse(
					getFixturePath("bearer-review-connection-request"))
				g.Expect(err).NotTo(HaveOccurred())

				kid2, err := extractKidFromConnectionURL(connURL2)
				g.Expect(err).NotTo(HaveOccurred())

				_, _ = fmt.Fprintf(GinkgoWriter, "Polling kid: %s (waiting for != %s)\n", kid2, preRotationKid)
				g.Expect(kid2).NotTo(Equal(preRotationKid), "Expected new kid after rotation")

				postRotationToken, err = extractTokenFromConnectionURL(connURL2)
				g.Expect(err).NotTo(HaveOccurred())
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("verifying pre-rotation token is still valid (old key retained)")
			result1, err := createBearerTokenReview(preRotationToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.Authenticated).To(BeTrue(),
				"Pre-rotation token should still be valid (old key retained)")
			_, _ = fmt.Fprintf(GinkgoWriter, "Pre-rotation token still valid: user=%s\n", result1.Username)

			By("verifying post-rotation token is also valid (new key active)")
			result2, err := createBearerTokenReview(postRotationToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.Authenticated).To(BeTrue(),
				"Post-rotation token should be valid (new key active)")
			_, _ = fmt.Fprintf(GinkgoWriter, "Post-rotation token valid: user=%s\n", result2.Username)

			By("cleaning up rotation job")
			cmd = exec.Command("kubectl", "delete", "job", "jwt-rotation-bearer-review-e2e",
				"-n", OperatorNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})
})

// deleteResourcesForExtensionAPITest cleans up resources created during extension API tests
func deleteResourcesForExtensionAPITest() {
	GinkgoHelper()

	// ConnectionAccessReview resources are not persisted in etcd, so no cleanup needed

	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", extensionAPITestNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up access strategies")
	cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	By("cleaning up RBAC resources by label")
	cmd = exec.Command("kubectl", "delete", "rolebinding", "-l", "jk8s/e2e=extension-api-test",
		"-n", extensionAPITestNamespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	cmd = exec.Command("kubectl", "delete", "role", "-l", "jk8s/e2e=extension-api-test",
		"-n", extensionAPITestNamespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
