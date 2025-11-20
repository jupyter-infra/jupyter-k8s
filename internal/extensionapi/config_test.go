/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package extensionapi

import (
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
			customOrigin := "https://example.com"
			config := NewConfig(WithAllowedOrigin(customOrigin))

			Expect(config.AllowedOrigin).To(Equal(customOrigin))
		})

		It("Should allow to override ClusterId", func() {
			customClusterId := "test-cluster-123"
			config := NewConfig(WithClusterId(customClusterId))

			Expect(config.ClusterId).To(Equal(customClusterId))
		})
	})
})
