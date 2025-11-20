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
