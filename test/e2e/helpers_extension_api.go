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

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
	"github.com/onsi/ginkgo/v2"
)

// getFixturePath constructs the file path for extension API test fixtures
func getFixturePath(filename string) string {
	return BuildTestResourcePath(filename, extensionAPIGroupDir, extensionAPISubgroupDir)
}

// createConnectionAccessReviewAndGetStatus creates a ConnectionAccessReview and parses the response status
func createConnectionAccessReviewAndGetStatus(filepath string) (allowed bool, notFound bool, reason string, err error) {
	ginkgo.GinkgoHelper()
	cmd := exec.Command("kubectl", "create", "-f", filepath, "-o", "yaml")
	output, createErr := utils.Run(cmd)
	if createErr != nil {
		return false, false, "", createErr
	}

	// Parse the YAML output to extract status fields
	// Look for "allowed:", "notFound:", and "reason:" in the output
	allowed = strings.Contains(output, "allowed: true")
	notFound = strings.Contains(output, "notFound: true")

	// Extract reason field using a simple approach
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "reason:") {
			reason = strings.TrimSpace(strings.TrimPrefix(trimmed, "reason:"))
			break
		}
	}

	return allowed, notFound, reason, nil
}

// createWorkspaceConnectionAndGetResponse creates a WorkspaceConnection and parses the response
// to extract workspaceConnectionType and workspaceConnectionUrl from the status.
func createWorkspaceConnectionAndGetResponse(filepath string) (connType, connURL string, err error) {
	ginkgo.GinkgoHelper()
	cmd := exec.Command("kubectl", "create", "-f", filepath, "-o", "yaml")
	output, createErr := utils.Run(cmd)
	if createErr != nil {
		return "", "", createErr
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "workspaceConnectionType:") {
			connType = strings.TrimSpace(strings.TrimPrefix(trimmed, "workspaceConnectionType:"))
		}
		if strings.HasPrefix(trimmed, "workspaceConnectionUrl:") {
			connURL = strings.TrimSpace(strings.TrimPrefix(trimmed, "workspaceConnectionUrl:"))
		}
	}

	return connType, connURL, nil
}

// createWorkspaceConnectionAsUser creates a WorkspaceConnection with kubectl impersonation,
// returning the raw output and error.
func createWorkspaceConnectionAsUser(filepath, user string, groups []string) (string, error) {
	ginkgo.GinkgoHelper()
	args := []string{"create", "-f", filepath, "-o", "yaml", "--as=" + user}
	for _, group := range groups {
		args = append(args, "--as-group="+group)
	}
	cmd := exec.Command("kubectl", args...)
	return utils.Run(cmd)
}

// extractKidFromConnectionURL parses a connection URL's ?token= query param
// and returns the "kid" field from the JWT header.
func extractKidFromConnectionURL(connURL string) (string, error) {
	parsed, err := url.Parse(connURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}
	token := parsed.Query().Get("token")
	if token == "" {
		return "", fmt.Errorf("no token query param in URL")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("JWT has %d parts, expected 3", len(parts))
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT header: %w", err)
	}
	var header map[string]interface{}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return "", fmt.Errorf("failed to parse JWT header JSON: %w", err)
	}
	kid, ok := header["kid"].(string)
	if !ok || kid == "" {
		return "", fmt.Errorf("JWT header missing 'kid' field")
	}
	return kid, nil
}

// bearerTokenReviewResult holds the parsed status from a BearerTokenReview response.
type bearerTokenReviewResult struct {
	Authenticated bool
	Username      string
	Groups        []string
	Path          string
	Domain        string
	Error         string
}

// createBearerTokenReview creates a BearerTokenReview via kubectl --raw and returns the parsed result.
// If asUser is non-empty, kubectl impersonation is used.
func createBearerTokenReview(token, asUser string) (*bearerTokenReviewResult, error) {
	ginkgo.GinkgoHelper()
	ns := extensionAPITestNamespace
	body := fmt.Sprintf(`{"apiVersion":"connection.workspace.jupyter.org/v1alpha1","kind":"BearerTokenReview","metadata":{"namespace":"%s"},"spec":{"token":"%s"}}`, ns, token)
	apiPath := fmt.Sprintf("/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/%s/bearertokenreviews", ns)

	args := []string{"create", "--raw", apiPath, "-f", "-"}
	if asUser != "" {
		args = append(args, "--as="+asUser)
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Stdin = strings.NewReader(body)
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	status, _ := raw["status"].(map[string]interface{})
	if status == nil {
		return nil, fmt.Errorf("response missing status field")
	}

	result := &bearerTokenReviewResult{}
	result.Authenticated, _ = status["authenticated"].(bool)
	result.Path, _ = status["path"].(string)
	result.Domain, _ = status["domain"].(string)
	result.Error, _ = status["error"].(string)
	if user, ok := status["user"].(map[string]interface{}); ok {
		result.Username, _ = user["username"].(string)
		if groups, ok := user["groups"].([]interface{}); ok {
			for _, g := range groups {
				if s, ok := g.(string); ok {
					result.Groups = append(result.Groups, s)
				}
			}
		}
	}

	return result, nil
}

// extractTokenFromConnectionURL parses a connection URL and returns the token query param.
func extractTokenFromConnectionURL(connURL string) (string, error) {
	parsed, err := url.Parse(connURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}
	token := parsed.Query().Get("token")
	if token == "" {
		return "", fmt.Errorf("no token query param in URL")
	}
	return token, nil
}

// updateObjectAsUser updates a Kubernetes object with kubectl impersonation
func updateObjectAsUser(filepath, user string, groups []string) error {
	ginkgo.GinkgoHelper()
	args := []string{"apply", "-f", filepath, "--as=" + user}
	for _, group := range groups {
		args = append(args, "--as-group="+group)
	}
	cmd := exec.Command("kubectl", args...)
	_, err := utils.Run(cmd)
	return err
}
