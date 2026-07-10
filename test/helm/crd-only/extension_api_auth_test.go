/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package crdonly_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Extension API Auth RBAC", func() {

	It("should use ClusterRole and ClusterRoleBinding instead of kube-system RoleBinding", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		templatePath := filepath.Join(rootDir, "dist", "chart", "templates", "rbac", "extension-api-auth-binding.yaml")
		data, err := os.ReadFile(templatePath)
		Expect(err).NotTo(HaveOccurred(), "Failed to read extension-api-auth-binding.yaml")

		content := string(data)

		By("not creating resources in kube-system namespace")
		Expect(content).NotTo(ContainSubstring("namespace: kube-system"),
			"extension-api-auth-binding should not create resources in kube-system; "+
				"EKS addon framework cannot create cross-namespace resources")

		By("using ClusterRole for configmap access")
		Expect(content).To(ContainSubstring("kind: ClusterRole"),
			"should use ClusterRole instead of referencing kube-system Role")

		By("using ClusterRoleBinding")
		Expect(content).To(ContainSubstring("kind: ClusterRoleBinding"),
			"should use ClusterRoleBinding instead of RoleBinding in kube-system")

		By("granting read access to extension-apiserver-authentication configmap")
		Expect(content).To(ContainSubstring("extension-apiserver-authentication"),
			"should reference the extension-apiserver-authentication configmap")

		By("being gated on extensionApi.enable")
		Expect(content).To(ContainSubstring(".Values.extensionApi.enable"),
			"should be conditional on extensionApi.enable")
	})

	It("should render ClusterRole and ClusterRoleBinding in helm output", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		outputDir := filepath.Join(rootDir, "dist", "test-output-crd-only", "jupyter-k8s", "templates", "rbac")
		entries, err := os.ReadDir(outputDir)
		Expect(err).NotTo(HaveOccurred())

		foundClusterRole := false
		foundClusterRoleBinding := false

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(outputDir, entry.Name()))
			if err != nil {
				continue
			}
			content := string(data)
			if strings.Contains(content, "read-auth-configmap") {
				if strings.Contains(content, "kind: ClusterRole") && !strings.Contains(content, "kind: ClusterRoleBinding") {
					foundClusterRole = true
				}
				if strings.Contains(content, "kind: ClusterRoleBinding") {
					foundClusterRoleBinding = true
				}
			}
		}

		Expect(foundClusterRole).To(BeTrue(), "Expected ClusterRole with read-auth-configmap in rendered output")
		Expect(foundClusterRoleBinding).To(BeTrue(), "Expected ClusterRoleBinding with read-auth-configmap in rendered output")
	})
})
