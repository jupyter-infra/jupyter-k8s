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
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Logging", func() {
	var (
		ctx    context.Context
		logger logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create a simple logr.Logger implementation
		logger = logr.Discard()
	})

	Context("AddLoggerToContext", func() {
		It("Should add the logger, and return the modified context", func() {
			newCtx := AddLoggerToContext(ctx, logger)

			// Check the contexts are different
			Expect(newCtx).NotTo(BeIdenticalTo(ctx))

			// Retrieve the logger from the context
			retrievedLogger := newCtx.Value(loggerKey{})
			Expect(retrievedLogger).NotTo(BeNil())
		})

		It("Should support a nil context value", func() {
			// Create a nil logger value
			var nilLogger logr.Logger

			newCtx := AddLoggerToContext(ctx, nilLogger)

			// Check the contexts are different
			Expect(newCtx).NotTo(BeIdenticalTo(ctx))

			// The key should exist in the context, but the value can be zero-valued
			Expect(newCtx.Value(loggerKey{})).To(BeZero())
		})
	})

	Context("GetLoggerFromContext", func() {
		It("Should return the logger when available in context", func() {
			// Add a logger to the context
			ctxWithLogger := AddLoggerToContext(ctx, logger)

			// Retrieve the logger using the function
			retrievedLogger := GetLoggerFromContext(ctxWithLogger)

			// The retrieved logger should not be nil
			Expect(retrievedLogger).NotTo(BeNil())
		})

		It("Should return a discarding logger when not available in context", func() {
			// Use a context without a logger
			retrievedLogger := GetLoggerFromContext(ctx)

			// The retrieved logger should not be nil (it should be a discard logger)
			Expect(retrievedLogger).NotTo(BeNil())

			// We can't directly check if it's a discard logger, but we can
			// verify it doesn't panic when used
			Expect(func() {
				retrievedLogger.Info("This should be discarded")
			}).NotTo(Panic())
		})
	})
})
