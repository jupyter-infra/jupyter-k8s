//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"os/exec"
	"strings"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
	"github.com/onsi/ginkgo/v2"
)

// createConnectionAccessReviewAsUser creates a ConnectionAccessReview with kubectl impersonation
func createConnectionAccessReviewAsUser(filepath, user string, groups []string) error {
	ginkgo.GinkgoHelper()
	args := []string{"create", "-f", filepath, "--as=" + user}
	for _, group := range groups {
		args = append(args, "--as-group="+group)
	}
	cmd := exec.Command("kubectl", args...)
	_, err := utils.Run(cmd)
	return err
}

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
