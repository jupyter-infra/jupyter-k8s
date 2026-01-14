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

	Context("KMS Signing Type", func() {
		BeforeEach(func() {
			cfg.JWTSigningType = JWTSigningTypeKMS
			cfg.KMSKeyId = "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012"
		})

		It("Should return error if KMS key ID is missing", func() {
			cfg.KMSKeyId = ""

			handler, standardSigner, err := NewJWTHandler(cfg, logger)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("KMS_KEY_ID required when JWT_SIGNING_TYPE is kms"))
			Expect(handler).To(BeNil())
			Expect(standardSigner).To(BeNil())
		})

		It("Should return nil StandardSigner for KMS signing (no secret watching needed)", func() {
			// Note: This test will fail to create KMS client in test environment
			// We're just verifying the logic flow
			_, standardSigner, err := NewJWTHandler(cfg, logger)

			// We expect an error creating KMS client in test environment
			// but we can verify the standardSigner would be nil if it succeeded
			if err == nil {
				Expect(standardSigner).To(BeNil(), "KMS signing should not create a StandardSigner")
			} else {
				// Expected in test environment without AWS credentials
				Expect(err.Error()).To(ContainSubstring("failed to create KMS client"))
			}
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
			Expect(err.Error()).To(Equal("unknown JWT signing type: unknown-type"))
			Expect(handler).To(BeNil())
			Expect(standardSigner).To(BeNil())
		})
	})
})
