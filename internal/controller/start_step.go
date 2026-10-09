/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

const (
	// startEventsInterval is the shortest interval between two reads of a starting pod's events. The
	// running path reconciles every PollRequeueDelay (200ms); event reads must not follow that loop.
	startEventsInterval = 5 * time.Second
	// startEventsRetention bounds how long a workspace's cached events outlive their last read, so a
	// workspace that stopped mid-start does not keep an entry forever.
	startEventsRetention = 10 * time.Minute
	// startEventsPerSecond and startEventsBurst bound event reads across all workspaces, so a hundred
	// workspaces starting at once do not spend the manager's API client budget (20 requests per second by
	// default) on events; each workspace then refreshes its events less often than startEventsInterval.
	startEventsPerSecond = 4
	startEventsBurst     = 4

	// Vocabulary of core Kubernetes components; k8s.io/kubernetes/pkg/kubelet is not importable.
	kubeletComponent              = "kubelet"
	kubeletEventPulling           = "Pulling"
	kubeletReasonInvalidImageName = "InvalidImageName"
	kubeletReasonCrashLoopBackOff = "CrashLoopBackOff"

	// Fallback messages when neither the pod nor its events carry one.
	waitingForNodeMessage    = "Waiting for a node to run the pod"
	startingContainerMessage = "Starting the container"

	// crashLoopResetAfter is how long a container has to run before the kubelet resets its restart back-off
	// (https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy); an exit older than
	// that no longer marks a crash loop.
	crashLoopResetAfter = 10 * time.Minute

	// recordedExitMarker is in every message crashLoopFailure writes, and in none the kubelet writes.
	recordedExitMarker = " exited with code "

	// maxCopiedMessageBytes bounds a message copied from an event or a container state onto a condition,
	// which holds at most 32768 bytes. The kubelet and the scheduler stay far below it; a core/v1 event
	// created by hand has no length limit.
	maxCopiedMessageBytes = 1024
)

// StartStep is the step a starting workspace pod is in: the Progressing reason and the message the
// component responsible for that step recorded, copied unchanged.
type StartStep struct {
	Reason  string
	Message string
}

// StartFailure is a kubelet waiting reason for a container it cannot start as specified, with the
// kubelet's message.
type StartFailure struct {
	Reason  string
	Message string
}

// isDefinitiveStartFailureReason reports whether reason is one the kubelet uses for a container it cannot
// start as specified: an image it cannot pull, a missing Secret or ConfigMap, a command that keeps failing.
// Argo CD's health check applies the same rule, any reason starting with Err or ending with Error or
// BackOff (ErrImagePull, ImagePullBackOff, ErrImageNeverPull, CreateContainerConfigError, RunContainerError,
// CrashLoopBackOff); InvalidImageName matches neither and is named.
func isDefinitiveStartFailureReason(reason string) bool {
	// the operator's own Degraded reasons end in Error as well
	if reason == ReasonDeploymentError || reason == ReasonServiceError {
		return false
	}
	return reason == kubeletReasonInvalidImageName || strings.HasPrefix(reason, "Err") ||
		strings.HasSuffix(reason, "Error") || strings.HasSuffix(reason, "BackOff")
}

// startEventsEntry is the last read of a starting pod's events for one workspace.
type startEventsEntry struct {
	podUID types.UID
	readAt time.Time
	events []corev1.Event
}

// reportedStartFailure returns the start failure the workspace's conditions already carry, written by
// reconcileFailedStart as Degraded=True and Progressing with the same kubelet reason, or nil. UpdateErrorStatus
// writes Degraded alone, so an operator error such as ComputeError does not count.
func reportedStartFailure(workspace *workspacev1alpha1.Workspace) *StartFailure {
	degraded := FindCondition(&workspace.Status.Conditions, ConditionTypeDegraded)
	progressing := FindCondition(&workspace.Status.Conditions, ConditionTypeProgressing)
	if degraded == nil || progressing == nil || degraded.Status != metav1.ConditionTrue ||
		progressing.Reason != degraded.Reason || !isDefinitiveStartFailureReason(degraded.Reason) {
		return nil
	}
	return &StartFailure{Reason: degraded.Reason, Message: degraded.Message}
}

// crashLoopingContainer returns the first container of the pod, init containers first, that is in a crash
// loop, or nil: a restarted container that is not ready and has exited again, or ran for less than
// crashLoopResetAfter since its last exit. The kubelet backs off any exited container, exit code 0
// included; between the exit and the next CrashLoopBackOff report the container shows as Running or
// Terminated rather than Waiting, and on Kubernetes 1.37 the Waiting report lasts about a second per
// restart where 1.33 held it for the whole back-off.
func crashLoopingContainer(pod *corev1.Pod, now time.Time) *corev1.ContainerStatus {
	statuses := containerStatuses(pod)
	for i := range statuses {
		containerStatus := &statuses[i]
		if containerStatus.Ready || containerStatus.RestartCount == 0 {
			continue
		}
		if containerStatus.State.Terminated != nil {
			return containerStatus
		}
		last := containerStatus.LastTerminationState.Terminated
		if last != nil && now.Sub(last.FinishedAt.Time) < crashLoopResetAfter {
			return containerStatus
		}
	}
	return nil
}

// crashLoopFailure describes a crash-looping container as the CrashLoopBackOff failure the kubelet reports
// between its own back-off messages, from the latest exit the pod status records.
func crashLoopFailure(containerStatus *corev1.ContainerStatus) *StartFailure {
	terminated := containerStatus.State.Terminated
	if terminated == nil {
		terminated = containerStatus.LastTerminationState.Terminated
	}
	message := fmt.Sprintf("container %s%s%d", containerStatus.Name, recordedExitMarker, terminated.ExitCode)
	if terminated.Reason != "" {
		message += " (" + terminated.Reason + ")"
	}
	message += fmt.Sprintf(", restart count %d", containerStatus.RestartCount)
	return &StartFailure{Reason: kubeletReasonCrashLoopBackOff, Message: message}
}

// containerStatuses returns the pod's container statuses, init containers first.
func containerStatuses(pod *corev1.Pod) []corev1.ContainerStatus {
	statuses := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	return append(statuses, pod.Status.ContainerStatuses...)
}

// waitingContainerMessage returns the reason and message of the first waiting container, init
// containers first (e.g. "ImagePullBackOff: Back-off pulling image ..."), or "".
func waitingContainerMessage(pod *corev1.Pod) string {
	for _, containerStatus := range containerStatuses(pod) {
		waiting := containerStatus.State.Waiting
		if waiting == nil || waiting.Reason == "" {
			continue
		}
		if waiting.Message == "" {
			return waiting.Reason
		}
		return waiting.Reason + ": " + waiting.Message
	}
	return ""
}

// definitiveStartFailure returns the first container, init containers first, that the kubelet reports
// as unable to start, or nil.
func definitiveStartFailure(pod *corev1.Pod) *StartFailure {
	for _, containerStatus := range containerStatuses(pod) {
		waiting := containerStatus.State.Waiting
		if waiting == nil || !isDefinitiveStartFailureReason(waiting.Reason) {
			continue
		}
		message := waiting.Message
		if message == "" {
			message = waiting.Reason
		}
		return &StartFailure{Reason: waiting.Reason, Message: message}
	}
	return nil
}

// podScheduled reports whether the pod has been assigned a node.
func podScheduled(pod *corev1.Pod) bool {
	if pod.Spec.NodeName != "" {
		return true
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// podStartStep derives the step from the pod and the events recorded on it. Without a node, the message
// is the newest event from a component other than the kube-scheduler, whose FailedScheduling text is
// already the PodScheduled message (the fallback); autoscalers such as Karpenter record their verdicts
// under their own component. With a node, the kubelet's waiting reason stays ContainerCreating through
// a pull or a failed mount, so its newest event is the message and the waiting reason the fallback.
func podStartStep(pod *corev1.Pod, events []corev1.Event) StartStep {
	if !podScheduled(pod) {
		if event := newestEvent(events, func(event *corev1.Event) bool { return !isSchedulerEvent(event) }); event != nil {
			return StartStep{Reason: ReasonWaitingForNode, Message: event.Message}
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Message != "" {
				return StartStep{Reason: ReasonWaitingForNode, Message: condition.Message}
			}
		}
		return StartStep{Reason: ReasonWaitingForNode, Message: waitingForNodeMessage}
	}
	kubeletEvent := newestEvent(events, isKubeletEvent)
	if kubeletEvent != nil && kubeletEvent.Reason == kubeletEventPulling {
		return StartStep{Reason: ReasonPullingImage, Message: kubeletEvent.Message}
	}
	if kubeletEvent != nil && kubeletEvent.Message != "" {
		return StartStep{Reason: ReasonStartingContainer, Message: kubeletEvent.Message}
	}
	if message := waitingContainerMessage(pod); message != "" {
		return StartStep{Reason: ReasonStartingContainer, Message: message}
	}
	return StartStep{Reason: ReasonStartingContainer, Message: startingContainerMessage}
}

// eventTime returns the time an event was last observed, whichever of its timestamp fields is set.
func eventTime(event *corev1.Event) time.Time {
	observed := event.EventTime.Time
	if event.LastTimestamp.After(observed) {
		observed = event.LastTimestamp.Time
	}
	if event.FirstTimestamp.After(observed) {
		observed = event.FirstTimestamp.Time
	}
	if event.Series != nil && event.Series.LastObservedTime.After(observed) {
		observed = event.Series.LastObservedTime.Time
	}
	return observed
}

func isSchedulerEvent(event *corev1.Event) bool {
	return strings.Contains(event.Source.Component, "scheduler") ||
		strings.Contains(event.ReportingController, "scheduler")
}

func isKubeletEvent(event *corev1.Event) bool {
	return event.Source.Component == kubeletComponent || event.ReportingController == kubeletComponent
}

// newestEvent returns the most recently observed event accepted by keep, or nil. Equal timestamps go to
// the later event in list order: the kubelet records timestamps at second resolution, and the API lists
// events by name, which embeds their creation time.
func newestEvent(events []corev1.Event, keep func(*corev1.Event) bool) *corev1.Event {
	var newest *corev1.Event
	for i := range events {
		event := &events[i]
		if !keep(event) {
			continue
		}
		if newest == nil || !eventTime(event).Before(eventTime(newest)) {
			newest = event
		}
	}
	return newest
}

// SetEventReader sets the reader used for a starting pod's events. The manager's API reader is used so
// that events are fetched for one pod at a time; the manager caches events cluster-wide only when
// workspace pod watching is enabled, which is off by default. Without a reader the step falls back to
// what the pod itself reports.
func (rm *ResourceManager) SetEventReader(reader client.Reader) {
	rm.eventReader = reader
}

// WorkspaceStartStep reports what the workspace's starting pod is doing: a failure the kubelet
// recorded as definitive, if any, otherwise the step it is in with the newest relevant message. Both
// are nil while no pod exists yet. For a crash-looping container the kubelet's back-off message, once
// seen, holds through the restarts so the status does not flap; until then the exit the pod status
// records is reported, with the current restart count.
func (rm *ResourceManager) WorkspaceStartStep(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) (*StartStep, *StartFailure) {
	pod := rm.workspaceStartingPod(ctx, workspace)
	if pod == nil {
		return nil, nil
	}
	if failure := definitiveStartFailure(pod); failure != nil {
		failure.Message = truncateMessage(failure.Message, maxCopiedMessageBytes)
		return nil, failure
	}
	if crashLooping := crashLoopingContainer(pod, rm.now()); crashLooping != nil {
		// the kubelet's own back-off message holds; a recorded exit is refreshed with the current restart count,
		// and an earlier failure of another kind (a pull error before the image was fixed) is replaced
		if reported := reportedStartFailure(workspace); reported != nil &&
			reported.Reason == kubeletReasonCrashLoopBackOff && !strings.Contains(reported.Message, recordedExitMarker) {
			return nil, reported
		}
		return nil, crashLoopFailure(crashLooping)
	}
	step := podStartStep(pod, rm.startEvents(ctx, workspace, pod))
	step.Message = truncateMessage(step.Message, maxCopiedMessageBytes)
	return &step, nil
}

// ForgetStartEvents drops the cached events of a workspace whose start is over.
func (rm *ResourceManager) ForgetStartEvents(workspace *workspacev1alpha1.Workspace) {
	rm.startEventsMu.Lock()
	defer rm.startEventsMu.Unlock()
	delete(rm.cachedStartEvents, client.ObjectKeyFromObject(workspace))
}

// workspaceStartingPod returns the newest live pod of the workspace, or nil. Pods being deleted and pods
// that have terminated are skipped: under the Recreate strategy the previous pod can still be
// terminating while the next one is pending, and an evicted pod stays in phase Failed next to its
// replacement. The cached list has no stable order, so the newest creation time decides.
func (rm *ResourceManager) workspaceStartingPod(ctx context.Context, workspace *workspacev1alpha1.Workspace) *corev1.Pod {
	podList := &corev1.PodList{}
	if err := rm.client.List(ctx, podList,
		client.InNamespace(workspace.Namespace),
		client.MatchingLabels(GenerateLabels(workspace.Name)),
	); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list workspace pods for the start step")
		return nil
	}
	var newest *corev1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if newest == nil || pod.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = pod
		}
	}
	return newest
}

// startEvents returns the events recorded on the pod, read through the event reader at most once per
// startEventsInterval per workspace and at most startEventsPerSecond across workspaces, and reused in
// between. A failed read keeps the previous events and counts as a read, so the interval holds through an
// API outage; a read the shared limiter denies does not, so the next reconcile tries again.
func (rm *ResourceManager) startEvents(ctx context.Context, workspace *workspacev1alpha1.Workspace, pod *corev1.Pod) []corev1.Event {
	if rm.eventReader == nil {
		return nil
	}
	key := client.ObjectKeyFromObject(workspace)
	now := rm.now()

	rm.startEventsMu.Lock()
	cached, found := rm.cachedStartEvents[key]
	rm.startEventsMu.Unlock()
	if found && cached.podUID == pod.UID && now.Sub(cached.readAt) < startEventsInterval {
		return cached.events
	}
	if !rm.eventReadLimiter.Allow() {
		if found && cached.podUID == pod.UID {
			return cached.events
		}
		return nil
	}

	eventList := &corev1.EventList{}
	err := rm.eventReader.List(ctx, eventList,
		client.InNamespace(pod.Namespace),
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("involvedObject.uid", string(pod.UID))},
	)
	events := eventList.Items
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list the events of the starting pod", "pod", pod.Name)
		events = nil
		if found && cached.podUID == pod.UID {
			events = cached.events
		}
	}

	rm.startEventsMu.Lock()
	for cachedKey, cached := range rm.cachedStartEvents {
		if now.Sub(cached.readAt) > startEventsRetention {
			delete(rm.cachedStartEvents, cachedKey)
		}
	}
	rm.cachedStartEvents[key] = startEventsEntry{podUID: pod.UID, readAt: now, events: events}
	rm.startEventsMu.Unlock()
	return events
}
