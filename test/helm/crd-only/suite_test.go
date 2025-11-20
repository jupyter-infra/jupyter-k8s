/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package crdonly_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCrdOnlyHelmResources(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRD-Only Helm Resources Suite")
}
