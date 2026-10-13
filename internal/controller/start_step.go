/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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
	// startEventsRetention bounds how long a workspace's cached events outlive their last read, so entries
	// of workspaces that are no longer reconciled do not accumulate.
	startEventsRetention = 10 * time.Minute
	// startEventsPerSecond and startEventsBurst bound event reads across all workspaces, so a hundred
	// workspaces starting at once do not spend the manager's API client budget (20 requests per second by
	// default) on events; each workspace then refreshes its events less often than startEventsInterval.
	startEventsPerSecond = 4
	startEventsBurst     = 4

	// Vocabulary of core Kubernetes components; k8s.io/kubernetes/pkg/kubelet is not importable.
	kubeletComponent          = "kubelet"
	kubeletEventPulling       = "Pulling"
	kubeletReasonErrImagePull = "ErrImagePull"
	// kubeletReasonRegistryUnavailable and kubeletReasonSignatureValidationFailed are the attempt reasons a
	// runtime following the CRI error convention makes the kubelet report instead of ErrImagePull.
	kubeletReasonRegistryUnavailable       = "RegistryUnavailable"
	kubeletReasonSignatureValidationFailed = "SignatureValidationFailed"
	kubeletReasonImagePullBackOff          = "ImagePullBackOff"
	kubeletReasonInvalidImageName          = "InvalidImageName"
	kubeletReasonCrashLoopBackOff          = "CrashLoopBackOff"

	// Fallback messages when neither the pod nor its events carry one.
	waitingForNodeMessage    = "Waiting for a node to run the pod"
	startingContainerMessage = "Starting the container"

	// crashLoopResetAfter is how long a container has to run before the kubelet resets its restart back-off
	// (https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy).
	crashLoopResetAfter = 10 * time.Minute

	// maxCopiedMessageBytes bounds a message copied from an event, a container state or a Deployment condition
	// onto a workspace condition, which holds at most 32768 bytes, and the Warning event composed from it.
	// The kubelet, the scheduler and the Deployment controller stay far below it; a core/v1 event created by
	// hand has no length limit.
	maxCopiedMessageBytes = 1024

	// maxConditionReasonBytes is the CRD's ceiling for a condition reason, from metav1.Condition.
	maxConditionReasonBytes = 1024
)

// conditionReasonPattern is the CRD's validation of a condition reason, from metav1.Condition.
var conditionReasonPattern = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`)

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

// startFailure builds a StartFailure from a kubelet reason and message, the reason itself when the message
// is empty. A reason the CRD would reject as a condition reason becomes ContainerStartFailed, with the raw
// reason prefixed to the message.
func startFailure(reason, message string) *StartFailure {
	if len(reason) > maxConditionReasonBytes || !conditionReasonPattern.MatchString(reason) {
		return &StartFailure{Reason: ReasonContainerStartFailed, Message: prefixReason(reason, message)}
	}
	if message == "" {
		message = reason
	}
	return &StartFailure{Reason: reason, Message: message}
}

// prefixReason joins a reason and its message as "Reason: message"; the reason alone when the message is
// empty or repeats the reason, as a StartFailure without a kubelet message does.
func prefixReason(reason, message string) string {
	if message == "" || message == reason {
		return reason
	}
	return reason + ": " + message
}

// isDefinitiveStartFailureReason reports whether reason is one the kubelet uses for a container it cannot
// start as specified: any reason starting with Err or ending in Error or BackOff, the rule Argo CD's health
// check applies, plus InvalidImageName. ErrImagePull is one failed pull attempt and stays a step.
func isDefinitiveStartFailureReason(reason string) bool {
	// the operator's own Degraded reasons end in Error as well
	if reason == ReasonDeploymentError || reason == ReasonServiceError || reason == kubeletReasonErrImagePull {
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
// reconcileFailedStart as Degraded=True and Progressing with the same kubelet reason or with
// ContainerStartFailed, or nil. UpdateErrorStatus writes Degraded alone, so an operator error such as
// ComputeError does not count.
func reportedStartFailure(workspace *workspacev1alpha1.Workspace) *StartFailure {
	degraded := FindCondition(&workspace.Status.Conditions, ConditionTypeDegraded)
	progressing := FindCondition(&workspace.Status.Conditions, ConditionTypeProgressing)
	if degraded == nil || progressing == nil || degraded.Status != metav1.ConditionTrue ||
		progressing.Reason != degraded.Reason {
		return nil
	}
	if degraded.Reason != ReasonContainerStartFailed && !isDefinitiveStartFailureReason(degraded.Reason) {
		return nil
	}
	return &StartFailure{Reason: degraded.Reason, Message: degraded.Message}
}

// crashLoopingContainer returns the first container of the pod, init containers first, that is in a crash
// loop, or nil: a restarted container that is not ready, has exited again or restarted at least twice, and
// whose last run was shorter than crashLoopResetAfter. The kubelet restarts a first exit without back-off,
// and on Kubernetes 1.37 its CrashLoopBackOff waiting report lasts about a second per restart, so the
// Waiting state alone cannot mark the loop.
func crashLoopingContainer(pod *corev1.Pod) *corev1.ContainerStatus {
	statuses := containerStatuses(pod)
	for i := range statuses {
		containerStatus := &statuses[i]
		if containerStatus.Ready || containerStatus.RestartCount == 0 {
			continue
		}
		if containerStatus.State.Terminated == nil && containerStatus.RestartCount < 2 {
			continue
		}
		if last := lastExit(containerStatus); last != nil && last.FinishedAt.Sub(last.StartedAt.Time) < crashLoopResetAfter {
			return containerStatus
		}
	}
	return nil
}

// lastExit returns the exit the pod status records for the container: the current one while it is
// terminated, otherwise the one before its last restart, or nil.
func lastExit(containerStatus *corev1.ContainerStatus) *corev1.ContainerStateTerminated {
	if containerStatus.State.Terminated != nil {
		return containerStatus.State.Terminated
	}
	return containerStatus.LastTerminationState.Terminated
}

// crashLoopFailure describes a crash-looping container as a CrashLoopBackOff failure whose message is the
// latest exit the pod status records, whether the kubelet is reporting its back-off or restarting the
// container, so the message changes only with the exit code or the restart count.
func crashLoopFailure(containerStatus *corev1.ContainerStatus) *StartFailure {
	terminated := lastExit(containerStatus)
	message := fmt.Sprintf("container %s exited with code %d", containerStatus.Name, terminated.ExitCode)
	if terminated.Reason != "" {
		message += " (" + terminated.Reason + ")"
	}
	message += fmt.Sprintf(", restart count %d", containerStatus.RestartCount)
	return startFailure(kubeletReasonCrashLoopBackOff, message)
}

// pullAttemptReasons are the waiting reasons of a failed pull attempt, which the kubelet reports for a few
// seconds each time its back-off expires.
var pullAttemptReasons = map[string]bool{
	kubeletReasonErrImagePull: true, kubeletReasonRegistryUnavailable: true, kubeletReasonSignatureValidationFailed: true,
}

// pullRetrying reports whether the container whose ImagePullBackOff the workspace reports is in a pull
// attempt. The reported message quotes the image with the default tag the kubelet applies, so a sidecar
// retrying another image does not hold the back-off.
func pullRetrying(pod *corev1.Pod, reported *StartFailure) bool {
	for _, containerStatus := range containerStatuses(pod) {
		waiting := containerStatus.State.Waiting
		if waiting == nil || !pullAttemptReasons[waiting.Reason] {
			continue
		}
		if image, found := containerImage(pod, containerStatus.Name); found &&
			strings.Contains(reported.Message, strconv.Quote(imageWithDefaultTag(image))) {
			return true
		}
	}
	return false
}

// imageWithDefaultTag appends the kubelet's default tag to an image reference that carries no tag or digest,
// the form the kubelet quotes in its messages.
func imageWithDefaultTag(image string) string {
	lastSegment := image[strings.LastIndex(image, "/")+1:]
	if strings.Contains(image, "@") || strings.Contains(lastSegment, ":") {
		return image
	}
	return image + ":latest"
}

// containerImage returns the image the pod spec gives the named container, init containers included.
func containerImage(pod *corev1.Pod, name string) (string, bool) {
	for _, containers := range [][]corev1.Container{pod.Spec.InitContainers, pod.Spec.Containers} {
		for _, container := range containers {
			if container.Name == name {
				return container.Image, true
			}
		}
	}
	return "", false
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
		return prefixReason(waiting.Reason, waiting.Message)
	}
	return ""
}

// definitiveStartFailure returns the failure of the first container, init containers first, that the
// kubelet reports as unable to start, or nil.
func definitiveStartFailure(pod *corev1.Pod) *StartFailure {
	statuses := containerStatuses(pod)
	for i := range statuses {
		containerStatus := &statuses[i]
		waiting := containerStatus.State.Waiting
		if waiting == nil || !isDefinitiveStartFailureReason(waiting.Reason) {
			continue
		}
		if waiting.Reason == kubeletReasonCrashLoopBackOff && lastExit(containerStatus) != nil {
			return crashLoopFailure(containerStatus)
		}
		return startFailure(waiting.Reason, waiting.Message)
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
// is the event whose text appeared last, the scheduler's verdict or an autoscaler's such as Karpenter's
// nomination, and the PodScheduled message the fallback. With a node, the kubelet's waiting reason stays
// ContainerCreating through a pull or a failed mount, so its newest event about the pod or a container
// not yet ready is the message and the waiting reason the fallback.
func podStartStep(pod *corev1.Pod, events []corev1.Event) StartStep {
	if !podScheduled(pod) {
		if event := newestEvent(events, nil); event != nil {
			return StartStep{Reason: ReasonWaitingForNode, Message: event.Message}
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Message != "" {
				return StartStep{Reason: ReasonWaitingForNode, Message: condition.Message}
			}
		}
		return StartStep{Reason: ReasonWaitingForNode, Message: waitingForNodeMessage}
	}
	kubeletEvent := newestEvent(events, kubeletStartEvent(pod))
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

// eventFirstSeen returns when an event's text first appeared: FirstTimestamp for a core/v1 event, EventTime
// for an events/v1 one. A repeat of the same text only raises the count and the last timestamp, so a
// verdict the scheduler keeps repeating counts from its first appearance and does not outrank a later one.
func eventFirstSeen(event *corev1.Event) time.Time {
	if !event.FirstTimestamp.IsZero() {
		return event.FirstTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	return event.LastTimestamp.Time
}

func isKubeletEvent(event *corev1.Event) bool {
	return event.Source.Component == kubeletComponent || event.ReportingController == kubeletComponent
}

// containerFieldPath is the involvedObject.fieldPath the kubelet gives a container's events.
var containerFieldPath = regexp.MustCompile(`^spec\.(?:containers|initContainers|ephemeralContainers)\{(.+)\}$`)

// kubeletStartEvent keeps the kubelet's events about the pod itself and about containers that are not
// ready yet. A ready sidecar's Started event would otherwise outrank the main container's pull.
func kubeletStartEvent(pod *corev1.Pod) func(*corev1.Event) bool {
	ready := map[string]bool{}
	for _, status := range containerStatuses(pod) {
		ready[status.Name] = status.Ready
	}
	return func(event *corev1.Event) bool {
		if !isKubeletEvent(event) {
			return false
		}
		match := containerFieldPath.FindStringSubmatch(event.InvolvedObject.FieldPath)
		return match == nil || !ready[match[1]]
	}
}

// newestEvent returns the event with a message whose text appeared last among those keep accepts, every
// event when keep is nil, or nil. Equal timestamps go to the later event in list order: the kubelet
// records timestamps at second resolution, and the API lists events by name, which embeds their creation
// time.
func newestEvent(events []corev1.Event, keep func(*corev1.Event) bool) *corev1.Event {
	var newest *corev1.Event
	for i := range events {
		event := &events[i]
		if event.Message == "" || (keep != nil && !keep(event)) {
			continue
		}
		if newest == nil || !eventFirstSeen(event).Before(eventFirstSeen(newest)) {
			newest = event
		}
	}
	return newest
}

// SetEventReader sets the reader used for a starting pod's events. The manager's API reader fetches one
// pod's events at a time, where its cached client would hold every event in the cluster. Without a reader
// the step falls back to what the pod itself reports.
func (rm *ResourceManager) SetEventReader(reader client.Reader) {
	rm.eventReader = reader
}

// WorkspaceStartStep reports what the workspace's starting pod is doing: a failure the kubelet
// recorded as definitive or a crash loop read from a restarted container, if any, otherwise the step
// it is in with the newest relevant message. Both are nil while no pod exists yet.
func (rm *ResourceManager) WorkspaceStartStep(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) (*StartStep, *StartFailure) {
	pod, err := rm.workspaceStartingPod(ctx, workspace)
	if err != nil {
		// a failed list says nothing new about the pod; what the workspace already reports stands
		logf.FromContext(ctx).Error(err, "Failed to list workspace pods for the start step")
		return nil, reportedStartFailure(workspace)
	}
	if pod == nil {
		return nil, nil
	}
	if failure := definitiveStartFailure(pod); failure != nil {
		return nil, failure
	}
	if crashLooping := crashLoopingContainer(pod); crashLooping != nil {
		return nil, crashLoopFailure(crashLooping)
	}
	// a retry after a back-off does not clear the reported back-off; only a pull that succeeds does
	if reported := reportedStartFailure(workspace); reported != nil &&
		reported.Reason == kubeletReasonImagePullBackOff && pullRetrying(pod, reported) {
		return nil, reported
	}
	step := podStartStep(pod, rm.startEvents(ctx, workspace, pod))
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
// replacement. The cached list has no stable order, so the newest creation time decides, and the name
// within one second.
func (rm *ResourceManager) workspaceStartingPod(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Pod, error) {
	pods, err := workspacePods(ctx, rm.client, workspace)
	if err != nil {
		return nil, err
	}
	var newest *corev1.Pod
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if newest == nil || pod.CreationTimestamp.After(newest.CreationTimestamp.Time) ||
			(pod.CreationTimestamp.Equal(&newest.CreationTimestamp) && pod.Name > newest.Name) {
			newest = pod
		}
	}
	return newest, nil
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
	for cachedKey, entry := range rm.cachedStartEvents {
		if now.Sub(entry.readAt) > startEventsRetention {
			delete(rm.cachedStartEvents, cachedKey)
		}
	}
	rm.cachedStartEvents[key] = startEventsEntry{podUID: pod.UID, readAt: now, events: events}
	rm.startEventsMu.Unlock()
	return events
}
