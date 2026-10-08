/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
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

	// Vocabulary of core Kubernetes components; k8s.io/kubernetes/pkg/kubelet is not importable.
	kubeletComponent               = "kubelet"
	kubeletEventPulling            = "Pulling"
	kubeletReasonContainerCreating = "ContainerCreating"
	kubeletReasonErrImagePull      = "ErrImagePull"
	kubeletReasonImagePullBackOff  = "ImagePullBackOff"
	kubeletReasonInvalidImageName  = "InvalidImageName"
	kubeletReasonCreateConfigError = "CreateContainerConfigError"
	kubeletReasonCrashLoopBackOff  = "CrashLoopBackOff"

	// Fallback messages when neither the pod nor its events carry one.
	waitingForNodeMessage    = "Waiting for a node to run the pod"
	startingContainerMessage = "Starting the container"
)

// definitiveStartFailureReasons names the kubelet waiting reasons under which a container will not start
// without a change to the workspace or its template; IsDefinitiveStartFailureReason extends the set with
// Argo CD's rule for the same verdict, any reason starting with Err or ending with Error or BackOff, which
// also covers ErrImageNeverPull, CreateContainerError and RunContainerError but not InvalidImageName.
var definitiveStartFailureReasons = map[string]bool{
	kubeletReasonErrImagePull:      true,
	kubeletReasonImagePullBackOff:  true,
	kubeletReasonInvalidImageName:  true,
	kubeletReasonCreateConfigError: true,
	kubeletReasonCrashLoopBackOff:  true,
}

// StartStep is the step a starting workspace pod is in: the Progressing reason and the message the
// component responsible for that step recorded, copied unchanged.
type StartStep struct {
	Reason  string
	Message string
}

// StartFailure is a kubelet waiting reason under which the pod's container will not start, with the
// kubelet's message.
type StartFailure struct {
	Reason  string
	Message string
}

// IsDefinitiveStartFailureReason reports whether reason is one the kubelet uses for a container that
// will not start without a change to the workspace or its template.
func IsDefinitiveStartFailureReason(reason string) bool {
	return definitiveStartFailureReasons[reason] || strings.HasPrefix(reason, "Err") ||
		strings.HasSuffix(reason, "Error") || strings.HasSuffix(reason, "BackOff")
}

// startEventsEntry is the last read of a starting pod's events for one workspace.
type startEventsEntry struct {
	podUID types.UID
	readAt time.Time
	events []corev1.Event
}

// definitiveStartFailure returns the first container, init containers first, that the kubelet reports
// as unable to start, or nil.
func definitiveStartFailure(pod *corev1.Pod) *StartFailure {
	for _, statuses := range [][]corev1.ContainerStatus{pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses} {
		for _, containerStatus := range statuses {
			waiting := containerStatus.State.Waiting
			if waiting == nil || !IsDefinitiveStartFailureReason(waiting.Reason) {
				continue
			}
			message := waiting.Message
			if message == "" {
				message = waiting.Reason
			}
			return &StartFailure{Reason: waiting.Reason, Message: message}
		}
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

// newestEvent returns the most recently observed event accepted by keep, or nil.
func newestEvent(events []corev1.Event, keep func(*corev1.Event) bool) *corev1.Event {
	var newest *corev1.Event
	for i := range events {
		event := &events[i]
		if !keep(event) {
			continue
		}
		if newest == nil || eventTime(event).After(eventTime(newest)) {
			newest = event
		}
	}
	return newest
}

// SetEventReader sets the reader used for a starting pod's events. The manager's API reader is used
// so that events are fetched for one pod at a time instead of cached cluster-wide; without a reader
// the step falls back to what the pod itself reports.
func (rm *ResourceManager) SetEventReader(reader client.Reader) {
	rm.eventReader = reader
}

// WorkspaceStartStep reports what the workspace's starting pod is doing: a failure the kubelet
// recorded as definitive, if any, otherwise the step it is in with the newest relevant message. Both
// are nil while no pod exists yet. Pods being deleted are skipped, as in WorkspacePodStallMessage.
func (rm *ResourceManager) WorkspaceStartStep(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) (*StartStep, *StartFailure) {
	pod := rm.workspaceStartingPod(ctx, workspace)
	if pod == nil {
		return nil, nil
	}
	if failure := definitiveStartFailure(pod); failure != nil {
		return nil, failure
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

func (rm *ResourceManager) workspaceStartingPod(ctx context.Context, workspace *workspacev1alpha1.Workspace) *corev1.Pod {
	podList := &corev1.PodList{}
	if err := rm.client.List(ctx, podList,
		client.InNamespace(workspace.Namespace),
		client.MatchingLabels(GenerateLabels(workspace.Name)),
	); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list workspace pods for the start step")
		return nil
	}
	for i := range podList.Items {
		if podList.Items[i].DeletionTimestamp == nil {
			return &podList.Items[i]
		}
	}
	return nil
}

// startEvents returns the events recorded on the pod, read through the event reader at most once per
// startEventsInterval per workspace and reused in between. A failed read keeps the previous events.
func (rm *ResourceManager) startEvents(ctx context.Context, workspace *workspacev1alpha1.Workspace, pod *corev1.Pod) []corev1.Event {
	if rm.eventReader == nil {
		return nil
	}
	key := client.ObjectKeyFromObject(workspace)
	now := rm.now()

	rm.startEventsMu.Lock()
	for cachedKey, cached := range rm.cachedStartEvents {
		if now.Sub(cached.readAt) > startEventsRetention {
			delete(rm.cachedStartEvents, cachedKey)
		}
	}
	cached, found := rm.cachedStartEvents[key]
	rm.startEventsMu.Unlock()
	if found && cached.podUID == pod.UID && now.Sub(cached.readAt) < startEventsInterval {
		return cached.events
	}

	eventList := &corev1.EventList{}
	err := rm.eventReader.List(ctx, eventList,
		client.InNamespace(pod.Namespace),
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("involvedObject.uid", string(pod.UID))},
	)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list the events of the starting pod", "pod", pod.Name)
		if found && cached.podUID == pod.UID {
			return cached.events
		}
		return nil
	}

	rm.startEventsMu.Lock()
	rm.cachedStartEvents[key] = startEventsEntry{podUID: pod.UID, readAt: now, events: eventList.Items}
	rm.startEventsMu.Unlock()
	return eventList.Items
}
