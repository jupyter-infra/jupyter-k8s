/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

const staleUpdateValue = "force-stale-update"

// mockAccessStartupProber is a mock prober for testing
type mockAccessStartupProber struct {
	ready bool
	err   error
}

func (m *mockAccessStartupProber) Probe(
	_ context.Context,
	_ *workspacev1alpha1.Workspace,
	_ *workspacev1alpha1.WorkspaceAccessStrategy,
	_ *corev1.Service,
) (bool, error) {
	return m.ready, m.err
}

var _ = Describe("ProbeAccessStartup", func() {
	var (
		ctx            context.Context
		workspace      *workspacev1alpha1.Workspace
		accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
		sm             *StateMachine
		mockProber     *mockAccessStartupProber
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockProber = &mockAccessStartupProber{}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("probe-test-%d", time.Now().UnixNano()),
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image: "jupyter/base-notebook:latest",
			},
		}

		accessStrategy = &workspacev1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-strategy",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
				DisplayName:             "Test Strategy",
				AccessResourceTemplates: []workspacev1alpha1.AccessResourceTemplate{},
				AccessStartupProbe: &workspacev1alpha1.AccessStartupProbe{
					HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
						URLTemplate: "http://example.com/test",
					},
					PeriodSeconds:    1,
					FailureThreshold: 3,
				},
			},
		}

		sm = &StateMachine{
			accessStartupProber: mockProber,
			recorder:            record.NewFakeRecorder(10),
		}
	})

	It("should return nil when no probe is configured", func() {
		accessStrategy.Spec.AccessStartupProbe = nil

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("should succeed immediately when probe returns ready", func() {
		mockProber.ready = true

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(workspace.Status.AccessStartupProbeFailures).To(BeNil())
	})

	It("should set failures to 0 and requeue on first attempt with initialDelaySeconds", func() {
		accessStrategy.Spec.AccessStartupProbe.InitialDelaySeconds = 5

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeRetrying))
		Expect(result.RequeueAfter).To(Equal(5 * time.Second))
		Expect(workspace.Status.AccessStartupProbeFailures).NotTo(BeNil())
		Expect(*workspace.Status.AccessStartupProbeFailures).To(Equal(int32(0)))
		Expect(workspace.Status.LastAccessStartupProbeTime).NotTo(BeNil())
	})

	It("should probe immediately on first attempt without initialDelaySeconds", func() {
		mockProber.ready = true

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("should increment failures and requeue on probe failure", func() {
		mockProber.ready = false
		zero := int32(0)
		workspace.Status.AccessStartupProbeFailures = &zero

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeRetrying))
		Expect(result.RequeueAfter).To(Equal(1 * time.Second))
		Expect(*workspace.Status.AccessStartupProbeFailures).To(Equal(int32(1)))
	})

	It("should clear failures and return nil on success after previous failures", func() {
		mockProber.ready = true
		failures := int32(2)
		workspace.Status.AccessStartupProbeFailures = &failures

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(workspace.Status.AccessStartupProbeFailures).To(BeNil())
		Expect(workspace.Status.LastAccessStartupProbeTime).To(BeNil())
	})

	It("should return ProbeFailureThresholdExceeded when failureThreshold is exceeded", func() {
		mockProber.ready = false
		failures := int32(2) // threshold is 3, so next failure (3) >= threshold
		workspace.Status.AccessStartupProbeFailures = &failures

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeFailureThresholdExceeded))
		Expect(workspace.Status.AccessStartupProbeFailures).NotTo(BeNil())
		Expect(*workspace.Status.AccessStartupProbeFailures).To(Equal(int32(3)))
	})

	It("should return error when prober returns an error", func() {
		mockProber.err = fmt.Errorf("template resolution failed")
		zero := int32(0)
		workspace.Status.AccessStartupProbeFailures = &zero

		_, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template resolution failed"))
	})

	It("should skip probe and requeue when less than periodSeconds has elapsed", func() {
		mockProber.ready = false
		zero := int32(0)
		workspace.Status.AccessStartupProbeFailures = &zero
		recent := metav1.Now()
		workspace.Status.LastAccessStartupProbeTime = &recent

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbePendingRetry))
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(result.RequeueAfter).To(BeNumerically("<=", 1*time.Second))
		// Counter should NOT have been incremented
		Expect(*workspace.Status.AccessStartupProbeFailures).To(Equal(int32(0)))
	})

	It("should return ProbeFailureThresholdExceeded immediately when already at threshold", func() {
		failures := int32(3) // already at threshold
		workspace.Status.AccessStartupProbeFailures = &failures

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeFailureThresholdExceeded))
		Expect(*workspace.Status.AccessStartupProbeFailures).To(Equal(int32(3)))
	})

	It("should use default failureThreshold when not set", func() {
		mockProber.ready = false
		accessStrategy.Spec.AccessStartupProbe.FailureThreshold = 0
		failures := int32(DefaultAccessStartupProbeFailureThreshold - 1)
		workspace.Status.AccessStartupProbeFailures = &failures

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeFailureThresholdExceeded))
		Expect(workspace.Status.AccessStartupProbeFailures).NotTo(BeNil())
		Expect(*workspace.Status.AccessStartupProbeFailures).To(Equal(int32(DefaultAccessStartupProbeFailureThreshold)))
	})
})

var _ = Describe("clearProbeState", func() {
	It("should clear both failures and last probe time", func() {
		failures := int32(5)
		now := metav1.Now()
		workspace := &workspacev1alpha1.Workspace{}
		workspace.Status.AccessStartupProbeFailures = &failures
		workspace.Status.LastAccessStartupProbeTime = &now

		clearProbeState(workspace)

		Expect(workspace.Status.AccessStartupProbeFailures).To(BeNil())
		Expect(workspace.Status.LastAccessStartupProbeTime).To(BeNil())
	})

	It("should be a no-op when both fields are already nil", func() {
		workspace := &workspacev1alpha1.Workspace{}

		clearProbeState(workspace)

		Expect(workspace.Status.AccessStartupProbeFailures).To(BeNil())
		Expect(workspace.Status.LastAccessStartupProbeTime).To(BeNil())
	})
})

var _ = Describe("timeUntilNextProbe", func() {
	It("should return 0 when LastAccessStartupProbeTime is nil", func() {
		workspace := &workspacev1alpha1.Workspace{}

		Expect(timeUntilNextProbe(workspace, 2)).To(Equal(time.Duration(0)))
	})

	It("should return remaining duration when within period", func() {
		workspace := &workspacev1alpha1.Workspace{}
		recent := metav1.NewTime(time.Now().Add(-500 * time.Millisecond))
		workspace.Status.LastAccessStartupProbeTime = &recent

		remaining := timeUntilNextProbe(workspace, 2)
		Expect(remaining).To(BeNumerically(">", 0))
		Expect(remaining).To(BeNumerically("<=", 2*time.Second))
	})

	It("should return 0 when period has fully elapsed", func() {
		workspace := &workspacev1alpha1.Workspace{}
		old := metav1.NewTime(time.Now().Add(-5 * time.Second))
		workspace.Status.LastAccessStartupProbeTime = &old

		Expect(timeUntilNextProbe(workspace, 2)).To(Equal(time.Duration(0)))
	})

	It("should return 0 when exactly at period boundary", func() {
		workspace := &workspacev1alpha1.Workspace{}
		exact := metav1.NewTime(time.Now().Add(-2 * time.Second))
		workspace.Status.LastAccessStartupProbeTime = &exact

		Expect(timeUntilNextProbe(workspace, 2)).To(Equal(time.Duration(0)))
	})
})
