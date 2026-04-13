/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_hyperpod_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"

	"github.com/jupyter-infra/jupyter-k8s/test/helm"
)

var _ = Describe("AWS HyperPod Resources", func() {
	It("should include all aws-hyperpod resources in the Helm chart", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		// Parse resources from source directory
		configDir := filepath.Join(rootDir, "guided-charts", "aws-hyperpod", "templates")
		configResources, configMap, err := helm.ParseYAMLDirectory(configDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(configResources).NotTo(BeEmpty(), "No resources found in guided-charts directory")

		// Parse resources from target directory
		helmDir := filepath.Join(
			rootDir, "dist", "test-output-guided", "aws-hyperpod", "jupyter-k8s-aws-hyperpod", "templates")
		helmResources, helmMap, err := helm.ParseYAMLDirectory(helmDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(helmResources).NotTo(BeEmpty(), "No resources found in output directory")

		// Compare resources
		missingResources, unmatchedResources, _ := helm.CompareResourceMaps(
			configResources, configMap, helmResources, helmMap)

		Expect(missingResources).To(BeEmpty(), "Resources from source chart missing in output: %v", missingResources)

		if len(unmatchedResources) > 0 {
			GinkgoWriter.Println("Note: Multiple resources generated from template files (expected):")
			for _, res := range unmatchedResources {
				GinkgoWriter.Printf("  - %s\n", res)
			}
		}
	})

	It("should generate JWT signing key with 48 bytes (384 bits) for HS384", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		secretPath := filepath.Join(
			rootDir, "dist", "test-output-guided", "aws-hyperpod", "jupyter-k8s-aws-hyperpod",
			"templates", "authmiddleware", "secrets.yaml")

		secretBytes, err := os.ReadFile(secretPath)
		Expect(err).NotTo(HaveOccurred())

		var secret corev1.Secret
		err = yaml.Unmarshal(secretBytes, &secret)
		Expect(err).NotTo(HaveOccurred())

		var keyValue []byte
		var keyName string
		for name, value := range secret.Data {
			if strings.HasPrefix(name, "jwt-signing-key-") {
				keyValue = value
				keyName = name
				break
			}
		}

		Expect(keyName).NotTo(BeEmpty(), "No JWT signing key found in secret")
		Expect(keyValue).To(HaveLen(48),
			"JWT signing key %s should be 48 bytes for HS384, but got %d bytes", keyName, len(keyValue))
	})

	It("should include bearertokenreviews RBAC when enableBearerAuth is true", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		rbacPath := filepath.Join(
			rootDir, "dist", "test-output-guided", "aws-hyperpod", "jupyter-k8s-aws-hyperpod",
			"templates", "authmiddleware", "rbac.yaml")

		rbacBytes, err := os.ReadFile(rbacPath)
		Expect(err).NotTo(HaveOccurred())

		// Find the ClusterRole among multiple YAML documents
		docs := strings.Split(string(rbacBytes), "---")
		var clusterRole rbacv1.ClusterRole
		found := false
		for _, doc := range docs {
			trimmed := strings.TrimSpace(doc)
			if trimmed == "" {
				continue
			}
			if !strings.Contains(trimmed, "kind: ClusterRole") ||
				strings.Contains(trimmed, "kind: ClusterRoleBinding") {
				continue
			}
			err = yaml.Unmarshal([]byte(trimmed), &clusterRole)
			Expect(err).NotTo(HaveOccurred())
			found = true
			break
		}
		Expect(found).To(BeTrue(), "ClusterRole not found in authmiddleware rbac.yaml")

		resourceNames := []string{}
		for _, rule := range clusterRole.Rules {
			resourceNames = append(resourceNames, rule.Resources...)
		}
		Expect(resourceNames).To(ContainElement("connectionaccessreviews"),
			"ClusterRole should grant access to connectionaccessreviews")
		Expect(resourceNames).To(ContainElement("bearertokenreviews"),
			"ClusterRole should grant access to bearertokenreviews when enableBearerAuth=true")
	})

	It("should render access strategy with correct plugin handler references", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		accessStrategyPath := filepath.Join(
			rootDir, "dist", "test-output-guided", "aws-hyperpod", "jupyter-k8s-aws-hyperpod",
			"templates", "hyperpod-access-strategy.yaml")

		data, err := os.ReadFile(accessStrategyPath)
		Expect(err).NotTo(HaveOccurred())
		content := string(data)

		// Verify plugin handler references
		Expect(content).To(ContainSubstring("podEventsHandler: \"aws:ssm-remote-access\""),
			"AccessStrategy should have podEventsHandler referencing aws plugin")
		Expect(content).To(ContainSubstring("vscode-remote: \"aws:createSession\""),
			"AccessStrategy should have createConnectionHandlerMap entry for vscode-remote")

		// Verify k8s-native is the default connection handler
		Expect(content).To(ContainSubstring("createConnectionHandler: \"k8s-native\""),
			"AccessStrategy should have k8s-native as default createConnectionHandler")

		// Verify podEventsContext has required keys
		Expect(content).To(ContainSubstring("sidecarContainerName:"),
			"podEventsContext should include sidecarContainerName")
		Expect(content).To(ContainSubstring("registrationStateFile:"),
			"podEventsContext should include registrationStateFile")
		Expect(content).To(ContainSubstring("controller::PodUid()"),
			"podEventsContext should use dynamic PodUid resolution")

		// Verify createConnectionContext has required keys
		Expect(content).To(ContainSubstring("extensionapi::PodUid()"),
			"createConnectionContext should use dynamic PodUid resolution")
		Expect(content).To(ContainSubstring("ssmDocumentName:"),
			"createConnectionContext should include ssmDocumentName")
	})

	It("should render authmiddleware JWT env vars consistent with values", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		deploymentPath := filepath.Join(
			rootDir, "dist", "test-output-guided", "aws-hyperpod", "jupyter-k8s-aws-hyperpod",
			"templates", "authmiddleware", "deployment.yaml")

		data, err := os.ReadFile(deploymentPath)
		Expect(err).NotTo(HaveOccurred())
		content := string(data)

		// Verify JWT settings are rendered
		Expect(content).To(ContainSubstring(`value: "1h"`),
			"JWT_EXPIRATION should be 1h")
		Expect(content).To(ContainSubstring(`value: "15m"`),
			"JWT_REFRESH_WINDOW should be 15m")
		Expect(content).To(ContainSubstring(`value: "12h"`),
			"JWT_REFRESH_HORIZON should be 12h")
		Expect(content).To(ContainSubstring(`value: "true"`),
			"JWT_REFRESH_ENABLE should be true")
		Expect(content).To(ContainSubstring(`value: "5s"`),
			"NEW_KEY_USE_DELAY should be 5s")
	})
})
