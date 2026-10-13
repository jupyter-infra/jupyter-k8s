/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

const (
	stepTestPodName           = "ws-pod"
	stepTestInitName          = "init"
	stepTestFinalizer         = "test.jupyter.org/keep"
	stepTestOtherName         = "other"
	stepTestPodUID            = types.UID("pod-uid-1")
	stepTestSchedulingMessage = "0/1 nodes are available: 1 Insufficient nvidia.com/gpu."
	stepTestNominated         = "Pod should schedule on: nodeclaim/workspace-gpu-abc12"
	stepTestNotReadyTaint     = "0/2 nodes are available: 1 node(s) had untolerated taint {node.kubernetes.io/not-ready: }."
	stepTestBackOffText       = "Back-off pulling image"
	stepTestMissingImage      = "example.com/missing:1"
	stepTestPulling           = "Pulling image \"jupyter/base-notebook:latest\""
	stepTestLimits            = "Failed to schedule pod, all available instance types exceed limits for nodepool \"gpu\""
	stepTestMount             = "MountVolume.SetUp failed for volume \"config\" : configmap \"settings\" not found"

	eventReasonFailedScheduling    = "FailedScheduling"
	kubeletReasonContainerCreating = "ContainerCreating"
	kubeletReasonErrImageNeverPull = "ErrImageNeverPull"
	kubeletReasonCreateConfigError = "CreateContainerConfigError"
	stepTestCreateErrorText        = "context deadline exceeded"
	stepTestMultiAttach            = "Multi-Attach error for volume \"pvc-1\" Volume is already used by pod(s) old"
	terminatedReasonError          = "Error"
)

func runningStatus() corev1.ContainerStatus {
	return corev1.ContainerStatus{Name: containerNameMain, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}
}

func readySidecarStatus() corev1.ContainerStatus {
	sidecar := runningStatus()
	sidecar.Name = stepTestOtherName
	sidecar.Ready = true
	return sidecar
}

// stepTestClock is the fixed time the start-step tests run at.
var stepTestClock = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

// restartedStatus is a container the kubelet restarted twice, the last time 30 seconds ago, after a run
// of five seconds that exited with exitCode.
func restartedStatus(ready bool, exitCode int32) corev1.ContainerStatus {
	reason := terminatedReasonError
	if exitCode == 0 {
		reason = "Completed"
	}
	finished := stepTestClock.Add(-30 * time.Second)
	return corev1.ContainerStatus{
		Name: containerNameMain, Ready: ready, RestartCount: 2,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			ExitCode: exitCode, Reason: reason, StartedAt: metav1.NewTime(finished.Add(-5 * time.Second)), FinishedAt: metav1.NewTime(finished),
		}},
	}
}

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

// repeatedEvent is an event the recorder aggregated, whose text appeared at firstSeen and was last repeated at lastSeen.
func repeatedEvent(name, component, reason, message string, firstSeen, lastSeen time.Time) corev1.Event {
	event := podEvent(name, component, reason, message, lastSeen)
	event.FirstTimestamp = metav1.NewTime(firstSeen)
	event.Count = 9
	return event
}

// containerEvent is a kubelet event about one container, named in involvedObject.fieldPath.
func containerEvent(name, container, reason, message string, at time.Time) corev1.Event {
	event := podEvent(name, kubeletComponent, reason, message, at)
	event.InvolvedObject.FieldPath = "spec.containers{" + container + "}"
	return event
}

// reportedEvent is an event written through events/v1: the component sits in reportingComponent and source is empty.
func reportedEvent(name, component, reason, message string, at time.Time) corev1.Event {
	event := podEvent(name, component, reason, message, at)
	event.Source = corev1.EventSource{}
	event.ReportingController = component
	return event
}

func TestDefinitiveStartFailure(t *testing.T) {
	now := stepTestClock
	createError := scheduledPod(waitingStatus(kubeletReasonCreateContainerError, stepTestCreateErrorText))
	failedAttempt := func(name string, count int32) corev1.Event {
		event := containerEvent(name, containerNameMain, kubeletEventFailed, "Error: "+stepTestCreateErrorText, now)
		event.Count = count
		return event
	}
	tests := []struct {
		name     string
		pod      *corev1.Pod
		events   []corev1.Event
		expected *StartFailure
	}{
		{name: "no container statuses", pod: scheduledPod()},
		{name: "container creating is not a failure", pod: scheduledPod(waitingStatus(kubeletReasonContainerCreating, ""))},
		{name: "running container", pod: scheduledPod(runningStatus())},
		{name: "a restarted container is not a waiting failure", pod: scheduledPod(restartedStatus(false, 1))},
		{name: "image pull back-off", pod: scheduledPod(waitingStatus(kubeletReasonImagePullBackOff, "Back-off pulling image \"x:1\"")),
			expected: &StartFailure{Reason: kubeletReasonImagePullBackOff, Message: "Back-off pulling image \"x:1\""}},
		{name: "one failed pull attempt is not a failure", pod: scheduledPod(waitingStatus(kubeletReasonErrImagePull, "rpc error: not found"))},
		{name: "invalid image name", pod: scheduledPod(waitingStatus(kubeletReasonInvalidImageName, "couldn't parse image name")),
			expected: &StartFailure{Reason: kubeletReasonInvalidImageName, Message: "couldn't parse image name"}},
		{name: "config error", pod: scheduledPod(waitingStatus(kubeletReasonCreateConfigError, "secret \"s\" not found")),
			expected: &StartFailure{Reason: kubeletReasonCreateConfigError, Message: "secret \"s\" not found"}},
		{name: "image absent under pull policy Never", pod: scheduledPod(waitingStatus(kubeletReasonErrImageNeverPull, "Container image \"x:1\" is not present with pull policy of Never")),
			expected: &StartFailure{Reason: kubeletReasonErrImageNeverPull, Message: "Container image \"x:1\" is not present with pull policy of Never"}},
		{name: "a create error with no failed attempt recorded is not yet a failure", pod: createError},
		{name: "a create error after one failed attempt is not yet a failure", pod: createError,
			events: []corev1.Event{failedAttempt("f1", 1)}},
		{name: "a create error the kubelet failed twice is a failure", pod: createError,
			events:   []corev1.Event{failedAttempt("f1", 2)},
			expected: &StartFailure{Reason: kubeletReasonCreateContainerError, Message: stepTestCreateErrorText}},
		{name: "two failed attempts recorded as separate events count together", pod: createError,
			events:   []corev1.Event{failedAttempt("f1", 1), failedAttempt("f2", 0)},
			expected: &StartFailure{Reason: kubeletReasonCreateContainerError, Message: stepTestCreateErrorText}},
		{name: "a repeat recorded as an events/v1 series counts", pod: createError,
			events: func() []corev1.Event {
				event := failedAttempt("f1", 0)
				event.Series = &corev1.EventSeries{Count: 2}
				return []corev1.Event{event}
			}(),
			expected: &StartFailure{Reason: kubeletReasonCreateContainerError, Message: stepTestCreateErrorText}},
		{name: "failed attempts of another container do not count", pod: createError,
			events: func() []corev1.Event {
				event := failedAttempt("f1", 2)
				event.InvolvedObject.FieldPath = "spec.containers{sidecar}"
				return []corev1.Event{event}
			}()},
		{name: "a failed pull before the create error is not a create attempt", pod: createError,
			events: []corev1.Event{
				containerEvent("pull", containerNameMain, kubeletEventFailed, "Failed to pull image \"x:1\": not found", now),
				containerEvent("pull-start", containerNameMain, kubeletEventFailed, "Error: ErrImagePull", now),
				failedAttempt("f1", 1),
			}},
		{name: "a failed attempt recorded by another component does not count", pod: createError,
			events: func() []corev1.Event {
				event := failedAttempt("f1", 2)
				event.Source.Component = "karpenter"
				return []corev1.Event{event}
			}()},
		{name: "a run error follows the same rule", pod: scheduledPod(waitingStatus(kubeletReasonRunContainerError, "failed to start container")),
			events: []corev1.Event{containerEvent("f1", containerNameMain, kubeletEventFailed, "Error: failed to start container", now)}},
		{name: "a run error the kubelet failed twice is a failure", pod: scheduledPod(waitingStatus(kubeletReasonRunContainerError, "failed to start container")),
			events: func() []corev1.Event {
				event := containerEvent("f1", containerNameMain, kubeletEventFailed, "Error: failed to start container", now)
				event.Count = 2
				return []corev1.Event{event}
			}(),
			expected: &StartFailure{Reason: kubeletReasonRunContainerError, Message: "failed to start container"}},
		{name: "the attempts of an earlier error on the container count towards this one", pod: createError,
			events: func() []corev1.Event {
				configError := containerEvent("config", containerNameMain, kubeletEventFailed, "Error: secret \"s\" not found", now.Add(-time.Hour))
				configError.Count = 50
				return []corev1.Event{configError, failedAttempt("f1", 1)}
			}(),
			expected: &StartFailure{Reason: kubeletReasonCreateContainerError, Message: stepTestCreateErrorText}},
		{name: "attempts whose text differs from the waiting state count", pod: createError,
			events: []corev1.Event{
				containerEvent("f1", containerNameMain, kubeletEventFailed, "Error: failed to create shim task for container 0a1b: deadline exceeded", now),
				containerEvent("f2", containerNameMain, kubeletEventFailed, "Error: failed to create shim task for container 9c8d: deadline exceeded", now),
			},
			expected: &StartFailure{Reason: kubeletReasonCreateContainerError, Message: stepTestCreateErrorText}},
		{name: "a reason the CRD would reject is guarded", pod: scheduledPod(waitingStatus("Err/ImagePull", "not found")),
			expected: &StartFailure{Reason: ReasonContainerStartFailed, Message: "Err/ImagePull: not found"}},
		{name: "pod initializing is not a failure", pod: scheduledPod(waitingStatus("PodInitializing", ""))},
		{name: "crash loop without a recorded exit gets a message that does not carry the back-off delay",
			pod:      scheduledPod(waitingStatus(kubeletReasonCrashLoopBackOff, "back-off 20s restarting failed container=main")),
			expected: &StartFailure{Reason: kubeletReasonCrashLoopBackOff, Message: "container main is restarting with back-off"}},
		{name: "a crash loop report carries the recorded exit, not the kubelet's back-off text",
			pod: func() *corev1.Pod {
				status := restartedStatus(false, 1)
				status.State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
					Reason: kubeletReasonCrashLoopBackOff, Message: "back-off 20s restarting failed container=main",
				}}
				return scheduledPod(status)
			}(),
			expected: &StartFailure{Reason: kubeletReasonCrashLoopBackOff, Message: "container main exited with code 1 (Error), restart count 2"}},
		{name: "init container failure is reported before a creating main container",
			pod: func() *corev1.Pod {
				pod := scheduledPod(waitingStatus(kubeletReasonContainerCreating, ""))
				pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: stepTestInitName, RestartCount: 3,
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason: kubeletReasonCrashLoopBackOff, Message: "back-off 10s restarting failed container",
					}},
					LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 2, Reason: terminatedReasonError,
					}},
				}}
				return pod
			}(),
			expected: &StartFailure{Reason: kubeletReasonCrashLoopBackOff, Message: "container init exited with code 2 (Error), restart count 3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, definitiveStartFailure(tt.pod, tt.events))
		})
	}
}

func TestStartFailureReasonGuard(t *testing.T) {
	tests := []struct {
		name            string
		reason, message string
		expected        *StartFailure
	}{
		{name: "a conforming reason is kept", reason: kubeletReasonRunContainerError, message: "no such file",
			expected: &StartFailure{Reason: kubeletReasonRunContainerError, Message: "no such file"}},
		{name: "a conforming reason without a message is its own message", reason: kubeletReasonErrImageNeverPull,
			expected: &StartFailure{Reason: kubeletReasonErrImageNeverPull, Message: kubeletReasonErrImageNeverPull}},
		{name: "a reason outside the CRD pattern becomes ContainerStartFailed", reason: "Err/ImagePull", message: "not found",
			expected: &StartFailure{Reason: ReasonContainerStartFailed, Message: "Err/ImagePull: not found"}},
		{name: "a reason outside the CRD pattern without a message", reason: "Err pull",
			expected: &StartFailure{Reason: ReasonContainerStartFailed, Message: "Err pull"}},
		{name: "a reason over the CRD length becomes ContainerStartFailed",
			reason: "Err" + strings.Repeat("x", maxConditionReasonBytes), message: "boom",
			expected: &StartFailure{Reason: ReasonContainerStartFailed, Message: "Err" + strings.Repeat("x", maxConditionReasonBytes) + ": boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, startFailure(tt.reason, tt.message))
		})
	}
}

func TestReportedStartFailure(t *testing.T) {
	conditions := func(degradedReason, progressingReason string) []metav1.Condition {
		return []metav1.Condition{
			{Type: ConditionTypeDegraded, Status: metav1.ConditionTrue, Reason: degradedReason, Message: "m"},
			{Type: ConditionTypeProgressing, Status: metav1.ConditionFalse, Reason: progressingReason, Message: "m"},
		}
	}
	tests := []struct {
		name       string
		conditions []metav1.Condition
		expected   *StartFailure
	}{
		{name: "no conditions"},
		{name: "a kubelet reason on Degraded and Progressing",
			conditions: conditions(kubeletReasonImagePullBackOff, kubeletReasonImagePullBackOff),
			expected:   &StartFailure{Reason: kubeletReasonImagePullBackOff, Message: "m"}},
		{name: "a guarded reason on Degraded and Progressing",
			conditions: conditions(ReasonContainerStartFailed, ReasonContainerStartFailed),
			expected:   &StartFailure{Reason: ReasonContainerStartFailed, Message: "m"}},
		{name: "an operator error on Degraded alone", conditions: conditions(ReasonDeploymentError, ReasonStartingContainer)},
		{name: "a stall", conditions: conditions(ReasonComputeStalled, ReasonComputeStalled)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := &workspacev1alpha1.Workspace{}
			workspace.Status.Conditions = tt.conditions
			assert.Equal(t, tt.expected, reportedStartFailure(workspace))
		})
	}
}

func TestIsDefinitiveStartFailureReason(t *testing.T) {
	for reason, expected := range map[string]bool{
		kubeletReasonInvalidImageName: true, kubeletReasonErrImageNeverPull: true,
		kubeletReasonImagePullBackOff: true, kubeletReasonCrashLoopBackOff: true, kubeletReasonRunContainerError: true,
		kubeletReasonCreateConfigError: true,
		kubeletReasonErrImagePull:      false, kubeletReasonContainerCreating: false, "PodInitializing": false,
		ReasonDeploymentError: false, ReasonServiceError: false, ReasonComputeStalled: false,
	} {
		assert.Equal(t, expected, isDefinitiveStartFailureReason(reason), reason)
	}
}

func TestPodStartStep(t *testing.T) {
	base := stepTestClock
	tests := []struct {
		name     string
		pod      *corev1.Pod
		events   []corev1.Event
		expected StartStep
	}{
		{
			name:     "unscheduled pod without events carries the scheduler's verdict",
			pod:      unscheduledPod(stepTestSchedulingMessage),
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestSchedulingMessage},
		},
		{
			name:     "unscheduled pod without verdict or events",
			pod:      unscheduledPod(""),
			expected: StartStep{Reason: ReasonWaitingForNode, Message: waitingForNodeMessage},
		},
		{
			name: "a failed pull attempt is a step named after the error, not after its two Failed events",
			pod:  scheduledPod(waitingStatus(kubeletReasonErrImagePull, "rpc error: not found")),
			events: []corev1.Event{
				containerEvent("main.1", containerNameMain, kubeletEventFailed, "Failed to pull image \"x:1\": rpc error: not found", base),
				containerEvent("main.2", containerNameMain, kubeletEventFailed, "Error: ErrImagePull", base),
			},
			expected: StartStep{Reason: ReasonStartingContainer, Message: kubeletReasonErrImagePull + ": rpc error: not found"},
		},
		{
			name:     "a create error the kubelet is retrying is a step named after the error, not its Failed event",
			pod:      scheduledPod(waitingStatus(kubeletReasonCreateContainerError, stepTestCreateErrorText)),
			events:   []corev1.Event{containerEvent("failed", containerNameMain, kubeletEventFailed, "Error: "+stepTestCreateErrorText, base)},
			expected: StartStep{Reason: ReasonStartingContainer, Message: kubeletReasonCreateContainerError + ": " + stepTestCreateErrorText},
		},
		{
			name: "a verdict the scheduler keeps repeating counts from its first appearance, so the later nomination wins",
			pod:  unscheduledPod(stepTestSchedulingMessage),
			events: []corev1.Event{
				repeatedEvent("sched", "default-scheduler", eventReasonFailedScheduling, stepTestSchedulingMessage,
					base.Add(-10*time.Second), base.Add(30*time.Second)),
				podEvent("nominated", "karpenter", "Nominated", stepTestNominated, base),
			},
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestNominated},
		},
		{
			name: "a repeat reported through reportingComponent with no source is a repeat too",
			pod:  unscheduledPod(stepTestSchedulingMessage),
			events: []corev1.Event{
				repeatedEvent("sched", "default-scheduler", eventReasonFailedScheduling, stepTestSchedulingMessage,
					base.Add(-10*time.Second), base.Add(30*time.Second)),
				reportedEvent("nominated", "karpenter", "Nominated", stepTestNominated, base),
			},
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestNominated},
		},
		{
			name: "the autoscaler's later FailedScheduling outranks the nomination and the scheduler's repeats",
			pod:  unscheduledPod(stepTestSchedulingMessage),
			events: []corev1.Event{
				podEvent("nominated", "karpenter", "Nominated", stepTestNominated, base),
				podEvent("limits", "karpenter", eventReasonFailedScheduling, stepTestLimits, base.Add(2*time.Minute)),
				repeatedEvent("sched", "default-scheduler", eventReasonFailedScheduling, stepTestSchedulingMessage,
					base.Add(-10*time.Second), base.Add(3*time.Minute)),
			},
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestLimits},
		},
		{
			name: "a scheduler verdict that changed after the nomination wins",
			pod:  unscheduledPod(stepTestSchedulingMessage),
			events: []corev1.Event{
				podEvent("nominated", "karpenter", "Nominated", stepTestNominated, base),
				podEvent("sched", "default-scheduler", eventReasonFailedScheduling, stepTestNotReadyTaint, base.Add(45*time.Second)),
			},
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestNotReadyTaint},
		},
		{
			name: "the scheduler's event alone is the message",
			pod:  unscheduledPod(stepTestSchedulingMessage),
			events: []corev1.Event{
				podEvent("sched", "default-scheduler", eventReasonFailedScheduling, stepTestSchedulingMessage, base),
			},
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestSchedulingMessage},
		},
		{
			name:     "scheduled pod pulling its image",
			pod:      scheduledPod(waitingStatus(kubeletReasonContainerCreating, "")),
			events:   []corev1.Event{podEvent("pulling", kubeletComponent, kubeletEventPulling, stepTestPulling, base)},
			expected: StartStep{Reason: ReasonPullingImage, Message: stepTestPulling},
		},
		{
			name: "a ready sidecar's newer Started event does not hide the main container's pull",
			pod:  scheduledPod(waitingStatus(kubeletReasonContainerCreating, ""), readySidecarStatus()),
			events: []corev1.Event{
				containerEvent("pulling", containerNameMain, kubeletEventPulling, stepTestPulling, base),
				containerEvent("started", stepTestOtherName, "Started", "Started container "+stepTestOtherName, base.Add(10*time.Second)),
			},
			expected: StartStep{Reason: ReasonPullingImage, Message: stepTestPulling},
		},
		{
			name: "an event about a container that runs but is not ready counts",
			pod:  scheduledPod(runningStatus()),
			events: []corev1.Event{
				containerEvent("unhealthy", containerNameMain, "Unhealthy", "Readiness probe failed: connection refused", base),
			},
			expected: StartStep{Reason: ReasonStartingContainer, Message: "Readiness probe failed: connection refused"},
		},
		{
			name:     "an event without a message does not count",
			pod:      unscheduledPod(stepTestSchedulingMessage),
			events:   []corev1.Event{podEvent("nominated", "karpenter", "Nominated", "", base.Add(time.Minute))},
			expected: StartStep{Reason: ReasonWaitingForNode, Message: stepTestSchedulingMessage},
		},
		{
			name: "a later kubelet event ends the pull step",
			pod:  scheduledPod(waitingStatus(kubeletReasonContainerCreating, "")),
			events: []corev1.Event{
				podEvent("pulling", kubeletComponent, kubeletEventPulling, stepTestPulling, base),
				podEvent("pulled", kubeletComponent, "Pulled", "Successfully pulled image in 42s", base.Add(42*time.Second)),
			},
			expected: StartStep{Reason: ReasonStartingContainer, Message: "Successfully pulled image in 42s"},
		},
		{
			name: "a volume attach failure reported about the pod by another controller is the message",
			pod:  scheduledPod(waitingStatus(kubeletReasonContainerCreating, "")),
			events: []corev1.Event{
				podEvent("scheduled", "default-scheduler", "Scheduled", "Successfully assigned default/pod to node-a", base),
				podEvent("attach", "attachdetach-controller", "FailedAttachVolume", stepTestMultiAttach, base.Add(time.Second)),
			},
			expected: StartStep{Reason: ReasonStartingContainer, Message: stepTestMultiAttach},
		},
		{
			name:     "a kubelet warning while the container is creating is the message",
			pod:      scheduledPod(waitingStatus(kubeletReasonContainerCreating, "")),
			events:   []corev1.Event{podEvent("mount", kubeletComponent, "FailedMount", stepTestMount, base)},
			expected: StartStep{Reason: ReasonStartingContainer, Message: stepTestMount},
		},
		{
			name:     "scheduled pod with a waiting container and no events",
			pod:      scheduledPod(waitingStatus("PodInitializing", "")),
			expected: StartStep{Reason: ReasonStartingContainer, Message: "PodInitializing"},
		},
		{
			name:     "a failed pull attempt is a step with the kubelet's reason and message",
			pod:      scheduledPod(waitingStatus(kubeletReasonErrImagePull, "rpc error: not found")),
			expected: StartStep{Reason: ReasonStartingContainer, Message: "ErrImagePull: rpc error: not found"},
		},
		{
			name:     "running but not ready container reports the kubelet's newest message",
			pod:      scheduledPod(runningStatus()),
			events:   []corev1.Event{podEvent("unhealthy", kubeletComponent, "Unhealthy", "Readiness probe failed: connection refused", base)},
			expected: StartStep{Reason: ReasonStartingContainer, Message: "Readiness probe failed: connection refused"},
		},
		{
			name:     "running but not ready container without events",
			pod:      scheduledPod(runningStatus()),
			expected: StartStep{Reason: ReasonStartingContainer, Message: startingContainerMessage},
		},
		{
			name: "same-second kubelet events resolve by name, which the recorder derives from the record time, not by list order",
			pod:  scheduledPod(runningStatus()),
			events: []corev1.Event{
				podEvent("main.18c2a1b2f3e4d5c7", kubeletComponent, "Started", "Started container main", base),
				podEvent("main.18c2a1b2f3e4d5c6", kubeletComponent, kubeletEventPulling, stepTestPulling, base),
			},
			expected: StartStep{Reason: ReasonStartingContainer, Message: "Started container main"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, podStartStep(tt.pod, tt.events))
		})
	}
}

func TestEventFirstSeen(t *testing.T) {
	base := stepTestClock
	repeated := corev1.Event{Count: 9, FirstTimestamp: metav1.NewTime(base), LastTimestamp: metav1.NewTime(base.Add(time.Minute))}
	assert.Equal(t, base, eventFirstSeen(&repeated))
	// events.k8s.io style: eventTime and series, no first or last timestamp
	series := corev1.Event{EventTime: metav1.NewMicroTime(base), Series: &corev1.EventSeries{LastObservedTime: metav1.NewMicroTime(base.Add(2 * time.Minute))}}
	assert.Equal(t, base, eventFirstSeen(&series))
	single := corev1.Event{LastTimestamp: metav1.NewTime(base.Add(time.Minute))}
	assert.Equal(t, base.Add(time.Minute), eventFirstSeen(&single))
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
		WithIndex(&corev1.Event{}, eventInvolvedObjectUIDField, EventInvolvedObjectUID).Build()
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
	evicted := labeled(scheduledPod(corev1.ContainerStatus{Name: containerNameMain, State: corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{ExitCode: 137, Reason: "Evicted"},
	}}))
	evicted.Name = "evicted"
	evicted.UID = "pod-uid-evicted"
	evicted.Status.Phase = corev1.PodFailed
	evicted.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Hour))
	older := labeled(unscheduledPod("old verdict"))
	older.Name = "older"
	older.UID = "pod-uid-older"
	older.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Minute))
	newer := labeled(unscheduledPod(stepTestSchedulingMessage))
	newer.CreationTimestamp = metav1.NewTime(time.Now())
	sameSecond := metav1.NewTime(time.Now().Truncate(time.Second))
	twinA := labeled(unscheduledPod("verdict of pod a"))
	twinA.Name, twinA.UID, twinA.CreationTimestamp = "twin-a", "pod-uid-twin-a", sameSecond
	twinB := labeled(unscheduledPod("verdict of pod b"))
	twinB.Name, twinB.UID, twinB.CreationTimestamp = "twin-b", "pod-uid-twin-b", sameSecond
	reportedBackOff := &StartFailure{Reason: kubeletReasonImagePullBackOff, Message: stepTestBackOffText}
	failedOnce := containerEvent("failed-once", containerNameMain, kubeletEventFailed, "Error: "+stepTestCreateErrorText, time.Now())
	failedOnce.Count = 1
	failedTwice := containerEvent("failed-twice", containerNameMain, kubeletEventFailed, "Error: "+stepTestCreateErrorText, time.Now())
	failedTwice.Count = 2

	tests := []struct {
		name         string
		objects      []client.Object
		withReader   bool
		listErr      error
		reported     *StartFailure
		reportedStep *StartStep
		expectStep   *StartStep
		expectFailed *StartFailure
	}{
		{name: "no pod yet", withReader: true},
		{name: "a failed pod list reports nothing", withReader: true, listErr: errInjected,
			objects: []client.Object{labeled(unscheduledPod(stepTestSchedulingMessage))}},
		{name: "a failed pod list keeps the reported failure", withReader: true, listErr: errInjected,
			objects: []client.Object{labeled(unscheduledPod(stepTestSchedulingMessage))}, reported: reportedBackOff, expectFailed: reportedBackOff},
		{name: "a failed pod list keeps the reported step", withReader: true, listErr: errInjected,
			objects:      []client.Object{labeled(unscheduledPod(stepTestSchedulingMessage))},
			reportedStep: &StartStep{Reason: ReasonPullingImage, Message: stepTestPulling},
			expectStep:   &StartStep{Reason: ReasonPullingImage, Message: stepTestPulling}},
		{name: "pods created in the same second resolve by name", withReader: true,
			objects:    []client.Object{twinA, twinB},
			expectStep: &StartStep{Reason: ReasonWaitingForNode, Message: "verdict of pod b"}},
		{name: "an evicted pod is skipped for its replacement", withReader: true,
			objects:    []client.Object{evicted, labeled(scheduledPod(waitingStatus(kubeletReasonContainerCreating, "")))},
			expectStep: &StartStep{Reason: ReasonStartingContainer, Message: kubeletReasonContainerCreating}},
		{name: "only an evicted pod is no starting pod", withReader: true, objects: []client.Object{evicted}},
		{name: "the newest live pod decides", withReader: true,
			objects:    []client.Object{older, newer},
			expectStep: &StartStep{Reason: ReasonWaitingForNode, Message: stepTestSchedulingMessage}},
		{name: "a pod of another workspace is ignored", withReader: true, objects: []client.Object{unscheduledPod(stepTestSchedulingMessage)}},
		{name: "failure is reported before any step", withReader: true,
			objects:      []client.Object{labeled(scheduledPod(waitingStatus(kubeletReasonImagePullBackOff, stepTestBackOffText)))},
			expectFailed: &StartFailure{Reason: kubeletReasonImagePullBackOff, Message: stepTestBackOffText}},
		{name: "a create error after one failed attempt is a step", withReader: true,
			objects:    []client.Object{labeled(scheduledPod(waitingStatus(kubeletReasonCreateContainerError, stepTestCreateErrorText))), &failedOnce},
			expectStep: &StartStep{Reason: ReasonStartingContainer, Message: kubeletReasonCreateContainerError + ": " + stepTestCreateErrorText}},
		{name: "a create error the kubelet failed twice is a failure", withReader: true,
			objects:      []client.Object{labeled(scheduledPod(waitingStatus(kubeletReasonCreateContainerError, stepTestCreateErrorText))), &failedTwice},
			expectFailed: &StartFailure{Reason: kubeletReasonCreateContainerError, Message: stepTestCreateErrorText}},
		{name: "waiting for a node with the autoscaler's message", withReader: true,
			objects:    []client.Object{labeled(unscheduledPod(stepTestSchedulingMessage)), &nominated},
			expectStep: &StartStep{Reason: ReasonWaitingForNode, Message: stepTestNominated}},
		{name: "without an event reader the pod's own verdict is the message",
			objects:    []client.Object{labeled(unscheduledPod(stepTestSchedulingMessage)), &nominated},
			expectStep: &StartStep{Reason: ReasonWaitingForNode, Message: stepTestSchedulingMessage}},
		{name: "a pod being deleted is skipped", withReader: true,
			objects:    []client.Object{deleting, labeled(scheduledPod(waitingStatus(kubeletReasonContainerCreating, "")))},
			expectStep: &StartStep{Reason: ReasonStartingContainer, Message: kubeletReasonContainerCreating}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := eventIndexedClient(t, tt.objects...)
			if tt.listErr != nil {
				c = &MockClient{Client: c, listFunc: func(context.Context, client.ObjectList, ...client.ListOption) error {
					return tt.listErr
				}}
			}
			rm := newResourceManagerForCRUD(c, crudScheme(t))
			require.NotNil(t, rm)
			if tt.withReader {
				rm.SetEventReader(c)
			}
			ws := workspace.DeepCopy()
			if tt.reported != nil {
				ws.Status.Conditions = []metav1.Condition{
					{Type: ConditionTypeDegraded, Status: metav1.ConditionTrue, Reason: tt.reported.Reason, Message: tt.reported.Message},
					{Type: ConditionTypeProgressing, Status: metav1.ConditionFalse, Reason: tt.reported.Reason, Message: tt.reported.Message},
				}
			}
			if tt.reportedStep != nil {
				ws.Status.Conditions = []metav1.Condition{
					{Type: ConditionTypeProgressing, Status: metav1.ConditionTrue, Reason: tt.reportedStep.Reason, Message: tt.reportedStep.Message},
				}
			}
			step, failure := rm.WorkspaceStartStep(context.Background(), ws)
			assert.Equal(t, tt.expectStep, step)
			assert.Equal(t, tt.expectFailed, failure)
		})
	}
}

func TestCrashLoopingContainer(t *testing.T) {
	running := corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}
	exitAfter := func(run time.Duration, at time.Time) corev1.ContainerState {
		return corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			ExitCode: 1, Reason: terminatedReasonError, StartedAt: metav1.NewTime(at.Add(-run)), FinishedAt: metav1.NewTime(at),
		}}
	}
	exit := func(at time.Time) corev1.ContainerState { return exitAfter(5*time.Second, at) }
	recent := stepTestClock.Add(-30 * time.Second)
	old := stepTestClock.Add(-time.Hour)
	tests := []struct {
		name      string
		status    corev1.ContainerStatus
		crashLoop bool
	}{
		{name: "first run", status: corev1.ContainerStatus{Name: containerNameMain, State: running}},
		{name: "one exit and running again",
			status: corev1.ContainerStatus{Name: containerNameMain, RestartCount: 1, State: running, LastTerminationState: exit(recent)}},
		{name: "one exit and terminated again", crashLoop: true,
			status: corev1.ContainerStatus{Name: containerNameMain, RestartCount: 1, State: exit(recent), LastTerminationState: exit(old)}},
		{name: "two restarts and running", crashLoop: true,
			status: corev1.ContainerStatus{Name: containerNameMain, RestartCount: 2, State: running, LastTerminationState: exit(recent)}},
		{name: "two restarts after a run of an hour",
			status: corev1.ContainerStatus{Name: containerNameMain, RestartCount: 2, State: running, LastTerminationState: exitAfter(time.Hour, recent)}},
		{name: "two restarts and an exit an hour ago",
			status: corev1.ContainerStatus{Name: containerNameMain, RestartCount: 2, State: running, LastTerminationState: exit(old)}},
		{name: "ready after two restarts",
			status: corev1.ContainerStatus{Name: containerNameMain, Ready: true, RestartCount: 2, State: running, LastTerminationState: exit(recent)}},
		{name: "a start that failed has no start time and counts as a short run", crashLoop: true,
			status: corev1.ContainerStatus{Name: containerNameMain, RestartCount: 2, State: running, LastTerminationState: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{ExitCode: 128, Reason: "StartError",
					StartedAt: metav1.NewTime(time.Unix(0, 0)), FinishedAt: metav1.NewTime(recent)}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crashLooping := crashLoopingContainer(scheduledPod(tt.status), stepTestClock)
			if tt.crashLoop {
				assert.Equal(t, &tt.status, crashLooping)
			} else {
				assert.Nil(t, crashLooping)
			}
		})
	}
}

func TestResourceManager_WorkspaceStartStep_CrashLoop(t *testing.T) {
	const recordedExit = "container main exited with code 1 (Error), restart count 2"
	longRun := restartedStatus(false, 137)
	longRun.LastTerminationState.Terminated.Reason = "OOMKilled"
	longRun.LastTerminationState.Terminated.StartedAt = metav1.NewTime(stepTestClock.Add(-time.Hour))
	terminatedAgain := restartedStatus(false, 1)
	terminatedAgain.RestartCount = 1
	terminatedAgain.State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
		ExitCode: 1, Reason: terminatedReasonError,
		StartedAt: metav1.NewTime(stepTestClock.Add(-15 * time.Second)), FinishedAt: metav1.NewTime(stepTestClock.Add(-10 * time.Second)),
	}}
	onceRunning := restartedStatus(false, 1)
	onceRunning.RestartCount = 1
	backingOff := restartedStatus(false, 1)
	backingOff.State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
		Reason: kubeletReasonCrashLoopBackOff, Message: "back-off 20s restarting failed container=main",
	}}
	tests := []struct {
		name         string
		status       corev1.ContainerStatus
		expectFailed *StartFailure
	}{
		{name: "a restarted container between restarts is a crash loop with the recorded exit",
			status:       restartedStatus(false, 1),
			expectFailed: &StartFailure{Reason: kubeletReasonCrashLoopBackOff, Message: recordedExit}},
		{name: "the kubelet's back-off report carries the same recorded exit",
			status:       backingOff,
			expectFailed: &StartFailure{Reason: kubeletReasonCrashLoopBackOff, Message: recordedExit}},
		{name: "a container that completes at once and loops is a crash loop too",
			status: restartedStatus(false, 0),
			expectFailed: &StartFailure{Reason: kubeletReasonCrashLoopBackOff,
				Message: "container main exited with code 0 (Completed), restart count 2"}},
		{name: "a container terminated again after one restart is a crash loop",
			status: terminatedAgain,
			expectFailed: &StartFailure{Reason: kubeletReasonCrashLoopBackOff,
				Message: "container main exited with code 1 (Error), restart count 1"}},
		{name: "a container running again after one exit is a step, not a failure", status: onceRunning},
		{name: "a container that ran an hour before it exited is a step, not a failure", status: longRun},
		{name: "a container on its first run is a step, not a failure", status: runningStatus()},
		{name: "a ready container is a step, not a failure", status: restartedStatus(true, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceName}}
			pod := scheduledPod(tt.status)
			pod.Labels = GenerateLabels(testWorkspaceName)
			c := eventIndexedClient(t, pod)
			rm := newResourceManagerForCRUD(c, crudScheme(t))
			require.NotNil(t, rm)
			rm.SetEventReader(c)
			rm.now = func() time.Time { return stepTestClock }
			step, failure := rm.WorkspaceStartStep(context.Background(), workspace)
			if tt.expectFailed != nil {
				assert.Nil(t, step)
				assert.Equal(t, tt.expectFailed, failure)
			} else {
				assert.Nil(t, failure)
				assert.Equal(t, &StartStep{Reason: ReasonStartingContainer, Message: startingContainerMessage}, step)
			}
		})
	}
}

func TestResourceManager_WorkspaceStartStep_HoldsReportedBackOffWhileRetrying(t *testing.T) {
	backOff := stepTestBackOffText + " \"" + stepTestMissingImage + "\""
	sidecarRetrying := waitingStatus(kubeletReasonErrImagePull, "rpc error: not found")
	sidecarRetrying.Name = stepTestOtherName
	tests := []struct {
		name     string
		image    string
		reported string
		statuses []corev1.ContainerStatus
		held     bool
	}{
		{name: "the reported container retrying its pull holds the back-off",
			image: stepTestMissingImage, reported: backOff,
			statuses: []corev1.ContainerStatus{waitingStatus(kubeletReasonErrImagePull, "rpc error: not found")}, held: true},
		{name: "a sidecar retrying its own image does not hold another container's resolved back-off",
			image: stepTestMissingImage, reported: backOff,
			statuses: []corev1.ContainerStatus{runningStatus(), sidecarRetrying}, held: false},
		{name: "an untagged image is quoted with the default tag by kubelets from 1.33 on",
			image: "example.com/missing", reported: "Back-off pulling image \"example.com/missing:latest\"",
			statuses: []corev1.ContainerStatus{waitingStatus(kubeletReasonErrImagePull, "rpc error: not found")}, held: true},
		{name: "an untagged image is quoted as written by older kubelets",
			image: "example.com/missing", reported: "Back-off pulling image \"example.com/missing\"",
			statuses: []corev1.ContainerStatus{waitingStatus(kubeletReasonErrImagePull, "rpc error: not found")}, held: true},
		{name: "a runtime reporting RegistryUnavailable for the attempt holds the back-off",
			image: stepTestMissingImage, reported: backOff,
			statuses: []corev1.ContainerStatus{waitingStatus(kubeletReasonRegistryUnavailable, "registry timed out")}, held: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceName}}
			workspace.Status.Conditions = []metav1.Condition{
				{Type: ConditionTypeDegraded, Status: metav1.ConditionTrue, Reason: kubeletReasonImagePullBackOff, Message: tt.reported},
				{Type: ConditionTypeProgressing, Status: metav1.ConditionFalse, Reason: kubeletReasonImagePullBackOff, Message: tt.reported},
			}
			pod := scheduledPod(tt.statuses...)
			pod.Spec.Containers = []corev1.Container{
				{Name: containerNameMain, Image: tt.image}, {Name: stepTestOtherName, Image: "example.com/sidecar:1"},
			}
			pod.Labels = GenerateLabels(testWorkspaceName)
			c := eventIndexedClient(t, pod)
			rm := newResourceManagerForCRUD(c, crudScheme(t))
			require.NotNil(t, rm)
			rm.SetEventReader(c)

			step, failure := rm.WorkspaceStartStep(context.Background(), workspace)
			if tt.held {
				assert.Nil(t, step)
				assert.Equal(t, &StartFailure{Reason: kubeletReasonImagePullBackOff, Message: tt.reported}, failure)
				return
			}
			assert.Nil(t, failure)
			assert.Equal(t, ReasonStartingContainer, step.Reason)
		})
	}
}

// blockingReader is an event reader whose List returns only when the context ends, like a stalled API server.
type blockingReader struct{ client.Reader }

func (r *blockingReader) List(ctx context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestResourceManager_StartEventsReadIsBounded(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceName}}
	pod := unscheduledPod(stepTestSchedulingMessage)
	pod.Labels = GenerateLabels(testWorkspaceName)
	nominated := podEvent("nominated", "karpenter", "Nominated", stepTestNominated, time.Now())
	c := eventIndexedClient(t, pod, &nominated)
	rm := newResourceManagerForCRUD(c, crudScheme(t))
	require.NotNil(t, rm)
	rm.SetEventReader(c)
	rm.eventReadLimiter = rate.NewLimiter(rate.Inf, 0)
	rm.eventReadTimeout = 50 * time.Millisecond
	clock := stepTestClock
	rm.now = func() time.Time { return clock }
	ctx := context.Background()

	step, _ := rm.WorkspaceStartStep(ctx, workspace)
	require.NotNil(t, step)
	assert.Equal(t, stepTestNominated, step.Message)

	// The API server stalls: the read ends at the timeout and the cached events stand.
	rm.SetEventReader(&blockingReader{Reader: c})
	clock = clock.Add(2 * startEventsInterval)
	started := time.Now()
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Less(t, time.Since(started), time.Second)
	require.NotNil(t, step)
	assert.Equal(t, stepTestNominated, step.Message)
}

func TestResourceManager_StartEventsShareARateLimit(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceName}}
	pod := unscheduledPod(stepTestSchedulingMessage)
	pod.Labels = GenerateLabels(testWorkspaceName)
	nominated := podEvent("nominated", "karpenter", "Nominated", stepTestNominated, time.Now())
	c := eventIndexedClient(t, pod, &nominated)
	reader := &countingReader{Reader: c}
	rm := newResourceManagerForCRUD(c, crudScheme(t))
	require.NotNil(t, rm)
	rm.SetEventReader(reader)
	// one read in the bucket and no refill
	rm.eventReadLimiter = rate.NewLimiter(0, 1)
	clock := stepTestClock
	rm.now = func() time.Time { return clock }
	ctx := context.Background()

	step, _ := rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, stepTestNominated, step.Message)
	assert.Equal(t, 1, reader.lists)

	// Past the interval, a denied read reuses the cached events and leaves the interval open.
	clock = clock.Add(2 * startEventsInterval)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, stepTestNominated, step.Message)
	assert.Equal(t, 1, reader.lists)

	rm.eventReadLimiter = rate.NewLimiter(rate.Inf, 0)
	_, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, 2, reader.lists)

	// Reads from the manager cache cost no API request, so neither the limiter nor the interval applies.
	rm.SetEventCacheReader(reader)
	rm.eventReadLimiter = rate.NewLimiter(0, 0)
	_, _ = rm.WorkspaceStartStep(ctx, workspace)
	_, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, 4, reader.lists)
}

func TestResourceManager_StartEventsAreThrottled(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceName}}
	pod := unscheduledPod(stepTestSchedulingMessage)
	pod.Labels = GenerateLabels(testWorkspaceName)
	nominated := podEvent("nominated", "karpenter", "Nominated", stepTestNominated, time.Now())
	c := eventIndexedClient(t, pod, &nominated)
	reader := &countingReader{Reader: c}
	rm := newResourceManagerForCRUD(c, crudScheme(t))
	require.NotNil(t, rm)
	rm.SetEventReader(reader)
	rm.eventReadLimiter = rate.NewLimiter(rate.Inf, 0)
	clock := stepTestClock
	rm.now = func() time.Time { return clock }
	ctx := context.Background()

	step, _ := rm.WorkspaceStartStep(ctx, workspace)
	require.NotNil(t, step)
	assert.Equal(t, stepTestNominated, step.Message)
	assert.Equal(t, 1, reader.lists)

	// A new event within the interval is not seen.
	limits := podEvent("limits", "karpenter", eventReasonFailedScheduling, stepTestLimits, time.Now().Add(time.Minute))
	require.NoError(t, c.Create(ctx, &limits))
	clock = clock.Add(startEventsInterval / 2)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, stepTestNominated, step.Message)
	assert.Equal(t, 1, reader.lists)

	// Past the interval the events are read again.
	clock = clock.Add(startEventsInterval)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, stepTestLimits, step.Message)
	assert.Equal(t, 2, reader.lists)

	// A failed read keeps the previous events and holds the interval.
	reader.failing = true
	clock = clock.Add(startEventsInterval)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, stepTestLimits, step.Message)
	assert.Equal(t, 3, reader.lists)
	clock = clock.Add(startEventsInterval / 2)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, stepTestLimits, step.Message)
	assert.Equal(t, 3, reader.lists)
	reader.failing = false

	// Forgetting the workspace forces a read.
	rm.ForgetStartEvents(workspace)
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, 4, reader.lists)
	assert.NotNil(t, step)

	// A new pod of the same workspace is read at once.
	require.NoError(t, c.Delete(ctx, pod))
	replacement := unscheduledPod(stepTestSchedulingMessage)
	replacement.Name = "ws-pod-2"
	replacement.UID = "pod-uid-2"
	replacement.Labels = GenerateLabels(testWorkspaceName)
	require.NoError(t, c.Create(ctx, replacement))
	step, _ = rm.WorkspaceStartStep(ctx, workspace)
	assert.Equal(t, 5, reader.lists)
	assert.Equal(t, stepTestSchedulingMessage, step.Message, "the new pod has no events, so the scheduler's verdict is the message")

	// Entries older than the retention are evicted on the next read.
	rm.cachedStartEvents[client.ObjectKey{Namespace: testNamespaceName, Name: stepTestOtherName}] = startEventsEntry{readAt: clock.Add(-2 * startEventsRetention)}
	clock = clock.Add(startEventsInterval)
	_, _ = rm.WorkspaceStartStep(ctx, workspace)
	_, kept := rm.cachedStartEvents[client.ObjectKey{Namespace: testNamespaceName, Name: stepTestOtherName}]
	assert.False(t, kept)
}

func TestPrefixReason(t *testing.T) {
	tests := []struct {
		name, reason, message, expected string
	}{
		{name: "reason and message", reason: kubeletReasonImagePullBackOff, message: stepTestBackOffText,
			expected: kubeletReasonImagePullBackOff + ": Back-off pulling image"},
		{name: "no message", reason: kubeletReasonContainerCreating, message: "", expected: kubeletReasonContainerCreating},
		{name: "message that repeats the reason", reason: kubeletReasonCrashLoopBackOff,
			message: kubeletReasonCrashLoopBackOff, expected: kubeletReasonCrashLoopBackOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, prefixReason(tt.reason, tt.message))
		})
	}
}

func TestWaitingContainerMessage(t *testing.T) {
	scheduled := corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}
	waiting := func(name, reason, message string) corev1.ContainerStatus {
		return corev1.ContainerStatus{Name: name, State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: message},
		}}
	}
	running := corev1.ContainerStatus{Name: "workspace", State: corev1.ContainerState{
		Running: &corev1.ContainerStateRunning{},
	}}

	tests := []struct {
		name     string
		status   corev1.PodStatus
		expected string
	}{
		{name: "no status", status: corev1.PodStatus{}},
		{
			name: "scheduled pod reports the waiting container's reason and message",
			status: corev1.PodStatus{
				Conditions:        []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{waiting("workspace", kubeletReasonImagePullBackOff, `Back-off pulling image stepTestMissingImage`)},
			},
			expected: `ImagePullBackOff: Back-off pulling image stepTestMissingImage`,
		},
		{
			name: "waiting container without a message reports the reason alone",
			status: corev1.PodStatus{
				Conditions:        []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{waiting("workspace", kubeletReasonContainerCreating, "")},
			},
			expected: kubeletReasonContainerCreating,
		},
		{
			name: "a waiting init container is reported before the main container",
			status: corev1.PodStatus{
				Conditions:            []corev1.PodCondition{scheduled},
				InitContainerStatuses: []corev1.ContainerStatus{waiting(stepTestInitName, kubeletReasonCrashLoopBackOff, "back-off 5m0s restarting failed container=init")},
				ContainerStatuses:     []corev1.ContainerStatus{waiting("workspace", "PodInitializing", "")},
			},
			expected: "CrashLoopBackOff: back-off 5m0s restarting failed container=init",
		},
		{
			name: "the first waiting container wins when a sidecar is already running",
			status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{
					running,
					waiting("sidecar", kubeletReasonCreateConfigError, `secret "token" not found`),
				},
			},
			expected: `CreateContainerConfigError: secret "token" not found`,
		},
		{
			name: "a waiting state without a reason is skipped",
			status: corev1.PodStatus{
				Conditions:        []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{waiting("workspace", "", "")},
			},
		},
		{
			name: "a running pod reports nothing",
			status: corev1.PodStatus{
				Conditions:        []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{running},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, waitingContainerMessage(&corev1.Pod{Status: tt.status}, nil))
		})
	}
}
