/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExtensionConfig", func() {

	Context("NewConfig", func() {
		It("Should generate a config with defaults when no overrides are passed to the builder", func() {
			config := NewConfig()

			Expect(config.ApiPath).To(Equal(DefaultApiPath))
			Expect(config.ServerPort).To(Equal(DefaultServerPort))
			Expect(config.CertPath).To(Equal(DefaultCertPath))
			Expect(config.KeyPath).To(Equal(DefaultKeyPath))
			Expect(config.LogLevel).To(Equal(DefaultLogLevel))
			Expect(config.DisableTLS).To(BeFalse())
			Expect(config.ReadTimeoutSeconds).To(Equal(DefaultReadTimeoutSeconds))
			Expect(config.WriteTimeoutSeconds).To(Equal(DefaultWriteTimeoutSeconds))
			Expect(config.AllowedOrigin).To(Equal(DefaultAllowedOrigin))
		})

		It("Should chain overrides", func() {
			config := NewConfig(
				WithDefaultApiPath("/custom/api"),
				WithServerPort(8080),
				WithLogLevel("debug"),
			)

			Expect(config.ApiPath).To(Equal("/custom/api"))
			Expect(config.ServerPort).To(Equal(8080))
			Expect(config.LogLevel).To(Equal("debug"))

			// Other fields should maintain defaults
			Expect(config.CertPath).To(Equal(DefaultCertPath))
			Expect(config.KeyPath).To(Equal(DefaultKeyPath))
			Expect(config.DisableTLS).To(BeFalse())
			Expect(config.ReadTimeoutSeconds).To(Equal(DefaultReadTimeoutSeconds))
			Expect(config.WriteTimeoutSeconds).To(Equal(DefaultWriteTimeoutSeconds))
			Expect(config.AllowedOrigin).To(Equal(DefaultAllowedOrigin))
		})

		It("Should allow to override ApiPath", func() {
			customApiPath := "/custom/api/v2"
			config := NewConfig(WithDefaultApiPath(customApiPath))

			Expect(config.ApiPath).To(Equal(customApiPath))
		})

		It("Should allow to override DefaultServerPort", func() {
			customPort := 9000
			config := NewConfig(WithServerPort(customPort))

			Expect(config.ServerPort).To(Equal(customPort))
		})

		It("Should allow to override DefaultCertPath", func() {
			customPath := "/custom/cert/path.crt"
			config := NewConfig(WithCertPath(customPath))

			Expect(config.CertPath).To(Equal(customPath))
		})

		It("Should allow to override DefaultKeyPath", func() {
			customPath := "/custom/key/path.key"
			config := NewConfig(WithKeyPath(customPath))

			Expect(config.KeyPath).To(Equal(customPath))
		})

		It("Should allow to override DefaultLogLevel", func() {
			customLevel := "debug"
			config := NewConfig(WithLogLevel(customLevel))

			Expect(config.LogLevel).To(Equal(customLevel))
		})

		It("Should allow to override DefaultDisableTLS", func() {
			config := NewConfig(WithDisableTLS(true))

			Expect(config.DisableTLS).To(BeTrue())
		})

		It("Should allow to override DefaultReadTimeoutSeconds", func() {
			customTimeout := 60
			config := NewConfig(WithReadTimeoutSeconds(customTimeout))

			Expect(config.ReadTimeoutSeconds).To(Equal(customTimeout))
		})

		It("Should allow to override DefaultWriteTimeoutSeconds", func() {
			customTimeout := 240
			config := NewConfig(WithWriteTimeoutSeconds(customTimeout))

			Expect(config.WriteTimeoutSeconds).To(Equal(customTimeout))
		})

		It("Should allow to override DefaultAllowedOrigin", func() {
			customOrigin := exampleURL
			config := NewConfig(WithAllowedOrigin(customOrigin))

			Expect(config.AllowedOrigin).To(Equal(customOrigin))
		})

		It("Should allow to override PluginEndpoints", func() {
			endpoints := map[string]string{"aws": "http://localhost:8080"}
			config := NewConfig(WithPluginEndpoints(endpoints))

			Expect(config.PluginEndpoints).To(Equal(endpoints))
		})

		It("Should allow to override ControllerNamespace", func() {
			config := NewConfig(WithControllerNamespace("jupyter-system"))

			Expect(config.ControllerNamespace).To(Equal("jupyter-system"))
		})

		It("Should allow to override JwtIssuer", func() {
			config := NewConfig(WithJwtIssuer("custom-issuer"))

			Expect(config.JwtIssuer).To(Equal("custom-issuer"))
		})

		It("Should allow to override JwtAudience", func() {
			config := NewConfig(WithJwtAudience("custom-audience"))

			Expect(config.JwtAudience).To(Equal("custom-audience"))
		})

		It("Should allow to override JwtTTL", func() {
			config := NewConfig(WithJwtTTL(10 * time.Minute))

			Expect(config.JwtTTL).To(Equal(10 * time.Minute))
		})

		It("Should allow to override NewKeyUseDelay", func() {
			config := NewConfig(WithNewKeyUseDelay(15 * time.Second))

			Expect(config.NewKeyUseDelay).To(Equal(15 * time.Second))
		})

	})
})
