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
	// startEventsReadTimeout bounds one event read: one reconcile worker serves every workspace, so a slow
	// API server must not hold it. KubeSpawner bounds its API calls the same way.
	startEventsReadTimeout = 3 * time.Second

	// eventInvolvedObjectUIDField selects the events recorded about one object, as a field selector on the
	// API server and as an index on the manager cache.
	eventInvolvedObjectUIDField = "involvedObject.uid"

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
	kubeletReasonCreateContainerError      = "CreateContainerError"
	kubeletReasonRunContainerError         = "RunContainerError"
	// kubeletEventFailed is the reason of the kubelet's Warning event for a pull, create or start attempt
	// that did not succeed; kubeletErrorEventPrefix opens its message, followed by the error's text.
	kubeletEventFailed      = "Failed"
	kubeletErrorEventPrefix = "Error: "

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
// check applies, plus InvalidImageName. ErrImagePull is one failed pull attempt and stays a step;
// retriedStartReasons count only once the kubelet has failed twice (definitiveStartFailure).
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

// startStepReasons are the Progressing reasons of a start under way.
var startStepReasons = map[string]bool{ReasonWaitingForNode: true, ReasonPullingImage: true, ReasonStartingContainer: true}

// reportedStartStep returns the step the workspace's Progressing condition already carries, or nil.
func reportedStartStep(workspace *workspacev1alpha1.Workspace) *StartStep {
	progressing := FindCondition(&workspace.Status.Conditions, ConditionTypeProgressing)
	if progressing == nil || progressing.Status != metav1.ConditionTrue || !startStepReasons[progressing.Reason] {
		return nil
	}
	return &StartStep{Reason: progressing.Reason, Message: progressing.Message}
}

// crashLoopingContainer returns the first container of the pod, init containers first, that is in a crash
// loop, or nil: a restarted container that is not ready, has exited again or restarted at least twice, and
// whose last exit is younger than crashLoopResetAfter and ended a run shorter than it. The kubelet restarts a first exit without back-off,
// and on Kubernetes 1.37 its CrashLoopBackOff waiting report lasts about a second per restart, so the
// Waiting state alone cannot mark the loop.
func crashLoopingContainer(pod *corev1.Pod, now time.Time) *corev1.ContainerStatus {
	statuses := containerStatuses(pod)
	for i := range statuses {
		containerStatus := &statuses[i]
		if containerStatus.Ready || containerStatus.RestartCount == 0 {
			continue
		}
		if containerStatus.State.Terminated == nil && containerStatus.RestartCount < 2 {
			continue
		}
		if last := lastExit(containerStatus); last != nil && now.Sub(last.FinishedAt.Time) < crashLoopResetAfter && shortRun(last) {
			return containerStatus
		}
	}
	return nil
}

// shortRun reports whether an exit ended a run shorter than crashLoopResetAfter. A container whose start
// itself failed (StartError, exit code 128) has no start time, which the kubelet reports as the Unix epoch.
func shortRun(exit *corev1.ContainerStateTerminated) bool {
	started := exit.StartedAt.Time
	return started.IsZero() || started.Unix() == 0 || exit.FinishedAt.Sub(started) < crashLoopResetAfter
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

// attemptReasons are the waiting reasons of one failed attempt the kubelet retries: a pull, a create or a start.
var attemptReasons = map[string]bool{
	kubeletReasonErrImagePull: true, kubeletReasonRegistryUnavailable: true, kubeletReasonSignatureValidationFailed: true,
	kubeletReasonCreateContainerError: true, kubeletReasonRunContainerError: true,
}

// pullRetrying reports whether the container whose ImagePullBackOff the workspace reports is in a pull
// attempt. The reported message quotes the container's image, so a sidecar retrying another image does not
// hold the back-off; kubelets before 1.33 quote the image as written and later ones with the default tag.
func pullRetrying(pod *corev1.Pod, reported *StartFailure) bool {
	for _, containerStatus := range containerStatuses(pod) {
		waiting := containerStatus.State.Waiting
		if waiting == nil || !pullAttemptReasons[waiting.Reason] {
			continue
		}
		image, found := containerImage(pod, containerStatus.Name)
		if found && (strings.Contains(reported.Message, strconv.Quote(image)) ||
			strings.Contains(reported.Message, strconv.Quote(imageWithDefaultTag(image)))) {
			return true
		}
	}
	return false
}

// imageWithDefaultTag appends the kubelet's default tag to an image reference that carries no tag or digest,
// the form kubelets from 1.33 on quote in their messages.
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
// containers first (e.g. "ImagePullBackOff: Back-off pulling image ..."), or "". With reasons set, only a
// container waiting on one of them counts.
func waitingContainerMessage(pod *corev1.Pod, reasons map[string]bool) string {
	for _, containerStatus := range containerStatuses(pod) {
		waiting := containerStatus.State.Waiting
		if waiting == nil || waiting.Reason == "" || (reasons != nil && !reasons[waiting.Reason]) {
			continue
		}
		return prefixReason(waiting.Reason, waiting.Message)
	}
	return ""
}

// retriedStartReasons are the waiting reasons of a create or start attempt the kubelet retries after its
// back-off: a loaded node reports them once and clears them, so they count as a failure only once the
// kubelet has failed twice for the container, which its Failed events record.
var retriedStartReasons = map[string]bool{kubeletReasonCreateContainerError: true, kubeletReasonRunContainerError: true}

// pullFailureMessages open the kubelet's Failed events about a pull attempt, which are not create or start
// attempts: the pull's own report and the start attempt it fails.
var pullFailureMessages = []string{"Failed to pull image", kubeletErrorEventPrefix + kubeletReasonErrImagePull, kubeletErrorEventPrefix + kubeletReasonImagePullBackOff}

func isPullFailureMessage(message string) bool {
	for _, prefix := range pullFailureMessages {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

// failedAttempts counts the kubelet's Failed events about create or start attempts of the named container
// that report the error the container is waiting on, each repeated event standing for its count. The
// kubelet writes the same text to the waiting state and to the event, behind "Error: ", so events of an
// earlier error, such as a Secret that was missing, do not count towards this one. Without a waiting message
// every create or start attempt counts.
func failedAttempts(events []corev1.Event, container, waitingMessage string) int {
	attempts := 0
	for i := range events {
		event := &events[i]
		if event.Reason != kubeletEventFailed || !isKubeletEvent(event) || isPullFailureMessage(event.Message) {
			continue
		}
		if waitingMessage != "" && event.Message != kubeletErrorEventPrefix+waitingMessage {
			continue
		}
		if match := containerFieldPathPattern.FindStringSubmatch(event.InvolvedObject.FieldPath); match != nil && match[1] == container {
			attempts += eventCount(event)
		}
	}
	return attempts
}

// eventCount returns how many times an event's text was recorded: the series count of an events/v1
// event, the count of a core/v1 one, and one for an event that carries neither.
func eventCount(event *corev1.Event) int {
	if event.Series != nil && event.Series.Count > 0 {
		return int(event.Series.Count)
	}
	if event.Count > 0 {
		return int(event.Count)
	}
	return 1
}

// logUnmatchedFailedAttempts notes, at verbosity 1, a container waiting on a retried error whose Failed events
// all report other text, so a change in the kubelet's wording shows in the operator log instead of holding
// the container at a step until the progress deadline.
func logUnmatchedFailedAttempts(ctx context.Context, pod *corev1.Pod, events []corev1.Event) {
	for _, containerStatus := range containerStatuses(pod) {
		waiting := containerStatus.State.Waiting
		if waiting == nil || !retriedStartReasons[waiting.Reason] || waiting.Message == "" ||
			failedAttempts(events, containerStatus.Name, waiting.Message) > 0 {
			continue
		}
		if all := failedAttempts(events, containerStatus.Name, ""); all > 0 {
			logf.FromContext(ctx).V(1).Info("The container's Failed events report another error than its waiting state",
				"container", containerStatus.Name, "waiting", waiting.Message, "attempts", all)
		}
	}
}

// definitiveStartFailure returns the failure of the first container, init containers first, that the
// kubelet reports as unable to start, or nil. A retried create or start error needs two failed attempts
// among the pod's events.
func definitiveStartFailure(pod *corev1.Pod, events []corev1.Event) *StartFailure {
	statuses := containerStatuses(pod)
	for i := range statuses {
		containerStatus := &statuses[i]
		waiting := containerStatus.State.Waiting
		if waiting == nil || !isDefinitiveStartFailureReason(waiting.Reason) {
			continue
		}
		if retriedStartReasons[waiting.Reason] && failedAttempts(events, containerStatus.Name, waiting.Message) < 2 {
			continue
		}
		if waiting.Reason == kubeletReasonCrashLoopBackOff {
			if lastExit(containerStatus) != nil {
				return crashLoopFailure(containerStatus)
			}
			// the kubelet's own text carries the growing back-off delay, which would rewrite the status every cycle
			return startFailure(waiting.Reason, "container "+containerStatus.Name+" is restarting with back-off")
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
// is the newest event by first appearance, from the scheduler or an autoscaler such as Karpenter; the
// PodScheduled condition's message is the fallback. With a node, the kubelet's waiting reason stays
// ContainerCreating through a pull or a failed mount, so the message is the newest event about the pod
// or the kubelet's about a container that is not ready yet; the waiting reason is the fallback.
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
	kubeletEvent := newestEvent(events, startingPodEvent(pod))
	if kubeletEvent != nil && isKubeletEvent(kubeletEvent) && kubeletEvent.Reason == kubeletEventPulling {
		return StartStep{Reason: ReasonPullingImage, Message: kubeletEvent.Message}
	}
	// an attempt the kubelet is retrying names its error in the waiting state; its Failed events only say
	// "Error: ErrImagePull" or "Error: ..."
	if message := waitingContainerMessage(pod, attemptReasons); message != "" {
		return StartStep{Reason: ReasonStartingContainer, Message: message}
	}
	if kubeletEvent != nil && kubeletEvent.Message != "" {
		return StartStep{Reason: ReasonStartingContainer, Message: kubeletEvent.Message}
	}
	if message := waitingContainerMessage(pod, nil); message != "" {
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

// containerFieldPathPattern is the involvedObject.fieldPath the kubelet gives a container's events.
var containerFieldPathPattern = regexp.MustCompile(`^spec\.(?:containers|initContainers|ephemeralContainers)\{(.+)\}$`)

// startingPodEvent keeps the events about the pod itself from any component, such as the volume attach
// controller's FailedAttachVolume, and the kubelet's events about containers that are not ready yet. A
// ready sidecar's Started event would otherwise outrank the main container's pull.
func startingPodEvent(pod *corev1.Pod) func(*corev1.Event) bool {
	ready := map[string]bool{}
	for _, status := range containerStatuses(pod) {
		ready[status.Name] = status.Ready
	}
	return func(event *corev1.Event) bool {
		match := containerFieldPathPattern.FindStringSubmatch(event.InvolvedObject.FieldPath)
		if match == nil {
			return event.InvolvedObject.FieldPath == ""
		}
		return isKubeletEvent(event) && !ready[match[1]]
	}
}

// newestEvent returns the event with a message whose text appeared last, among those keep accepts or among
// all events when keep is nil; nil when there is none. The kubelet records timestamps at second resolution,
// so equal timestamps go to the greater name: the recorder names an event after its creation time in
// nanoseconds, and a list served from the cache has no order of its own.
func newestEvent(events []corev1.Event, keep func(*corev1.Event) bool) *corev1.Event {
	var newest *corev1.Event
	for i := range events {
		event := &events[i]
		if event.Message == "" || (keep != nil && !keep(event)) {
			continue
		}
		if newest == nil || eventFirstSeen(event).After(eventFirstSeen(newest)) ||
			(eventFirstSeen(event).Equal(eventFirstSeen(newest)) && event.Name > newest.Name) {
			newest = event
		}
	}
	return newest
}

// SetEventReader sets the reader used for a starting pod's events, the manager's API reader; its reads are
// limited across workspaces. Without a reader the step falls back to what the pod itself reports.
func (rm *ResourceManager) SetEventReader(reader client.Reader) {
	rm.eventReader = reader
	rm.eventReaderCached = false
}

// SetEventCacheReader sets the manager's cached client as the event reader, once EventInvolvedObjectUID is
// registered as the eventInvolvedObjectUIDField index; its reads cost no API request, so they run on every
// reconcile without the interval or the limiter, and the per-workspace cache only covers a failed read.
func (rm *ResourceManager) SetEventCacheReader(reader client.Reader) {
	rm.eventReader = reader
	rm.eventReaderCached = true
}

// EventInvolvedObjectUID indexes events by the UID of the object they are about, so the manager cache can
// serve the reads startEvents makes with eventInvolvedObjectUIDField.
func EventInvolvedObjectUID(obj client.Object) []string {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return nil
	}
	return []string{string(event.InvolvedObject.UID)}
}

// WorkspaceStartStep reports what the workspace's starting pod is doing: a failure the kubelet
// recorded as definitive or a crash loop read from a restarted container, if any, otherwise the step
// it is in with the newest relevant message. Both are nil while no pod exists yet. The pod's events are
// read once for both verdicts.
func (rm *ResourceManager) WorkspaceStartStep(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) (*StartStep, *StartFailure) {
	pod, err := rm.workspaceStartingPod(ctx, workspace)
	if err != nil {
		// a failed list says nothing new about the pod; what the workspace already reports stands
		logf.FromContext(ctx).Error(err, "Failed to list workspace pods for the start step")
		return reportedStartStep(workspace), reportedStartFailure(workspace)
	}
	if pod == nil {
		return nil, nil
	}
	events := rm.startEvents(ctx, workspace, pod)
	if failure := definitiveStartFailure(pod, events); failure != nil {
		return nil, failure
	}
	logUnmatchedFailedAttempts(ctx, pod, events)
	if crashLooping := crashLoopingContainer(pod, rm.now()); crashLooping != nil {
		return nil, crashLoopFailure(crashLooping)
	}
	// a retry after a back-off does not clear the reported back-off; only a pull that succeeds does
	if reported := reportedStartFailure(workspace); reported != nil &&
		reported.Reason == kubeletReasonImagePullBackOff && pullRetrying(pod, reported) {
		return nil, reported
	}
	step := podStartStep(pod, events)
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

// startEvents returns the events recorded on the pod, read through the API reader at most once per
// startEventsInterval per workspace and at most startEventsPerSecond across workspaces, and reused in
// between, or through the manager cache on every call. A failed read, a timeout included, keeps the
// previous events and counts as a read, so the interval holds through an API outage; a read the shared
// limiter denies does not, so the next reconcile tries again. Events are text that anyone with event write access in the namespace can create: they
// supply messages, and the only verdict they touch is the count of failed attempts, which a forged event
// can bring forward for a reason the pod status already reports.
func (rm *ResourceManager) startEvents(ctx context.Context, workspace *workspacev1alpha1.Workspace, pod *corev1.Pod) []corev1.Event {
	if rm.eventReader == nil {
		return nil
	}
	key := client.ObjectKeyFromObject(workspace)
	now := rm.now()

	rm.startEventsMu.Lock()
	cached, found := rm.cachedStartEvents[key]
	rm.startEventsMu.Unlock()
	if !rm.eventReaderCached && found && cached.podUID == pod.UID && now.Sub(cached.readAt) < startEventsInterval {
		return cached.events
	}
	if !rm.eventReaderCached && !rm.eventReadLimiter.Allow() {
		if found && cached.podUID == pod.UID {
			return cached.events
		}
		return nil
	}

	eventList := &corev1.EventList{}
	// ResourceVersion 0 lets the API server answer from its watch cache instead of a quorum read from etcd;
	// events a moment stale are fine for a status message.
	readCtx, cancel := context.WithTimeout(ctx, rm.eventReadTimeout)
	defer cancel()
	err := rm.eventReader.List(readCtx, eventList,
		client.InNamespace(pod.Namespace),
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(eventInvolvedObjectUIDField, string(pod.UID))},
		&client.ListOptions{Raw: &metav1.ListOptions{ResourceVersion: "0"}},
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
