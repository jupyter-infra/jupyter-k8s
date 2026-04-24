/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("AccessStartupProber", func() {
	var (
		prober         *AccessStartupProber
		workspace      *workspacev1alpha1.Workspace
		accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
	)

	BeforeEach(func() {
		prober = NewAccessStartupProber(NewAccessResourcesBuilder())

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "test-namespace",
			},
		}

		accessStrategy = &workspacev1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-strategy",
				Namespace: "test-namespace",
			},
			Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
				DisplayName:             "Test Strategy",
				AccessResourceTemplates: []workspacev1alpha1.AccessResourceTemplate{},
			},
		}
	})

	Context("isProbeStatusSuccess", func() {
		It("should treat 200-399 as success", func() {
			Expect(isProbeStatusSuccess(200, nil)).To(BeTrue())
			Expect(isProbeStatusSuccess(301, nil)).To(BeTrue())
			Expect(isProbeStatusSuccess(302, nil)).To(BeTrue())
			Expect(isProbeStatusSuccess(399, nil)).To(BeTrue())
		})

		It("should treat 400+ as failure by default", func() {
			Expect(isProbeStatusSuccess(400, nil)).To(BeFalse())
			Expect(isProbeStatusSuccess(401, nil)).To(BeFalse())
			Expect(isProbeStatusSuccess(404, nil)).To(BeFalse())
			Expect(isProbeStatusSuccess(500, nil)).To(BeFalse())
			Expect(isProbeStatusSuccess(502, nil)).To(BeFalse())
			Expect(isProbeStatusSuccess(503, nil)).To(BeFalse())
		})

		It("should treat additionalSuccessStatusCodes as success", func() {
			additional := []int{401, 403}
			Expect(isProbeStatusSuccess(401, additional)).To(BeTrue())
			Expect(isProbeStatusSuccess(403, additional)).To(BeTrue())
			Expect(isProbeStatusSuccess(502, additional)).To(BeFalse())
		})

		It("should handle sub-200 codes as failure", func() {
			Expect(isProbeStatusSuccess(100, nil)).To(BeFalse())
			Expect(isProbeStatusSuccess(199, nil)).To(BeFalse())
		})
	})

	Context("Probe", func() {
		It("should return true for 200 OK", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: server.URL,
				},
				TimeoutSeconds: 5,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())
		})

		It("should return true for 302 redirect (oauth flow)", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Redirect(w, &http.Request{}, "https://dex.example.com/auth", http.StatusFound)
			}))
			defer server.Close()

			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: server.URL,
				},
				TimeoutSeconds: 5,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())
		})

		It("should return false for 502 Bad Gateway", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			}))
			defer server.Close()

			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: server.URL,
				},
				TimeoutSeconds: 5,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		It("should return false for 503 Service Unavailable", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()

			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: server.URL,
				},
				TimeoutSeconds: 5,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		It("should return true for 401 when in additionalSuccessStatusCodes", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer server.Close()

			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate:                  server.URL,
					AdditionalSuccessStatusCodes: []int{401},
				},
				TimeoutSeconds: 5,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())
		})

		It("should return false for 401 without additionalSuccessStatusCodes", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer server.Close()

			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: server.URL,
				},
				TimeoutSeconds: 5,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		It("should return false on connection refused", func() {
			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: "http://localhost:1", // unlikely to be listening
				},
				TimeoutSeconds: 1,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		It("should not follow redirects", func() {
			redirectCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirectCount++
				http.Redirect(w, r, "/next", http.StatusFound)
			}))
			defer server.Close()

			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: server.URL,
				},
				TimeoutSeconds: 5,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())
			Expect(redirectCount).To(Equal(1))
		})

		It("should resolve URL templates", func() {
			var receivedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: server.URL + "/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/",
				},
				TimeoutSeconds: 5,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())
			Expect(receivedPath).To(Equal("/workspaces/test-namespace/test-workspace/"))
		})

		It("should return error for nil httpGet", func() {
			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				TimeoutSeconds: 5,
			}

			_, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("httpGet is required"))
		})

		It("should return error for invalid URL template", func() {
			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: "{{ .InvalidSyntax }",
				},
				TimeoutSeconds: 5,
			}

			_, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to resolve probe URL"))
		})

		It("should return false on timeout", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(3 * time.Second)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			accessStrategy.Spec.AccessStartupProbe = &workspacev1alpha1.AccessStartupProbe{
				HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
					URLTemplate: server.URL,
				},
				TimeoutSeconds: 1,
			}

			ready, err := prober.Probe(context.Background(), workspace, accessStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})
	})

	Context("resolve helpers", func() {
		It("should use defaults when values are zero", func() {
			probe := &workspacev1alpha1.AccessStartupProbe{}
			Expect(resolveTimeoutSeconds(probe)).To(Equal(int32(DefaultAccessStartupProbeTimeoutSeconds)))
			Expect(resolvePeriodSeconds(probe)).To(Equal(int32(DefaultAccessStartupProbePeriodSeconds)))
			Expect(resolveFailureThreshold(probe)).To(Equal(int32(DefaultAccessStartupProbeFailureThreshold)))
		})

		It("should use configured values when set", func() {
			probe := &workspacev1alpha1.AccessStartupProbe{
				TimeoutSeconds:   10,
				PeriodSeconds:    3,
				FailureThreshold: 5,
			}
			Expect(resolveTimeoutSeconds(probe)).To(Equal(int32(10)))
			Expect(resolvePeriodSeconds(probe)).To(Equal(int32(3)))
			Expect(resolveFailureThreshold(probe)).To(Equal(int32(5)))
		})
	})
})
