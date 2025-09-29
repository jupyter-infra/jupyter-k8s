package aws_traefik_dex_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAwsTraefikDex(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AWS Traefik Dex Suite")
}
