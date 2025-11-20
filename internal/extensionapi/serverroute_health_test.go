/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServerRouteHealth", func() {
	Context("handleHealth", func() {
		var (
			server   *ExtensionServer
			recorder *httptest.ResponseRecorder
			req      *http.Request
		)

		BeforeEach(func() {
			// Setup for each test
			server = &ExtensionServer{}
			recorder = httptest.NewRecorder()
			req = httptest.NewRequest("GET", "/health", nil)
		})

		It("Should set the content header", func() {
			// Call the function under test
			server.handleHealth(recorder, req)

			// Verify content type header is set
			contentType := recorder.Header().Get("Content-Type")
			Expect(contentType).To(Equal("application/json"))
		})

		It("Should set the 200 HTTP status", func() {
			// Call the function under test
			server.handleHealth(recorder, req)

			// Verify status code
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})

		It("Should write a simple response body", func() {
			// Call the function under test
			server.handleHealth(recorder, req)

			// Verify response body
			Expect(recorder.Body.String()).To(Equal(`{"status":"ok"}`))
		})

		It("Should write a 500 internal error if writing the body fails", func() {
			// Create an error writer that will fail on Write
			errorWriter := &ErrorWriter{}

			// Call the function with our error writer
			server.handleHealth(errorWriter, req)

			// The WriteError function should be called when the initial Write fails
			// This will set the status code to 500
			Expect(errorWriter.statusCode).To(Equal(http.StatusInternalServerError))

			// Also verify that the content type header was set correctly in the error response
			contentType := errorWriter.Header().Get("Content-Type")
			Expect(contentType).To(Equal("application/json"))
		})
	})
})
