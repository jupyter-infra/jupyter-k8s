package extensionapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

var _ = Describe("Utils", func() {
	Context("WriteError", func() {
		var (
			recorder *httptest.ResponseRecorder
		)

		BeforeEach(func() {
			recorder = httptest.NewRecorder()
		})

		It("Should set the content type header", func() {
			// Call the function
			WriteError(recorder, http.StatusBadRequest, "Test error")

			// Verify content type header is set correctly
			contentType := recorder.Header().Get("Content-Type")
			Expect(contentType).To(Equal("application/json"))
		})

		It("Should set the correct status", func() {
			// Call the function with different status codes
			testCases := []int{
				http.StatusBadRequest,
				http.StatusNotFound,
				http.StatusForbidden,
				http.StatusInternalServerError,
			}

			for _, statusCode := range testCases {
				recorder = httptest.NewRecorder()
				WriteError(recorder, statusCode, "Test error")

				// Verify status code is set correctly
				Expect(recorder.Code).To(Equal(statusCode))
			}
		})

		It("Should write the correct error message", func() {
			// Call the function with a test error message
			testMessage := "This is a test error message"
			WriteError(recorder, http.StatusBadRequest, testMessage)

			// Read and parse the response body
			var response map[string]string
			err := json.NewDecoder(recorder.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())

			// Verify the error message
			Expect(response).To(HaveKeyWithValue("error", testMessage))
		})
	})

	Context("GetNamespaceFromPath", func() {
		type testCase struct {
			path          string
			expected      string
			expectError   bool
			errorContains string
		}

		DescribeTable("extracts namespace from path correctly",
			func(tc testCase) {
				namespace, err := GetNamespaceFromPath(tc.path)

				if tc.expectError {
					Expect(err).To(HaveOccurred())
					if tc.errorContains != "" {
						Expect(err.Error()).To(ContainSubstring(tc.errorContains))
					}
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(namespace).To(Equal(tc.expected))
				}
			},
			Entry("standard path with namespace and trailing slash", testCase{
				path:        "/a/lot/of/sub/paths/namespaces/ns/",
				expected:    "ns",
				expectError: false,
			}),
			Entry("standard path with namespace and resource", testCase{
				path:        "/a/lot/of/sub/paths/namespaces/ns/workspace/ws1",
				expected:    "ns",
				expectError: false,
			}),
			Entry("path with singular 'namespace' instead of plural", testCase{
				path:          "/a/lot/of/sub/paths/namespace/ns/workspace/ws1",
				expected:      "",
				expectError:   true,
				errorContains: "cannot find the namespace",
			}),
			Entry("path with empty namespace", testCase{
				path:          "/a/lot/of/sub/paths/namespace//workspace/ws1",
				expected:      "",
				expectError:   true,
				errorContains: "cannot find the namespace",
			}),
			Entry("standard path with connection resource", testCase{
				path:        "/a/lot/of/sub/paths/namespaces/ns2/connection",
				expected:    "ns2",
				expectError: false,
			}),
			Entry("root path only", testCase{
				path:          "/",
				expected:      "",
				expectError:   true,
				errorContains: "cannot find the namespace",
			}),
			Entry("empty path", testCase{
				path:          "",
				expected:      "",
				expectError:   true,
				errorContains: "cannot find the namespace",
			}),
			Entry("api path with version and namespace", testCase{
				path:        "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/connection",
				expected:    "default",
				expectError: false,
			}),
		)
	})

	Context("GetUserFromHeaders", func() {
		It("Should return user from Kubernetes request context when available", func() {
			req := httptest.NewRequest("GET", "/test", nil)

			// Simulate Kubernetes authentication context
			userInfo := &user.DefaultInfo{Name: "k8s-authenticated-user"}
			ctx := request.WithUser(req.Context(), userInfo)
			req = req.WithContext(ctx)

			user := GetUserFromHeaders(req)
			Expect(user).To(Equal("k8s-authenticated-user"))
		})

		It("Should fallback to X-User header when Kubernetes context is not available", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-User", "header-user")

			user := GetUserFromHeaders(req)
			Expect(user).To(Equal("header-user"))
		})

		It("Should prioritize Kubernetes context over headers (anti-spoofing)", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-User", "spoofed-user")
			req.Header.Set("X-Remote-User", "another-spoofed-user")

			// Simulate Kubernetes authentication context
			userInfo := &user.DefaultInfo{Name: "real-k8s-user"}
			ctx := request.WithUser(req.Context(), userInfo)
			req = req.WithContext(ctx)

			user := GetUserFromHeaders(req)
			Expect(user).To(Equal("real-k8s-user"))
		})

		It("Should return user from X-Remote-User header when X-User is not present and no K8s context", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Remote-User", "remote-user")

			user := GetUserFromHeaders(req)
			Expect(user).To(Equal("remote-user"))
		})

		It("Should return empty string when no user info is available", func() {
			req := httptest.NewRequest("GET", "/test", nil)

			user := GetUserFromHeaders(req)
			Expect(user).To(Equal(""))
		})

		It("Should handle nil user info in Kubernetes context", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-User", "fallback-user")

			// Simulate Kubernetes context with nil user info
			ctx := request.WithUser(req.Context(), nil)
			req = req.WithContext(ctx)

			user := GetUserFromHeaders(req)
			Expect(user).To(Equal("fallback-user"))
		})

		It("Should handle empty username in Kubernetes context", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-User", "fallback-user")

			// Simulate Kubernetes context with empty username
			userInfo := &user.DefaultInfo{Name: ""}
			ctx := request.WithUser(req.Context(), userInfo)
			req = req.WithContext(ctx)

			user := GetUserFromHeaders(req)
			Expect(user).To(Equal("fallback-user"))
		})
	})
})
