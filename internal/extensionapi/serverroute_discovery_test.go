package extensionapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServerRouteDiscovery", func() {
	Context("handleDiscovery", func() {
		var (
			server   *ExtensionServer
			recorder *httptest.ResponseRecorder
			req      *http.Request
		)

		BeforeEach(func() {
			// Setup for each test
			server = &ExtensionServer{}
			recorder = httptest.NewRecorder()
			req = httptest.NewRequest("GET", "/apis/connection.workspace.jupyter.org/v1alpha1", nil)
		})

		It("Should set the content header", func() {
			// Call the function under test
			server.handleDiscovery(recorder, req)

			// Verify content type header is set
			contentType := recorder.Header().Get("Content-Type")
			Expect(contentType).To(Equal("application/json"))
		})

		It("Should respond to a GET request", func() {
			// Call the function under test
			server.handleDiscovery(recorder, req)

			// Verify status code for GET request
			Expect(recorder.Code).To(Equal(http.StatusOK))

			// Verify that the response body is not empty
			Expect(recorder.Body.String()).NotTo(BeEmpty())
		})

		It("Should return a response body object with kind=APIResourceList", func() {
			// Call the function under test
			server.handleDiscovery(recorder, req)

			// Parse the response body as JSON
			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())

			// Verify the kind field
			Expect(response).To(HaveKey("kind"))
			Expect(response["kind"]).To(Equal("APIResourceList"))
		})

		It("Should return a response body object with apiVersion=v1", func() {
			// Call the function under test
			server.handleDiscovery(recorder, req)

			// Parse the response body as JSON
			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())

			// Verify the apiVersion field
			Expect(response).To(HaveKey("apiVersion"))
			Expect(response["apiVersion"]).To(Equal("v1"))
		})

		It("Should return a response body object with the correct groupVersion", func() {
			// Call the function under test
			server.handleDiscovery(recorder, req)

			// Parse the response body as JSON
			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())

			// Verify the groupVersion field
			Expect(response).To(HaveKey("groupVersion"))
			Expect(response["groupVersion"]).To(Equal("connection.workspace.jupyter.org/v1alpha1"))
		})

		It("Should return a response body object with resources with at least 2 entries", func() {
			// Call the function under test
			server.handleDiscovery(recorder, req)

			// Parse the response body as JSON
			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())

			// Verify the resources field exists and has at least 2 entries
			Expect(response).To(HaveKey("resources"))
			resources, ok := response["resources"].([]interface{})
			Expect(ok).To(BeTrue(), "resources should be an array")
			Expect(len(resources)).To(BeNumerically(">=", 2), "should have at least 2 resources")
		})

		It("Should include 'kind: Connection' and 'verbs: [create]' in one entry", func() {
			// Call the function under test
			server.handleDiscovery(recorder, req)

			// Parse the response body as JSON
			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())

			// Get the resources array
			resources, ok := response["resources"].([]interface{})
			Expect(ok).To(BeTrue(), "resources should be an array")

			// Find the Connection resource
			var connectionResource map[string]interface{}
			for _, res := range resources {
				resource, ok := res.(map[string]interface{})
				Expect(ok).To(BeTrue(), "resource should be an object")

				if resource["kind"] == "Connection" {
					connectionResource = resource
					break
				}
			}

			// Verify that we found the Connection resource
			Expect(connectionResource).NotTo(BeNil(), "should have a Connection resource")

			// Verify the Connection resource has the correct properties
			Expect(connectionResource["kind"]).To(Equal("Connection"))
			Expect(connectionResource["namespaced"]).To(BeTrue())

			// Verify the verbs array contains "create"
			verbs, ok := connectionResource["verbs"].([]interface{})
			Expect(ok).To(BeTrue(), "verbs should be an array")
			Expect(verbs).To(ContainElement("create"))
		})

		It("Should include 'kind: ConnectionAccessReview' and 'verbs: [create]' in one entry", func() {
			// Call the function under test
			server.handleDiscovery(recorder, req)

			// Parse the response body as JSON
			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())

			// Get the resources array
			resources, ok := response["resources"].([]interface{})
			Expect(ok).To(BeTrue(), "resources should be an array")

			// Find the ConnectionAccessReview resource
			var connectionAccessReviewResource map[string]interface{}
			for _, res := range resources {
				resource, ok := res.(map[string]interface{})
				Expect(ok).To(BeTrue(), "resource should be an object")

				if resource["kind"] == "ConnectionAccessReview" {
					connectionAccessReviewResource = resource
					break
				}
			}

			// Verify that we found the ConnectionAccessReview resource
			Expect(connectionAccessReviewResource).NotTo(BeNil(), "should have a ConnectionAccessReview resource")

			// Verify the ConnectionAccessReview resource has the correct properties
			Expect(connectionAccessReviewResource["kind"]).To(Equal("ConnectionAccessReview"))
			Expect(connectionAccessReviewResource["namespaced"]).To(BeTrue())

			// Verify the verbs array contains "create"
			verbs, ok := connectionAccessReviewResource["verbs"].([]interface{})
			Expect(ok).To(BeTrue(), "verbs should be an array")
			Expect(verbs).To(ContainElement("create"))
		})

		It("Should write a 500 if writing the body fails", func() {
			// Create an error writer that will fail on Write
			errorWriter := &ErrorWriter{}

			// Call the function with our error writer
			server.handleDiscovery(errorWriter, req)

			// The WriteError function should be called when the initial Write fails
			// This will set the status code to 500
			Expect(errorWriter.statusCode).To(Equal(http.StatusInternalServerError))

			// Also verify that the content type header was set correctly in the error response
			contentType := errorWriter.Header().Get("Content-Type")
			Expect(contentType).To(Equal("application/json"))
		})
	})
})
