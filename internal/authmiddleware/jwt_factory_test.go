/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("NewJWTHandler", func() {
	var (
		logger = zap.New(zap.UseDevMode(true))
		cfg    *Config
	)

	BeforeEach(func() {
		// Default config for standard signing
		cfg = &Config{
			JWTSigningType:    JWTSigningTypeStandard,
			JwtSecretName:     "test-jwt-secret",
			JWTIssuer:         "test-issuer",
			JWTAudience:       "test-audience",
			JWTExpiration:     time.Hour,
			JwtNewKeyUseDelay: 5 * time.Minute,
			JWTRefreshEnable:  true,
			JWTRefreshWindow:  10 * time.Minute,
			JWTRefreshHorizon: 5 * time.Minute,
		}
	})

	Context("Standard Signing Type", func() {
		It("Should create JWT handler and StandardSigner", func() {
			handler, standardSigner, err := NewJWTHandler(cfg, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(handler).NotTo(BeNil())
			Expect(standardSigner).NotTo(BeNil())
		})

	})

	Context("Invalid Configuration", func() {
		It("Should return error if config is nil", func() {
			handler, standardSigner, err := NewJWTHandler(nil, logger)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("config cannot be nil"))
			Expect(handler).To(BeNil())
			Expect(standardSigner).To(BeNil())
		})

		It("Should return error for unknown JWT signing type", func() {
			cfg.JWTSigningType = "unknown-type"

			handler, standardSigner, err := NewJWTHandler(cfg, logger)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported JWT signing type"))
			Expect(handler).To(BeNil())
			Expect(standardSigner).To(BeNil())
		})

		It("Should return error for kms JWT signing type with migration guidance", func() {
			cfg.JWTSigningType = "kms"

			handler, standardSigner, err := NewJWTHandler(cfg, logger)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported JWT signing type"))
			Expect(handler).To(BeNil())
			Expect(standardSigner).To(BeNil())
		})
	})
})
