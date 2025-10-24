package controller

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Variables for dependency injection in tests
var (
	getConfig       = config.GetConfig
	newClientset    = kubernetes.NewForConfig
	newSPDYExecutor = remotecommand.NewSPDYExecutor
)

// PodExecUtil provides utilities for executing commands in pods
type PodExecUtil struct {
	clientset kubernetes.Interface
	config    *rest.Config
}

// NewPodExecUtil creates a new PodExecUtil instance
func NewPodExecUtil() (*PodExecUtil, error) {
	cfg, err := getConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	clientset, err := newClientset(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	return &PodExecUtil{
		clientset: clientset,
		config:    cfg,
	}, nil
}

// ExecInPod executes a command in a specific container of a pod with optional stdin input
func (p *PodExecUtil) ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error) {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "container", containerName, "cmd", cmd)

	// Create exec request
	req := p.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")

	// Enable stdin only if we have stdin data
	hasStdin := stdin != ""
	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   cmd,
		Stdin:     hasStdin,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	// Execute command
	exec, err := newSPDYExecutor(p.config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	streamOptions := remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// Add stdin only if we have data
	if hasStdin {
		streamOptions.Stdin = strings.NewReader(stdin)
	}

	err = exec.StreamWithContext(ctx, streamOptions)

	output := strings.TrimSpace(stdout.String())
	if err != nil {
		logger.V(1).Info("Command execution failed", "hasStdin", hasStdin, "error", err, "stderr", stderr.String())
		return output, err
	}

	logger.V(1).Info("Command executed successfully", "hasStdin", hasStdin, "output", output)
	return output, nil
}
