package aws_traefik_dex_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OIDC Configuration Consistency", func() {
	var (
		rootDir            string
		testOutputDir      string
		dexConfigmapData   []byte
		authMiddlewareData []byte
		oauth2ProxyData    []byte
		secretsData        []byte

		// Dex configmap values
		dexIssuerURL      string
		dexRedirectURI    string
		dexOAuth2ClientID string
		dexOAuth2Secret   string
		dexAuthMwClientID string
		dexAuthMwSecret   string

		// Authmiddleware values
		authMwIssuerURL    string
		authMwClientID     string
		authMwClientSecret string

		// OAuth2-proxy values
		oauth2IssuerURL    string
		oauth2RedirectURL  string
		oauth2ClientID     string
		oauth2ClientSecret string
	)

	BeforeEach(func() {
		var err error
		rootDir, err = filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		// Path to rendered test output
		testOutputDir = filepath.Join(rootDir, "dist/test-output-guided/jupyter-k8s-aws-traefik-dex/templates")

		// Read the necessary files
		dexConfigmapPath := filepath.Join(testOutputDir, "dex/configmap.yaml")
		dexConfigmapData, err = os.ReadFile(dexConfigmapPath)
		Expect(err).NotTo(HaveOccurred(), "Failed to read dex configmap file")

		authMiddlewarePath := filepath.Join(testOutputDir, "authmiddleware/deployment.yaml")
		authMiddlewareData, err = os.ReadFile(authMiddlewarePath)
		Expect(err).NotTo(HaveOccurred(), "Failed to read authmiddleware deployment file")

		authMwSecretsPath := filepath.Join(testOutputDir, "authmiddleware/secrets.yaml")
		secretsData, err = os.ReadFile(authMwSecretsPath)
		Expect(err).NotTo(HaveOccurred(), "Failed to read authmiddleware secrets file")

		oauth2ProxyPath := filepath.Join(testOutputDir, "oauth2-proxy/deployment.yaml")
		oauth2ProxyData, err = os.ReadFile(oauth2ProxyPath)
		Expect(err).NotTo(HaveOccurred(), "Failed to read oauth2-proxy deployment file")

		// Extract values from dex configmap
		dexConfigmapContent := string(dexConfigmapData)

		// Extract issuer URL
		matches := regexp.MustCompile(`(?m)^\s*issuer:\s*(.+)$`).FindStringSubmatch(dexConfigmapContent)
		Expect(matches).To(HaveLen(2), "Could not find issuer URL in dex configmap")
		dexIssuerURL = matches[1]
		Expect(dexIssuerURL).NotTo(BeEmpty(), "Issuer URL in dex config is empty")
		By(fmt.Sprintf("Extracted dex issuer URL: %s", dexIssuerURL))

		// Extract redirectURI
		matches = regexp.MustCompile(`(?m)redirectURIs:\s*\n\s*-\s*(.+)`).FindStringSubmatch(dexConfigmapContent)
		Expect(matches).To(HaveLen(2), "Could not find redirectURI in dex configmap")
		dexRedirectURI = matches[1]
		Expect(dexRedirectURI).NotTo(BeEmpty(), "RedirectURI in dex configmap is empty")
		By(fmt.Sprintf("Extracted dex redirectURI: %s", dexRedirectURI))

		// Extract oauth2-proxy client ID
		matches = regexp.MustCompile(`(?m)^\s*-\s*id:\s*(\S+)\s*\n\s*redirectURIs:`).FindStringSubmatch(dexConfigmapContent)
		Expect(matches).To(HaveLen(2), "Could not find oauth2-proxy client ID in dex configmap")
		dexOAuth2ClientID = matches[1]
		Expect(dexOAuth2ClientID).NotTo(BeEmpty(), "oauth2-proxy client ID in dex configmap is empty")
		By(fmt.Sprintf("Extracted oauth2-proxy client ID from dex: %s", dexOAuth2ClientID))

		// Extract oauth2-proxy client secret
		oauth2ProxySecretRegex := `(?m)^\s*name:\s*'OAuth2 Proxy'\s*\n\s*secret:\s*(\S+)`
		matches = regexp.MustCompile(oauth2ProxySecretRegex).FindStringSubmatch(dexConfigmapContent)
		Expect(matches).To(HaveLen(2), "Could not find oauth2-proxy secret in dex configmap")
		dexOAuth2Secret = matches[1]
		Expect(dexOAuth2Secret).NotTo(BeEmpty(), "oauth2-proxy secret in dex configmap is empty")
		By(fmt.Sprintf("Extracted oauth2-proxy secret from dex: %s", dexOAuth2Secret))

		// Extract authmiddleware client ID
		authMwClientIDRegex := `(?m)^\s*-\s*id:\s*(\S+)\s*\n\s*name:\s*'Auth Middleware'`
		matches = regexp.MustCompile(authMwClientIDRegex).FindStringSubmatch(dexConfigmapContent)
		Expect(matches).To(HaveLen(2), "Could not find authmiddleware client ID in dex configmap")
		dexAuthMwClientID = matches[1]
		Expect(dexAuthMwClientID).NotTo(BeEmpty(), "authmiddleware client ID in dex configmap is empty")
		By(fmt.Sprintf("Extracted authmiddleware client ID from dex: %s", dexAuthMwClientID))

		// Extract authmiddleware client secret
		authMwSecretRegex := `(?m)^\s*name:\s*'Auth Middleware'\s*\n\s*secret:\s*(\S+)`
		matches = regexp.MustCompile(authMwSecretRegex).FindStringSubmatch(dexConfigmapContent)
		Expect(matches).To(HaveLen(2), "Could not find authmiddleware secret in dex configmap")
		dexAuthMwSecret = matches[1]
		Expect(dexAuthMwSecret).NotTo(BeEmpty(), "authmiddleware secret in dex configmap is empty")
		By(fmt.Sprintf("Extracted authmiddleware secret from dex: %s", dexAuthMwSecret))

		// Extract values from oauth2-proxy deployment
		oauth2ProxyContent := string(oauth2ProxyData)

		// Extract oidc-issuer-url
		matches = regexp.MustCompile(`(?m)^\s*-\s*--oidc-issuer-url=(.+)$`).FindStringSubmatch(oauth2ProxyContent)
		Expect(matches).To(HaveLen(2), "Could not find --oidc-issuer-url in oauth2-proxy deployment")
		oauth2IssuerURL = matches[1]
		Expect(oauth2IssuerURL).NotTo(BeEmpty(), "--oidc-issuer-url in oauth2-proxy deployment is empty")
		By(fmt.Sprintf("Extracted oauth2-proxy issuer URL: %s", oauth2IssuerURL))

		// Extract redirect-url
		matches = regexp.MustCompile(`(?m)^\s*-\s*--redirect-url=(.+)$`).FindStringSubmatch(oauth2ProxyContent)
		Expect(matches).To(HaveLen(2), "Could not find --redirect-url in oauth2-proxy deployment")
		oauth2RedirectURL = matches[1]
		Expect(oauth2RedirectURL).NotTo(BeEmpty(), "--redirect-url in oauth2-proxy deployment is empty")
		By(fmt.Sprintf("Extracted oauth2-proxy redirect URL: %s", oauth2RedirectURL))

		// Extract oauth2-proxy client ID
		matches = regexp.MustCompile(`(?m)^\s*-\s*--client-id=(.+)$`).FindStringSubmatch(oauth2ProxyContent)
		Expect(matches).To(HaveLen(2), "Could not find --client-id in oauth2-proxy deployment")
		oauth2ClientID = matches[1]
		Expect(oauth2ClientID).NotTo(BeEmpty(), "--client-id in oauth2-proxy deployment is empty")
		By(fmt.Sprintf("Extracted oauth2-proxy client ID: %s", oauth2ClientID))

		// Extract oauth2-proxy client secret
		matches = regexp.MustCompile(`(?m)^\s*-\s*--client-secret=(.+)$`).FindStringSubmatch(oauth2ProxyContent)
		Expect(matches).To(HaveLen(2), "Could not find --client-secret in oauth2-proxy deployment")
		oauth2ClientSecret = matches[1]
		Expect(oauth2ClientSecret).NotTo(BeEmpty(), "--client-secret in oauth2-proxy deployment is empty")
		By(fmt.Sprintf("Extracted oauth2-proxy client secret: %s", oauth2ClientSecret))

		// Extract values from authmiddleware deployment
		authMiddlewareContent := string(authMiddlewareData)

		// Extract OIDC_ISSUER_URL
		oidcIssuerURLRegex := `(?m)^\s*-\s*name:\s*OIDC_ISSUER_URL\s*\n\s*value:\s*"(.+?)"`
		matches = regexp.MustCompile(oidcIssuerURLRegex).FindStringSubmatch(authMiddlewareContent)
		Expect(matches).To(HaveLen(2), "Could not find OIDC_ISSUER_URL in authmiddleware deployment")
		authMwIssuerURL = matches[1]
		Expect(authMwIssuerURL).NotTo(BeEmpty(), "OIDC_ISSUER_URL in authmiddleware deployment is empty")
		By(fmt.Sprintf("Extracted authmiddleware OIDC_ISSUER_URL: %s", authMwIssuerURL))

		// Extract OIDC_CLIENT_ID
		oidcClientIDRegex := `(?m)^\s*-\s*name:\s*OIDC_CLIENT_ID\s*\n\s*value:\s*"(.+?)"`
		matches = regexp.MustCompile(oidcClientIDRegex).FindStringSubmatch(authMiddlewareContent)
		Expect(matches).To(HaveLen(2), "Could not find OIDC_CLIENT_ID in authmiddleware deployment")
		authMwClientID = matches[1]
		Expect(authMwClientID).NotTo(BeEmpty(), "OIDC_CLIENT_ID in authmiddleware deployment is empty")
		By(fmt.Sprintf("Extracted authmiddleware client ID: %s", authMwClientID))

		// Extract client secret from authmiddleware secrets
		authMwSecretsContent := string(secretsData)
		matches = regexp.MustCompile(`(?m)^\s*oidc-client-secret:\s*(.+)$`).FindStringSubmatch(authMwSecretsContent)
		Expect(matches).To(HaveLen(2), "Could not find oidc-client-secret in authmiddleware secrets")
		authMwClientSecret = matches[1]

		// Remove quotes if present
		authMwClientSecret = regexp.MustCompile(`^"(.*)"$`).ReplaceAllString(authMwClientSecret, "$1")
		Expect(authMwClientSecret).NotTo(BeEmpty(), "oidc-client-secret in authmiddleware secrets is empty")
		By(fmt.Sprintf("Extracted authmiddleware client secret: %s", authMwClientSecret))
	})

	It("should have consistent OIDC issuer URL between dex configmap and authmiddleware deployment", func() {
		By(fmt.Sprintf("Comparing dex issuer URL '%s' with authmiddleware URL '%s'",
			dexIssuerURL, authMwIssuerURL))

		const errMsg = "OIDC_ISSUER_URL in authmiddleware deployment does not match issuer URL in dex configmap"
		Expect(authMwIssuerURL).To(Equal(dexIssuerURL), errMsg)
	})

	It("should have consistent OIDC issuer URL between dex configmap and oauth2-proxy deployment", func() {
		By(fmt.Sprintf("Comparing dex issuer URL '%s' with oauth2-proxy URL '%s'",
			dexIssuerURL, oauth2IssuerURL))

		const errMsg = "--oidc-issuer-url in oauth2-proxy deployment does not match issuer URL in dex configmap"
		Expect(oauth2IssuerURL).To(Equal(dexIssuerURL), errMsg)
	})

	It("should have consistent redirect URL between dex configmap and oauth2-proxy deployment", func() {
		By(fmt.Sprintf("Comparing dex redirectURI '%s' with oauth2-proxy URL '%s'",
			dexRedirectURI, oauth2RedirectURL))

		const errMsg = "--redirect-url in oauth2-proxy deployment does not match redirectURI in dex configmap"
		Expect(oauth2RedirectURL).To(Equal(dexRedirectURI), errMsg)
	})

	It("should have consistent client ID between dex configmap and authmiddleware deployment", func() {
		By(fmt.Sprintf("Comparing authmiddleware client ID in dex configmap '%s' with "+
			"OIDC_CLIENT_ID in authmiddleware deployment '%s'", dexAuthMwClientID, authMwClientID))

		const errMsg = "OIDC_CLIENT_ID in authmiddleware deployment does not match client ID in dex configmap"
		Expect(authMwClientID).To(Equal(dexAuthMwClientID), errMsg)
	})

	It("should have consistent client ID between dex configmap and oauth2-proxy deployment", func() {
		By(fmt.Sprintf("Comparing oauth2-proxy client ID in dex configmap '%s' with "+
			"--client-id in oauth2-proxy deployment '%s'", dexOAuth2ClientID, oauth2ClientID))

		const errMsg = "--client-id in oauth2-proxy deployment does not match client ID in dex configmap"
		Expect(oauth2ClientID).To(Equal(dexOAuth2ClientID), errMsg)
	})

	It("should have consistent client secret between dex configmap and oauth2-proxy deployment", func() {
		By(fmt.Sprintf("Comparing oauth2-proxy client secret in dex configmap '%s' with "+
			"--client-secret in oauth2-proxy deployment '%s'", dexOAuth2Secret, oauth2ClientSecret))

		const errMsg = "--client-secret in oauth2-proxy deployment does not match client secret in dex configmap"
		Expect(oauth2ClientSecret).To(Equal(dexOAuth2Secret), errMsg)
	})

	It("should have consistent client secret between dex configmap and authmiddleware secrets", func() {
		By(fmt.Sprintf("Comparing authmiddleware client secret in dex configmap '%s' with "+
			"oidc-client-secret in authmiddleware secrets '%s'", dexAuthMwSecret, authMwClientSecret))

		const errMsg = "oidc-client-secret in authmiddleware secrets does not match client secret in dex configmap"
		Expect(authMwClientSecret).To(Equal(dexAuthMwSecret), errMsg)
	})
})
