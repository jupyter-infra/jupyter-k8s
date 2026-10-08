/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

const (
	stepTestPodName   = "ws-pod"
	stepTestInitName  = "init"
	stepTestFinalizer = "test.jupyter.org/keep"
	stepTestOtherName = "other"
	stepTestPodUID    = types.UID("pod-uid-1")
	stepTestSchedMsg  = "0/1 nodes are available: 1 Insufficient nvidia.com/gpu."
	stepTestNominated = "Pod should schedule on: nodeclaim/workspace-gpu-abc12"
	stepTestPulling   = "Pulling image \"jupyter/base-notebook:latest\""
)

func waitingStatus(reason, message string) corev1.ContainerStatus {
	return corev1.ContainerStatus{Name: containerNameMain, State: corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: message},
	}}
}

func scheduledPod(statuses ...corev1.ContainerStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: stepTestPodName, Namespace: testNamespaceName, UID: stepTestPodUID},
		Spec:       corev1.PodSpec{NodeName: "node-1"},
		Status: corev1.PodStatus{
			Phase:             corev1.PodPending,
			Conditions:        []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}},
			ContainerStatuses: statuses,
		},
	}
}

func unscheduledPod(message string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: stepTestPodName, Namespace: testNamespaceName, UID: stepTestPodUID},
		Status: corev1.PodStatus{Phase: corev1.PodPending, Conditions: []corev1.PodCondition{{
			Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: message,
		}}},
	}
}

func podEvent(name, component, reason, message string, at time.Time) corev1.Event {
	return corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: name, Namespace: testNamespaceName},
		InvolvedObject: corev1.ObjectReference{Kind: KindPod, Name: stepTestPodName, Namespace: testNamespaceName, UID: stepTestPodUID},
		Reason:         reason,
		Message:        message,
		Source:         corev1.EventSource{Component: component},
		LastTimestamp:  metav1.NewTime(at),
	}
}

func TestDefinitiveStartFailure(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected *StartFailure
	}{
		{name: "no container statuses", pod: scheduledPod()},
		{name: "container creating is not a failure", pod: scheduledPod(waitingStatus(kubeletReasonContainerCreating, ""))},
		{name: "running container", pod: scheduledPod(corev1.ContainerStatus{Name: containerNameMain,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}})},
		{name: "image pull back-off", pod: scheduledPod(waitingStatus(kubeletReasonImagePullBackOff, "Back-off pulling image \"x:1\"")),
			expected: &StartFailure{Reason: kubeletReasonImagePullBackOff, Message: "Back-off pulling image \"x:1\""}},
		{name: "image pull error", pod: scheduledPod(waitingStatus(kubeletReasonErrImagePull, "rpc error: not found")),
			expected: &StartFailure{Reason: kubeletReasonErrImagePull, Message: "rpc error: not found"}},
		{name: "invalid image name", pod: scheduledPod(waitingStatus(kubeletReasonInvalidImageName, "couldn't parse image name")),
			expected: &StartFailure{Reason: kubeletReasonInvalidImageName, Message: "couldn't parse image name"}},
		{name: "config error", pod: scheduledPod(waitingStatus(kubeletReasonCreateConfigError, "secret \"s\" not found")),
			expected: &StartFailure{Reason: kubeletReasonCreateConfigError, Message: "secret \"s\" not found"}},
		{name: "crash loop with an empty message falls back to the reason",
			pod:      scheduledPod(waitingStatus(kubeletReasonCrashLoopBackOff, "")),
			expected: &StartFailure{Reason: kubeletReasonCrashLoopBackOff, Message: kubeletReasonCrashLoopBackOff}},
		{name: "init container failure is reported before a creating main container",
			pod: func() *corev1.Pod {
				pod := scheduledPod(waitingStatus(kubeletReasonContainerCreating, ""))
				pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: stepTestInitName, State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{Reason: kubeletReasonCrashLoopBackOff, Message: "back-off 10s restarting failed container"},
				}}}
				return pod
			}(),
			expected: &StartFailure{Reason: kubeletReasonCrashLoopBackOff, Message: "back-off 10s restarting failed container"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, definitiveStartFailure(tt.pod))
		})
	}
}

func TestPodStartStep(t *testing.T) {
	base := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		pod      *corev1.Pod
		events   []corev1.Event
		expected StartStep
	}{
		{
			name:     "unscheduled pod without events carries the scheduler's verdict",
			pod:      unscheduledPod(stepTestSchedMsg),
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestSchedMsg},
		},
		{
			name:     "unscheduled pod without verdict or events",
			pod:      unscheduledPod(""),
			expected: StartStep{Reason: ReasonWaitingForNode, Message: waitingForNodeMessage},
		},
		{
			name: "the autoscaler's event wins over a newer FailedScheduling repeat",
			pod:  unscheduledPod(stepTestSchedMsg),
			events: []corev1.Event{
				podEvent("sched", "default-scheduler", schedulerEventFailedScheduling, stepTestSchedMsg, base.Add(30*time.Second)),
				podEvent("nominated", "karpenter", "Nominated", stepTestNominated, base),
			},
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestNominated},
		},
		{
			name: "the newest non-scheduler event wins among several",
			pod:  unscheduledPod(stepTestSchedMsg),
			events: []corev1.Event{
				podEvent("nominated", "karpenter", "Nominated", stepTestNominated, base),
				podEvent("failed", "karpenter", "FailedScheduling", "ignored: scheduler reason", base.Add(time.Minute)),
				podEvent("limits", "karpenter", "Failed", "all available instance types exceed limits for nodepool", base.Add(2*time.Minute)),
			},
			expected: StartStep{Reason: ReasonWaitingForNode, Message: "all available instance types exceed limits for nodepool"},
		},
		{
			name: "only scheduler events fall back to the verdict",
			pod:  unscheduledPod(stepTestSchedMsg),
			events: []corev1.Event{
				podEvent("sched", "default-scheduler", schedulerEventFailedScheduling, stepTestSchedMsg, base),
			},
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestSchedMsg},
		},
		{
			name:     "scheduled pod pulling its image",
			pod:      scheduledPod(waitingStatus(kubeletReasonContainerCreating, "")),
			events:   []corev1.Event{podEvent("pulling", kubeletComponent, kubeletEventPulling, stepTestPulling, base)},
			expected: StartStep{Reason: ReasonPullingImage, Message: stepTestPulling},
		},
		{
			name: "a later kubelet event ends the pull step",
			pod:  scheduledPod(waitingStatus(kubeletReasonContainerCreating, "")),
			events: []corev1.Event{
				podEvent("pulling", kubeletComponent, kubeletEventPulling, stepTestPulling, base),
				podEvent("pulled", kubeletComponent, "Pulled", "Successfully pulled image in 42s", base.Add(42*time.Second)),
			},
			expected: StartStep{Reason: ReasonStartingContainer, Message: kubeletReasonContainerCreating},
		},
		{
			name:     "scheduled pod with a waiting container and no events",
			pod:      scheduledPod(waitingStatus("PodInitializing", "")),
			expected: StartStep{Reason: ReasonStartingContainer, Message: "PodInitializing"},
		},
		{
			name: "running but not ready container reports the kubelet's newest message",
			pod: scheduledPod(corev1.ContainerStatus{Name: containerNameMain,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}),
			events:   []corev1.Event{podEvent("unhealthy", kubeletComponent, "Unhealthy", "Readiness probe failed: connection refused", base)},
			expected: StartStep{Reason: ReasonStartingContainer, Message: "Readiness probe failed: connection refused"},
		},
		{
			name:     "running but not ready container without events",
			pod:      scheduledPod(corev1.ContainerStatus{Name: containerNameMain, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}),
			expected: StartStep{Reason: ReasonStartingContainer, Message: startingContainerMessage},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, podStartStep(tt.pod, tt.events))
		})
	}
}

func TestEventTime(t *testing.T) {
	base := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	event := corev1.Event{FirstTimestamp: metav1.NewTime(base), LastTimestamp: metav1.NewTime(base.Add(time.Minute))}
	assert.Equal(t, base.Add(time.Minute), eventTime(&event))
	// events.k8s.io style: eventTime and series, no last timestamp
	series := corev1.Event{EventTime: metav1.NewMicroTime(base), Series: &corev1.EventSeries{LastObservedTime: metav1.NewMicroTime(base.Add(2 * time.Minute))}}
	assert.Equal(t, base.Add(2*time.Minute), eventTime(&series))
}

// countingReader counts List calls and can be made to fail.
type countingReader struct {
	client.Reader
	lists   int
	failing bool
}

func (r *countingReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	r.lists++
	if r.failing {
		return errInjected
	}
	return r.Reader.List(ctx, list, opts...)
}

func eventIndexedClient(t *testing.T, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(crudScheme(t)).WithObjects(objects...).
		WithIndex(&corev1.Event{}, "involvedObject.uid", func(o client.Object) []string {
			return []string{string(o.(*corev1.Event).InvolvedObject.UID)}
		}).Build()
}

func TestResourceManager_WorkspaceStartStep(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceName}}
	labeled := func(pod *corev1.Pod) *corev1.Pod {
		pod.Labels = GenerateLabels(testWorkspaceName)
		return pod
	}
	now := metav1.Now()
	deleting := labeled(unscheduledPod("stale verdict"))
	deleting.Name = "terminating"
	deleting.UID = "pod-uid-old"
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{stepTestFinalizer}
	nominated := podEvent("nominated", "karpenter", "Nominated", stepTestNominated, time.Now())

	tests := []struct {
		name         string
		objects      []client.Object
		withReader   bool
		expectStep   *StartStep
		expectFailed *StartFailure
	}{
		{name: "no pod yet", withReader: true},
		{name: "a pod of another workspace is ignored", withReader: true, objects: []client.Object{unscheduledPod(stepTestSchedMsg)}},
		{name: "failure is reported before any step", withReader: true,
			objects:      []client.Object{labeled(scheduledPod(waitingStatus(kubeletReasonImagePullBackOff, "Back-off pulling image")))},
			expectFailed: &StartFailure{Reason: kubeletReasonImagePullBackOff, Message: "Back-off pulling image"}},
		{name: "waiting for a node with the autoscaler's message", withReader: true,
			objects:    []client.Object{labeled(unscheduledPod(stepTestSchedMsg)), &nominated},
			expectStep: &StartStep{Reason: ReasonWaitingForNode, Message: stepTestNominated}},
		{name: "without an event reader the pod's own verdict is the message",
			objects:    []client.Object{labeled(unscheduledPod(stepTestSchedMsg)), &nominated},
			expectStep: &StartStep{Reason: ReasonWaitingForNode, Message: stepTestSchedMsg}},
		{name: "a pod being deleted is skipped", withReader: true,
			objects:    []client.Object{deleting, labeled(scheduledPod(waitingStatus(kubeletReasonContainerCreating, "")))},
			expectStep: &StartStep{Reason: ReasonStartingContainer, Message: kubeletReasonContainerCreating}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := eventIndexedClient(t, tt.objects...)
			rm := newResourceManagerForCRUD(c, crudScheme(t))
			require.NotNil(t, rm)
			if tt.withReader {
				rm.SetEventReader(c)
			}
			step, failure := rm.WorkspaceStartStep(context.Background(), workspace)
			assert.Equal(t, tt.expectStep, step)
			assert.Equal(t, tt.expectFailed, failure)
		})
	}
}

func TestResourceManager_StartEventsAreThrottled(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceName}}
	pod := unscheduledPod(stepTestSchedMsg)
	pod.Labels = GenerateLabels(testWorkspaceName)
	nominated := podEvent("nominated", "karpenter", "Nominated", stepTestNominated, time.Now())
	c := eventIndexedClient(t, pod, &nominated)
	reader := &countingReader{Reader: c}
	rm := newResourceManagerForCRUD(c, crudScheme(t))
	require.NotNil(t, rm)
	rm.SetEventReader(reader)
	clock := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	rm.now = func() time.Time { return clock }
	ctx := context.Background()

	step, _ := rm.WorkspaceStartStep(ctx, workspace)
	require.NotNil(t, step)
	assert.Equal(t, stepTestNominated, step.Message)
	assert.Equal(t, 1, reader.lists)

	// A new event within the interval is not seen: the cached events are reused.
	limits := podEvent("limits", "karpenter", "Failed", "all available instance types exceed limits for nodepool", time.Now().Add(time.Minute))
	require.NoError(t, c.Create(ctx, &limits))
	clock = clock.Add(startEventsInterval / 2)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, stepTestNominated, step.Message)
	assert.Equal(t, 1, reader.lists)

	// Past the interval the events are read again.
	clock = clock.Add(startEventsInterval)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, "all available instance types exceed limits for nodepool", step.Message)
	assert.Equal(t, 2, reader.lists)

	// A failed read keeps the previous events.
	reader.failing = true
	clock = clock.Add(startEventsInterval)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, "all available instance types exceed limits for nodepool", step.Message)
	assert.Equal(t, 3, reader.lists)
	reader.failing = false

	// Forgetting the workspace forces a read.
	rm.ForgetStartEvents(workspace)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, 4, reader.lists)
	assert.NotNil(t, step)

	// A new pod of the same workspace is read at once.
	require.NoError(t, c.Delete(ctx, pod))
	replacement := unscheduledPod(stepTestSchedMsg)
	replacement.Name = "ws-pod-2"
	replacement.UID = "pod-uid-2"
	replacement.Labels = GenerateLabels(testWorkspaceName)
	require.NoError(t, c.Create(ctx, replacement))
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, 5, reader.lists)
	assert.Equal(t, stepTestSchedMsg, step.Message, "the new pod has no events, so the scheduler's verdict is the message")

	// Entries older than the retention are evicted on the next read.
	rm.cachedStartEvents[client.ObjectKey{Namespace: testNamespaceName, Name: stepTestOtherName}] = startEventsEntry{readAt: clock.Add(-2 * startEventsRetention)}
	clock = clock.Add(startEventsInterval)
	_, _ = rm.WorkspaceStartStep(ctx, workspace)
	_, kept := rm.cachedStartEvents[client.ObjectKey{Namespace: testNamespaceName, Name: stepTestOtherName}]
	assert.False(t, kept)
}
