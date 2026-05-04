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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

const staleUpdateValue = "force-stale-update"

// mockAccessStartupProber is a mock prober for testing
type mockAccessStartupProber struct {
	ready      bool
	err        error
	probeCount int
}

func (m *mockAccessStartupProber) Probe(
	_ context.Context,
	_ *workspacev1alpha1.Workspace,
	_ *workspacev1alpha1.WorkspaceAccessStrategy,
	_ *corev1.Service,
) (bool, error) {
	m.probeCount++
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
				Name:       "test-strategy",
				Namespace:  "default",
				UID:        types.UID("test-uid"),
				Generation: 1,
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

		workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d",
			accessStrategy.UID, accessStrategy.Generation)

		sm = &StateMachine{
			accessStartupProber: mockProber,
			recorder:            record.NewFakeRecorder(10),
		}
	})

	It("should return ProbeNotDefined when no probe is configured", func() {
		accessStrategy.Spec.AccessStartupProbe = nil

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeNotDefined))
	})

	It("should return ProbeSucceeded when probe returns ready", func() {
		mockProber.ready = true

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeSucceeded))
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
		Expect(workspace.Status.EarliestNextProbeTime).NotTo(BeNil())
	})

	It("should probe immediately on first attempt without initialDelaySeconds", func() {
		mockProber.ready = true

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeSucceeded))
	})

	It("should increment failures and requeue on probe failure", func() {
		mockProber.ready = false
		zero := int32(0)
		workspace.Status.AccessStartupProbeFailures = &zero

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeRetrying))
		// failureThreshold=3 < ProbeBackoffThreshold, so backoff is active
		// from the start: first failure → periodSeconds*2 = 2s
		Expect(result.RequeueAfter).To(Equal(2 * time.Second))
		Expect(*workspace.Status.AccessStartupProbeFailures).To(Equal(int32(1)))
	})

	It("should clear failures and return ProbeSucceeded on success after previous failures", func() {
		mockProber.ready = true
		failures := int32(2)
		workspace.Status.AccessStartupProbeFailures = &failures

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeSucceeded))
		Expect(workspace.Status.AccessStartupProbeFailures).To(BeNil())
		Expect(workspace.Status.EarliestNextProbeTime).To(BeNil())
	})

	It("should return ProbeAlreadySucceeded when access resources are already ready", func() {
		workspace.Status.AccessStartupProbeSucceeded = true

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal(ProbeAlreadySucceeded))
		Expect(mockProber.probeCount).To(Equal(0))
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

	It("should skip probe and requeue when deadline has not passed", func() {
		mockProber.ready = false
		zero := int32(0)
		workspace.Status.AccessStartupProbeFailures = &zero
		future := metav1.NewTime(time.Now().Add(1 * time.Second))
		workspace.Status.EarliestNextProbeTime = &future

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

	It("should use normal periodSeconds in the fast phase", func() {
		mockProber.ready = false
		// threshold=20, backoff starts at failure 11
		// failure 9 is still in the fast phase
		accessStrategy.Spec.AccessStartupProbe.FailureThreshold = 20
		failures := int32(8)
		workspace.Status.AccessStartupProbeFailures = &failures

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeRetrying))
		Expect(result.RequeueAfter).To(Equal(1 * time.Second))
	})

	It("should start exponential backoff in the last 10 retries", func() {
		mockProber.ready = false
		// threshold=20, backoff starts at failure 11
		accessStrategy.Spec.AccessStartupProbe.FailureThreshold = 20

		failures := int32(10)
		workspace.Status.AccessStartupProbeFailures = &failures

		// Failure 11 (1st in backoff phase) → 1*2=2s
		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeRetrying))
		Expect(result.RequeueAfter).To(Equal(2 * time.Second))

		workspace.Status.EarliestNextProbeTime = nil

		// Failure 12 → 1*4=4s
		result, err = sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeRetrying))
		Expect(result.RequeueAfter).To(Equal(4 * time.Second))

		workspace.Status.EarliestNextProbeTime = nil

		// Failure 13 → 1*8=8s
		result, err = sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeRetrying))
		Expect(result.RequeueAfter).To(Equal(8 * time.Second))
	})

	It("should use all backoff when failureThreshold <= ProbeBackoffThreshold", func() {
		mockProber.ready = false
		accessStrategy.Spec.AccessStartupProbe.FailureThreshold = 5
		zero := int32(0)
		workspace.Status.AccessStartupProbeFailures = &zero

		// threshold=5 < ProbeBackoffThreshold=10, so backoff from failure 1
		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeRetrying))
		Expect(result.RequeueAfter).To(Equal(2 * time.Second))
	})

	It("should reset probe when AccessStrategy generation changes (degraded self-heal)", func() {
		mockProber.ready = false
		failures := int32(3) // already at threshold
		workspace.Status.AccessStartupProbeFailures = &failures

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeFailureThresholdExceeded))

		// Simulate AccessStrategy spec update (generation bump)
		accessStrategy.Generation = 2
		mockProber.ready = true

		result, err = sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeSucceeded))
		Expect(workspace.Status.AccessStartupProbeSucceeded).To(BeTrue())
		Expect(workspace.Status.ObservedAccessStrategyVersion).To(Equal(
			fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)))
	})

	It("should reset probe when AccessStrategy generation changes (already succeeded)", func() {
		workspace.Status.AccessStartupProbeSucceeded = true

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeAlreadySucceeded))

		// Simulate AccessStrategy spec update (generation bump)
		accessStrategy.Generation = 2
		mockProber.ready = false

		result, err = sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeRetrying))
		Expect(workspace.Status.AccessStartupProbeSucceeded).To(BeFalse())
	})

	It("should reset probe when AccessStrategy UID changes (ref switch)", func() {
		workspace.Status.AccessStartupProbeSucceeded = true

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeAlreadySucceeded))

		// Simulate switching to a different AccessStrategy (different UID, same generation)
		accessStrategy.UID = types.UID("different-uid")
		accessStrategy.Generation = 1
		mockProber.ready = true

		result, err = sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeSucceeded))
		Expect(workspace.Status.ObservedAccessStrategyVersion).To(Equal("different-uid.1"))
	})

	It("should not reset probe when AccessStrategy version has not changed", func() {
		mockProber.ready = false
		failures := int32(1)
		workspace.Status.AccessStartupProbeFailures = &failures

		result, err := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(ProbeRetrying))
		Expect(*workspace.Status.AccessStartupProbeFailures).To(Equal(int32(2)))
	})
})

var _ = Describe("clearProbeState", func() {
	It("should clear failures and earliest next probe time but not version", func() {
		failures := int32(5)
		future := metav1.NewTime(time.Now().Add(5 * time.Second))
		workspace := &workspacev1alpha1.Workspace{}
		workspace.Status.AccessStartupProbeFailures = &failures
		workspace.Status.EarliestNextProbeTime = &future
		workspace.Status.ObservedAccessStrategyVersion = "some-uid.1"

		clearProbeState(workspace)

		Expect(workspace.Status.AccessStartupProbeFailures).To(BeNil())
		Expect(workspace.Status.EarliestNextProbeTime).To(BeNil())
		Expect(workspace.Status.ObservedAccessStrategyVersion).To(Equal("some-uid.1"))
	})

	It("should be a no-op when both fields are already nil", func() {
		workspace := &workspacev1alpha1.Workspace{}

		clearProbeState(workspace)

		Expect(workspace.Status.AccessStartupProbeFailures).To(BeNil())
		Expect(workspace.Status.EarliestNextProbeTime).To(BeNil())
	})
})

var _ = Describe("timeUntilProbeDeadline", func() {
	It("should return 0 when EarliestNextProbeTime is nil", func() {
		workspace := &workspacev1alpha1.Workspace{}

		Expect(timeUntilProbeDeadline(workspace)).To(Equal(time.Duration(0)))
	})

	It("should return remaining duration when deadline is in the future", func() {
		workspace := &workspacev1alpha1.Workspace{}
		future := metav1.NewTime(time.Now().Add(1500 * time.Millisecond))
		workspace.Status.EarliestNextProbeTime = &future

		remaining := timeUntilProbeDeadline(workspace)
		Expect(remaining).To(BeNumerically(">", 0))
		Expect(remaining).To(BeNumerically("<=", 1500*time.Millisecond))
	})

	It("should return 0 when deadline has passed", func() {
		workspace := &workspacev1alpha1.Workspace{}
		past := metav1.NewTime(time.Now().Add(-5 * time.Second))
		workspace.Status.EarliestNextProbeTime = &past

		Expect(timeUntilProbeDeadline(workspace)).To(Equal(time.Duration(0)))
	})

	It("should return 0 when deadline is exactly now", func() {
		workspace := &workspacev1alpha1.Workspace{}
		now := metav1.NewTime(time.Now())
		workspace.Status.EarliestNextProbeTime = &now

		Expect(timeUntilProbeDeadline(workspace)).To(Equal(time.Duration(0)))
	})
})

var _ = Describe("probeRetrySeconds", func() {
	It("should return periodSeconds in the fast phase", func() {
		// failureThreshold=20, backoffStart=10
		Expect(probeRetrySeconds(2, 0, 20)).To(Equal(int32(2)))
		Expect(probeRetrySeconds(2, 5, 20)).To(Equal(int32(2)))
		Expect(probeRetrySeconds(2, 10, 20)).To(Equal(int32(2)))
	})

	It("should double on each failure in the backoff phase", func() {
		// failureThreshold=20, backoffStart=10: failure 11→4, 12→8, 13→16
		Expect(probeRetrySeconds(2, 11, 20)).To(Equal(int32(4)))
		Expect(probeRetrySeconds(2, 12, 20)).To(Equal(int32(8)))
		Expect(probeRetrySeconds(2, 13, 20)).To(Equal(int32(16)))
	})

	It("should cap at ProbeBackoffMaxRetrySeconds", func() {
		Expect(probeRetrySeconds(2, 100, 20)).To(Equal(int32(ProbeBackoffMaxRetrySeconds)))
		Expect(probeRetrySeconds(10, 100, 20)).To(Equal(int32(ProbeBackoffMaxRetrySeconds)))
	})

	It("should use all backoff when failureThreshold <= ProbeBackoffThreshold", func() {
		// failureThreshold=5, backoffStart=max(5-10,0)=0
		Expect(probeRetrySeconds(2, 1, 5)).To(Equal(int32(4)))
		Expect(probeRetrySeconds(2, 2, 5)).To(Equal(int32(8)))
		Expect(probeRetrySeconds(2, 3, 5)).To(Equal(int32(16)))
	})
})
