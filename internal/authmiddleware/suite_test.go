/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAuthmiddleware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Authmiddleware Suite")
}
