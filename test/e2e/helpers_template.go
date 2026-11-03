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

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// valueTestCaseForTemplateTest is a jsonPath expectation checked by testTemplateFeaturesInheritance
type valueTestCaseForTemplateTest struct {
	description string
	jsonPath    string
	expected    string
}

func deleteResourcesForTemplateTest() {
	ginkgo.By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	ginkgo.By("cleaning up templates")
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	ginkgo.By("cleaning up access strategies")
	cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	ginkgo.By("cleaning up standalone PVCs")
	cmd = exec.Command("kubectl", "delete", "pvc", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	ginkgo.By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}

// Helper function to test template feature inheritance
//
//nolint:unparam
func testTemplateFeaturesInheritance(
	workspaceName string,
	workspaceNamespace string,
	testCases []valueTestCaseForTemplateTest,
) {
	// Test each value case
	for _, tc := range testCases {
		ginkgo.By(tc.description)
		output, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
			tc.jsonPath)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(output).To(gomega.Equal(tc.expected))
	}
}
