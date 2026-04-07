/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package pluginadapters defines interfaces for handling plugin lifecycle events
// in a plugin-agnostic way. Implementations live in separate packages
// (e.g., internal/awsadapter/).
package pluginadapters

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// PodEventPluginAdapter defines the interface for handling pod lifecycle events.
// Implementations receive a resolved context map (with all dynamic values
// like controller::PodUid() already substituted).
type PodEventPluginAdapter interface {
	HandlePodRunning(ctx context.Context, pod *corev1.Pod, workspaceName, namespace string, podEventsContext map[string]string) error
	HandlePodDeleted(ctx context.Context, pod *corev1.Pod, podEventsContext map[string]string) error
}

// ResolvePodContext resolves dynamic values in the pod events context map.
// Values prefixed with "controller::" are replaced with computed values.
// Currently supported: "controller::PodUid()" → pod.UID
func ResolvePodContext(contextMap map[string]string, pod *corev1.Pod) (map[string]string, error) {
	if len(contextMap) == 0 {
		return contextMap, nil
	}

	resolved := make(map[string]string, len(contextMap))
	for k, v := range contextMap {
		if strings.HasPrefix(v, controllerFuncPrefix) {
			funcName := strings.TrimPrefix(v, controllerFuncPrefix)
			val, err := resolvePodControllerFunc(funcName, pod)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve %q for key %q: %w", v, k, err)
			}
			resolved[k] = val
		} else {
			resolved[k] = v
		}
	}
	return resolved, nil
}

func resolvePodControllerFunc(funcName string, pod *corev1.Pod) (string, error) {
	switch funcName {
	case funcPodUID:
		if pod == nil {
			return "", fmt.Errorf("pod is nil, cannot resolve PodUid()")
		}
		return string(pod.UID), nil
	default:
		return "", fmt.Errorf("unknown controller function: %q", funcName)
	}
}
