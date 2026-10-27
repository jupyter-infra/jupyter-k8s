//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// Authmiddleware TLS (#479): with config-auth/default the auth middleware serves plain HTTP as before;
// with config-auth/tls it serves HTTPS (health included, so HTTPS probes pass) from a cert-manager
// certificate, and serves a renewed certificate without restarting the pod.
var _ = Describe("Authmiddleware TLS", Ordered, func() {
	const (
		routerNamespace = "jupyter-k8s-router"
		deploymentName  = "jupyter-k8s-authmiddleware"
		certificateName = "jupyter-k8s-authmiddleware-tls"
		tlsSecretName   = "jupyter-k8s-authmiddleware-tls"
		overlayDefault  = "config-auth/default"
		overlayTLS      = "config-auth/tls"
		curlImage       = "curlimages/curl:8.10.1"
	)

	healthURL := func(scheme string) string {
		return fmt.Sprintf("%s://%s.%s.svc:8080/health", scheme, deploymentName, routerNamespace)
	}

	// applyOverlay deploys a config-auth overlay, with the images under test, and waits for the rollout
	applyOverlay := func(overlay string) {
		GinkgoHelper()
		By(fmt.Sprintf("applying %s", overlay))
		manifests, err := utils.Run(exec.Command("kubectl", "kustomize", overlay))
		Expect(err).NotTo(HaveOccurred())
		manifests = strings.NewReplacer(
			localAuthmiddlewareImage, authmiddlewareImage,
			localRotatorImage, rotatorImage,
		).Replace(manifests)
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(manifests)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		_, err = utils.Run(exec.Command("kubectl", "rollout", "status", "deployment/"+deploymentName,
			"-n", routerNamespace, "--timeout=3m"))
		Expect(err).NotTo(HaveOccurred(), "authmiddleware did not roll out (probes failing?)")
	}

	// curlInCluster runs curl in a one-shot pod and returns its output
	curlInCluster := func(g Gomega, podName string, args ...string) string {
		GinkgoHelper()
		_, _ = utils.Run(exec.Command("kubectl", "delete", "pod", podName, "-n", routerNamespace,
			"--ignore-not-found", "--now"))
		runArgs := append([]string{"run", podName, "-n", routerNamespace, "--restart=Never",
			"--image=" + curlImage, "--command", "--", "curl"}, args...)
		_, err := utils.Run(exec.Command("kubectl", runArgs...))
		g.Expect(err).NotTo(HaveOccurred())
		g.Eventually(func(g Gomega) {
			phase, err := kubectlGet("pod", podName, routerNamespace, jsonPathStatusPhase)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(phase).To(BeElementOf("Succeeded", "Failed"))
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		output, err := utils.Run(exec.Command("kubectl", "logs", podName, "-n", routerNamespace))
		g.Expect(err).NotTo(HaveOccurred())
		_, _ = utils.Run(exec.Command("kubectl", "delete", "pod", podName, "-n", routerNamespace, "--now"))
		return output
	}

	// curlStatus returns the HTTP status curl got for url, or "000" when the request failed
	curlStatus := func(podName string, args ...string) string {
		GinkgoHelper()
		args = append([]string{"-s", "-o", "/dev/null", "-w", "status=%{http_code}"}, args...)
		output := curlInCluster(Default, podName, args...)
		return strings.TrimPrefix(strings.TrimSpace(output), "status=")
	}

	// servedCertStartDate returns the start date of the certificate the service presents
	startDatePattern := regexp.MustCompile(`start date: (.+)`)
	servedCertStartDate := func(g Gomega) string {
		output := curlInCluster(g, "authmw-tls-cert", "-skv", "-o", "/dev/null", healthURL("https"))
		match := startDatePattern.FindStringSubmatch(output)
		g.Expect(match).To(HaveLen(2), "no certificate start date in curl output:\n%s", output)
		return strings.TrimSpace(match[1])
	}

	// authmiddlewarePod returns the name and restart count of the one non-terminating authmiddleware pod
	authmiddlewarePod := func() (string, string) {
		GinkgoHelper()
		out, err := kubectlGetByLabels("pods", "app=authmiddleware", routerNamespace,
			`{range .items[*]}{.metadata.name} {.status.containerStatuses[0].restartCount} `+
				`{.metadata.deletionTimestamp}{"\n"}{end}`)
		Expect(err).NotTo(HaveOccurred())
		var live [][]string
		for _, line := range strings.Split(out, "\n") {
			if fields := strings.Fields(line); len(fields) == 2 {
				live = append(live, fields)
			}
		}
		Expect(live).To(HaveLen(1), "expected exactly one live authmiddleware pod, got %q", out)
		return live[0][0], live[0][1]
	}

	BeforeAll(func() {
		By("creating the router namespace")
		nsYaml, err := utils.Run(exec.Command("kubectl", verbCreate, "namespace", routerNamespace,
			"--dry-run=client", "-o", "yaml"))
		Expect(err).NotTo(HaveOccurred())
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(nsYaml)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "-k", overlayTLS, "--ignore-not-found"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", routerNamespace,
			"--ignore-not-found", "--timeout=2m"))
	})

	It("serves plain HTTP when TLS is not configured", func() {
		applyOverlay(overlayDefault)

		Expect(curlStatus("authmw-http", healthURL("http"))).To(Equal("200"))
		Expect(curlStatus("authmw-https-refused", "-k", healthURL("https"))).
			To(Equal("000"), "TLS handshake should fail against HTTP")
	})

	It("serves HTTPS, health included, when TLS is configured", func() {
		applyOverlay(overlayTLS)
		_, err := utils.Run(exec.Command("kubectl", "wait", "certificate/"+certificateName,
			"-n", routerNamespace, "--for=condition=Ready", "--timeout=2m"))
		Expect(err).NotTo(HaveOccurred())

		Expect(curlStatus("authmw-https", "-k", healthURL("https"))).To(Equal("200"))
		Expect(curlStatus("authmw-http-refused", healthURL("http"))).To(Equal("400"))
		Expect(curlStatus("authmw-tls12", "-k", "--tls-max", "1.2", healthURL("https"))).
			To(Equal("000"), "TLS 1.2 should be rejected")
	})

	It("serves a renewed certificate without restarting", func() {
		podBefore, restartsBefore := authmiddlewarePod()
		startDateBefore := servedCertStartDate(Default)

		By("forcing cert-manager to reissue the certificate")
		// Ensure the reissued certificate's start date, at second granularity, differs
		time.Sleep(time.Second)
		_, err := utils.Run(exec.Command("kubectl", "delete", "secret", tlsSecretName, "-n", routerNamespace))
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the pod to serve the new certificate")
		// The kubelet projects the new Secret within about a minute; the server polls the files every 10s
		Eventually(func(g Gomega) {
			g.Expect(servedCertStartDate(g)).NotTo(Equal(startDateBefore))
		}).WithTimeout(4 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		podAfter, restartsAfter := authmiddlewarePod()
		Expect(podAfter).To(Equal(podBefore), "authmiddleware pod was replaced")
		Expect(restartsAfter).To(Equal(restartsBefore), "authmiddleware container restarted")
	})
})
