package extensionapi

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExtensionapi(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Extensionapi Suite")
}
