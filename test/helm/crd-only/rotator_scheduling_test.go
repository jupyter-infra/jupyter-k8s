/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package crdonly_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

// The helm-test render (see Makefile) sets manager scheduling but leaves the
// rotator's own scheduling empty, so the rotator CronJob must inherit the
// manager's nodeSelector/tolerations via the `| default .Values.manager.X`
// fallback in the template. This guards that wiring (issue #382).
var _ = Describe("JWT Rotator Scheduling", func() {
	It("should inherit manager scheduling on the rotator CronJob pod spec", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		rotatorPath := filepath.Join(rootDir,
			"dist/test-output-crd-only/jupyter-k8s/templates/extras/jwt-rotator.yaml")
		data, err := os.ReadFile(rotatorPath)
		Expect(err).NotTo(HaveOccurred(),
			"rotator CronJob should be rendered (helm-test sets extensionApi.jwtSecret.enable=true)")

		var cronJob struct {
			Kind string `json:"kind"`
			Spec struct {
				JobTemplate struct {
					Spec struct {
						Template struct {
							Spec struct {
								NodeSelector map[string]string `json:"nodeSelector"`
								Tolerations  []struct {
									Key      string `json:"key"`
									Operator string `json:"operator"`
								} `json:"tolerations"`
							} `json:"spec"`
						} `json:"template"`
					} `json:"spec"`
				} `json:"jobTemplate"`
			} `json:"spec"`
		}
		Expect(yaml.Unmarshal(data, &cronJob)).To(Succeed())
		Expect(cronJob.Kind).To(Equal("CronJob"))

		podSpec := cronJob.Spec.JobTemplate.Spec.Template.Spec
		// Inherited from manager.nodeSelector set in the helm-test render.
		Expect(podSpec.NodeSelector).To(HaveKeyWithValue("jupyter-deploy/role", "components"))
		// Inherited from manager.tolerations set in the helm-test render.
		Expect(podSpec.Tolerations).To(HaveLen(1))
		Expect(podSpec.Tolerations[0].Key).To(Equal("dedicated"))
		Expect(podSpec.Tolerations[0].Operator).To(Equal("Exists"))
	})
})
