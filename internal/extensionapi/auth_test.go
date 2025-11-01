package extensionapi

import (
	"crypto/x509"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AuthConfig", func() {
	var authConfig *AuthConfig

	BeforeEach(func() {
		authConfig = &AuthConfig{
			ClientCA:     &x509.Certificate{},
			AllowedNames: []string{"front-proxy-client"},
		}
	})

	Describe("InitializeAuthenticator", func() {
		It("should initialize authenticator successfully", func() {
			err := authConfig.InitializeAuthenticator()
			Expect(err).ToNot(HaveOccurred())
			Expect(authConfig.authenticator).ToNot(BeNil())
		})

		It("should return error for nil AuthConfig", func() {
			var nilConfig *AuthConfig
			err := nilConfig.InitializeAuthenticator()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AuthConfig is nil"))
		})
	})

	Describe("AuthenticateRequest", func() {
		BeforeEach(func() {
			err := authConfig.InitializeAuthenticator()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should authenticate request with valid headers", func() {
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Remote-User", "test-user")
			req.Header.Set("X-Remote-Group", "system:authenticated")

			userInfo, err := authConfig.AuthenticateRequest(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(userInfo).ToNot(BeNil())
			Expect(userInfo.Username).To(Equal("test-user"))
			Expect(userInfo.Groups).To(ContainElement("system:authenticated"))
		})

		It("should fail with missing username header", func() {
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Remote-Group", "system:authenticated")

			userInfo, err := authConfig.AuthenticateRequest(req)
			Expect(err).To(HaveOccurred())
			Expect(userInfo).To(BeNil())
		})

		It("should return error for nil request", func() {
			userInfo, err := authConfig.AuthenticateRequest(nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("request is nil"))
			Expect(userInfo).To(BeNil())
		})

		It("should return error for uninitialized authenticator", func() {
			authConfig.authenticator = nil
			req, _ := http.NewRequest("GET", "/test", nil)

			userInfo, err := authConfig.AuthenticateRequest(req)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("authenticator not initialized"))
			Expect(userInfo).To(BeNil())
		})
	})

	Describe("IsAllowedClientName", func() {
		It("should return true for allowed name", func() {
			result := authConfig.IsAllowedClientName("front-proxy-client")
			Expect(result).To(BeTrue())
		})

		It("should return false for disallowed name", func() {
			result := authConfig.IsAllowedClientName("malicious-client")
			Expect(result).To(BeFalse())
		})

		It("should return false for empty name", func() {
			result := authConfig.IsAllowedClientName("")
			Expect(result).To(BeFalse())
		})

		It("should return false for nil AuthConfig", func() {
			var nilConfig *AuthConfig
			result := nilConfig.IsAllowedClientName("front-proxy-client")
			Expect(result).To(BeFalse())
		})
	})
})
