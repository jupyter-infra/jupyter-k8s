package extensionapi

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("ConfigMap Reader", func() {
	var (
		k8sClient client.Client
		ctx       context.Context
		dummyCert string
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Simple dummy certificate for testing
		dummyCert = `-----BEGIN CERTIFICATE-----
DUMMY-CERT-DATA-FOR-TESTING
-----END CERTIFICATE-----`
	})

	Describe("loadAuthConfigFromConfigMap", func() {
		It("should return error for invalid certificate", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      AuthConfigMapName,
					Namespace: AuthConfigMapNamespace,
				},
				Data: map[string]string{
					RequestHeaderClientCAFileKey: dummyCert,
					RequestHeaderAllowedNamesKey: `["front-proxy-client"]`,
				},
			}

			k8sClient = fake.NewClientBuilder().WithObjects(configMap).Build()

			authConfig, err := loadAuthConfigFromConfigMap(ctx, k8sClient)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse client CA certificate"))
			Expect(authConfig).To(BeNil())
		})

		It("should return error when ConfigMap not found", func() {
			k8sClient = fake.NewClientBuilder().Build()

			authConfig, err := loadAuthConfigFromConfigMap(ctx, k8sClient)
			Expect(err).To(HaveOccurred())
			Expect(authConfig).To(BeNil())
		})

		It("should return error when CA file missing", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      AuthConfigMapName,
					Namespace: AuthConfigMapNamespace,
				},
				Data: map[string]string{
					RequestHeaderAllowedNamesKey: `["front-proxy-client"]`,
				},
			}

			k8sClient = fake.NewClientBuilder().WithObjects(configMap).Build()

			authConfig, err := loadAuthConfigFromConfigMap(ctx, k8sClient)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing requestheader-client-ca-file"))
			Expect(authConfig).To(BeNil())
		})

		It("should return error when allowed names missing", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      AuthConfigMapName,
					Namespace: AuthConfigMapNamespace,
				},
				Data: map[string]string{
					RequestHeaderClientCAFileKey: dummyCert,
				},
			}

			k8sClient = fake.NewClientBuilder().WithObjects(configMap).Build()

			authConfig, err := loadAuthConfigFromConfigMap(ctx, k8sClient)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse client CA certificate"))
			Expect(authConfig).To(BeNil())
		})
	})

	Describe("parseCertificate", func() {
		It("should return error for invalid PEM", func() {
			invalidCert := "not-a-certificate"

			cert, err := parseCertificate(invalidCert)
			Expect(err).To(HaveOccurred())
			Expect(cert).To(BeNil())
		})

		It("should return error for empty certificate", func() {
			cert, err := parseCertificate("")
			Expect(err).To(HaveOccurred())
			Expect(cert).To(BeNil())
		})

		It("should return error for dummy certificate", func() {
			cert, err := parseCertificate(dummyCert)
			Expect(err).To(HaveOccurred())
			Expect(cert).To(BeNil())
		})
	})
})
