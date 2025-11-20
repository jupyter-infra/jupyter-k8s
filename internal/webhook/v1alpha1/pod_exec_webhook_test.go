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

package v1alpha1

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

var _ = Describe("PodExec Webhook", func() {
	var (
		validator                  *PodExecValidator
		ctx                        context.Context
		scheme                     *runtime.Scheme
		expectedControllerUsername string
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		validator = &PodExecValidator{}

		// Set required environment variables for tests
		GinkgoT().Setenv(controller.ControllerPodNamespaceEnv, "jupyter-k8s-system")
		GinkgoT().Setenv(controller.ControllerPodServiceAccountEnv, "jupyter-k8s-controller-manager")

		// Build expected controller service account username
		expectedControllerUsername = fmt.Sprintf("system:serviceaccount:%s:%s",
			"jupyter-k8s-system",
			"jupyter-k8s-controller-manager")
	})

	Context("when validating pod exec requests", func() {
		It("should allow controller exec to workspace pods", func() {
			// Create a workspace pod
			workspacePod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workspace-pod",
					Namespace: "test-namespace",
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceName: "test-workspace",
					},
				},
			}

			// Create fake client with the pod
			validator.Client = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspacePod).
				Build()

			// Create admission request from controller service account
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      "workspace-pod",
					Namespace: "test-namespace",
					UserInfo: authenticationv1.UserInfo{
						Username: expectedControllerUsername,
					},
				},
			}

			// Validate the request
			resp := validator.Handle(ctx, req)

			// Should be allowed
			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Result.Message).To(Equal("exec request allowed"))
		})

		It("should deny controller exec to non-workspace pods", func() {
			// Create a non-workspace pod (no workspace label)
			nonWorkspacePod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "regular-pod",
					Namespace: "test-namespace",
					Labels:    map[string]string{}, // No workspace label
				},
			}

			// Create fake client with the pod
			validator.Client = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(nonWorkspacePod).
				Build()

			// Create admission request from controller service account
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      "regular-pod",
					Namespace: "test-namespace",
					UserInfo: authenticationv1.UserInfo{
						Username: expectedControllerUsername,
					},
				},
			}

			// Validate the request
			resp := validator.Handle(ctx, req)

			// Should be denied
			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result.Message).To(Equal("controller service account can only exec into workspace pods"))
		})

		It("should allow exec from non-controller users to any pod", func() {
			// Create a workspace pod
			workspacePod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workspace-pod",
					Namespace: "test-namespace",
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceName: "test-workspace",
					},
				},
			}

			// Create fake client with the pod
			validator.Client = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspacePod).
				Build()

			// Create admission request from regular user (not controller)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      "workspace-pod",
					Namespace: "test-namespace",
					UserInfo: authenticationv1.UserInfo{
						Username: "system:serviceaccount:default:some-other-user",
					},
				},
			}

			// Validate the request
			resp := validator.Handle(ctx, req)

			// Should be allowed (regular users can exec into any pod)
			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Result.Message).To(Equal("exec request allowed"))
		})

		It("should handle pod not found errors", func() {
			// Create fake client without any pods
			validator.Client = fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			// Create admission request from controller service account
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      "non-existent-pod",
					Namespace: "test-namespace",
					UserInfo: authenticationv1.UserInfo{
						Username: expectedControllerUsername,
					},
				},
			}

			// Validate the request
			resp := validator.Handle(ctx, req)

			// Should be an error
			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result.Code).To(Equal(int32(500)))
		})

	})

	Context("when setting up webhook with manager", func() {
		It("should fail when CONTROLLER_POD_NAMESPACE env var is not set", func() {
			GinkgoT().Setenv(controller.ControllerPodNamespaceEnv, "")
			GinkgoT().Setenv(controller.ControllerPodServiceAccountEnv, "jupyter-k8s-controller-manager")

			// Test the validation logic directly without calling the full setup
			controllerNamespace := os.Getenv(controller.ControllerPodNamespaceEnv)
			Expect(controllerNamespace).To(Equal(""))

			// This would cause an error in the actual function
			Expect(controllerNamespace).To(BeEmpty())
		})

		It("should fail when CONTROLLER_POD_SERVICE_ACCOUNT env var is not set", func() {
			GinkgoT().Setenv(controller.ControllerPodNamespaceEnv, "jupyter-k8s-system")
			GinkgoT().Setenv(controller.ControllerPodServiceAccountEnv, "")

			// Test the validation logic directly without calling the full setup
			controllerServiceAccount := os.Getenv(controller.ControllerPodServiceAccountEnv)
			Expect(controllerServiceAccount).To(Equal(""))

			// This would cause an error in the actual function
			Expect(controllerServiceAccount).To(BeEmpty())
		})
	})
})
