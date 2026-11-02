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

	// curl's own bounds, so a probe against an unreachable endpoint fails fast instead of hanging
	curlBounds := []string{"--connect-timeout", "5", "--max-time", "15"}

	// curlInCluster runs bounded curl in a one-shot pod and returns its output
	curlInCluster := func(g Gomega, podName string, args ...string) string {
		GinkgoHelper()
		_, _ = utils.Run(exec.Command("kubectl", "delete", "pod", podName, "-n", routerNamespace,
			"--ignore-not-found", "--now"))
		runArgs := append([]string{"run", podName, "-n", routerNamespace, "--restart=Never",
			"--image=" + curlImage, "--command", "--", "curl"}, curlBounds...)
		_, err := utils.Run(exec.Command("kubectl", append(runArgs, args...)...))
		g.Expect(err).NotTo(HaveOccurred())
		g.Eventually(func(g Gomega) {
			phase, err := kubectlGet("pod", podName, routerNamespace, jsonPathStatusPhase)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(phase).To(BeElementOf("Succeeded", "Failed"))
		}).WithTimeout(2*time.Minute).WithPolling(2*time.Second).Should(Succeed(), "curl pod %s did not complete", podName)
		output, err := utils.Run(exec.Command("kubectl", "logs", podName, "-n", routerNamespace))
		g.Expect(err).NotTo(HaveOccurred())
		_, _ = utils.Run(exec.Command("kubectl", "delete", "pod", podName, "-n", routerNamespace, "--now"))
		return output
	}

	// curlProbe returns the HTTP status ("000" when no response) and the curl exit code for a request
	probePattern := regexp.MustCompile(`status=(\d{3}) exit=(\d+)`)
	curlProbe := func(g Gomega, podName string, args ...string) (string, string) {
		GinkgoHelper()
		args = append([]string{"-s", "-o", "/dev/null", "-w", "status=%{http_code} exit=%{exitcode}"}, args...)
		output := curlInCluster(g, podName, args...)
		match := probePattern.FindStringSubmatch(output)
		g.Expect(match).To(HaveLen(3), "unexpected curl output:\n%s", output)
		return match[1], match[2]
	}

	// expectHTTPStatus retries until a request gets the wanted HTTP status. Right after a rollout, the
	// service may briefly still route to the old pod.
	expectHTTPStatus := func(want, podName string, args ...string) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			status, exitCode := curlProbe(g, podName, args...)
			g.Expect(status).To(Equal(want), "curl exit code %s", exitCode)
		}).WithTimeout(90 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
	}

	// expectTLSHandshakeFailure retries until a request fails the TLS handshake (curl exit 35); a
	// connection failure (7) or timeout (28) would also report status "000", so it does not count.
	const curlExitSSLConnectError = "35"
	expectTLSHandshakeFailure := func(reason, podName string, args ...string) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			status, exitCode := curlProbe(g, podName, args...)
			g.Expect(exitCode).To(Equal(curlExitSSLConnectError), "%s (HTTP status %s)", reason, status)
		}).WithTimeout(90 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
	}

	// servedCertStartDate returns the start date of the certificate the service presents
	startDatePattern := regexp.MustCompile(`start date: (.+)`)
	servedCertStartDate := func(g Gomega) string {
		output := curlInCluster(g, "authmw-tls-cert", "-skv", "-o", "/dev/null", healthURL("https"))
		match := startDatePattern.FindStringSubmatch(output)
		g.Expect(match).To(HaveLen(2), "no certificate start date in curl output:\n%s", output)
		return strings.TrimSpace(match[1])
	}

	// liveAuthmiddlewarePods returns name, restart count and IP of the non-terminating authmiddleware pods
	liveAuthmiddlewarePods := func(g Gomega) [][]string {
		GinkgoHelper()
		out, err := kubectlGetByLabels("pods", "app=authmiddleware", routerNamespace,
			`{range .items[*]}{.metadata.name},{.status.containerStatuses[0].restartCount},`+
				`{.status.podIP},{.metadata.deletionTimestamp}{"\n"}{end}`)
		g.Expect(err).NotTo(HaveOccurred())
		var live [][]string
		for _, line := range strings.Split(out, "\n") {
			if fields := strings.Split(strings.TrimSpace(line), ","); len(fields) == 4 && fields[3] == "" {
				live = append(live, fields[:3])
			}
		}
		return live
	}

	// authmiddlewarePod returns the name and restart count of the one non-terminating authmiddleware pod
	authmiddlewarePod := func() (string, string) {
		GinkgoHelper()
		live := liveAuthmiddlewarePods(Default)
		Expect(live).To(HaveLen(1), "expected exactly one live authmiddleware pod, got %q", live)
		return live[0][0], live[0][1]
	}

	// waitForServiceEndpoints waits until the service's EndpointSlices list only the live pod, so
	// probes do not hit the pod the last rollout replaced
	waitForServiceEndpoints := func() {
		GinkgoHelper()
		By("waiting for the service endpoints to list only the new pod")
		Eventually(func(g Gomega) {
			live := liveAuthmiddlewarePods(g)
			g.Expect(live).To(HaveLen(1), "expected exactly one live authmiddleware pod, got %q", live)
			endpoints, err := kubectlGetByLabels("endpointslices", "kubernetes.io/service-name="+deploymentName,
				routerNamespace, `{range .items[*].endpoints[*]}{.addresses[0]}={.conditions.ready}{" "}{end}`)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.Fields(endpoints)).To(ConsistOf(live[0][2] + "=true"))
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
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
		waitForServiceEndpoints()
	}

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		for _, args := range [][]string{
			{"get", "pods,endpointslices", "-n", routerNamespace, "-o", "wide"},
			{"describe", "pods", "-n", routerNamespace},
			{"logs", "-l", "run", "-n", routerNamespace, "--prefix"},
			{"logs", "-l", "app=authmiddleware", "-n", routerNamespace, "--all-containers", "--prefix", "--tail=200"},
			{"get", "events", "-n", routerNamespace, "--sort-by=.lastTimestamp"},
		} {
			output, err := utils.Run(exec.Command("kubectl", args...))
			_, _ = fmt.Fprintf(GinkgoWriter, "--- kubectl %s (err: %v)\n%s\n", strings.Join(args, " "), err, output)
		}
	})

	AfterAll(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "-k", overlayTLS, "--ignore-not-found"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", routerNamespace,
			"--ignore-not-found", "--timeout=2m"))
	})

	It("serves plain HTTP when TLS is not configured", func() {
		applyOverlay(overlayDefault)

		expectHTTPStatus("200", "authmw-http", healthURL("http"))
		expectTLSHandshakeFailure("TLS handshake should fail against HTTP", "authmw-https-refused",
			"-k", healthURL("https"))
	})

	It("serves HTTPS, health included, when TLS is configured", func() {
		applyOverlay(overlayTLS)
		_, err := utils.Run(exec.Command("kubectl", "wait", "certificate/"+certificateName,
			"-n", routerNamespace, "--for=condition=Ready", "--timeout=2m"))
		Expect(err).NotTo(HaveOccurred())

		expectHTTPStatus("200", "authmw-https", "-k", healthURL("https"))
		expectHTTPStatus("400", "authmw-http-refused", healthURL("http"))
		expectTLSHandshakeFailure("TLS 1.2 should be rejected", "authmw-tls12",
			"-k", "--tls-max", "1.2", healthURL("https"))
	})

	It("serves a renewed certificate without restarting", func() {
		podBefore, restartsBefore := authmiddlewarePod()
		var startDateBefore string
		Eventually(func(g Gomega) {
			startDateBefore = servedCertStartDate(g)
		}).WithTimeout(90 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

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
