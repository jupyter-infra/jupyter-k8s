/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

//revive:disable:var-naming
package utils

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// WaitForPVCBinding waits for a PVC to be bound
func WaitForPVCBinding(pvcName, namespace string, timeout time.Duration) error {
	return WaitForCondition(func() error {
		cmd := exec.Command("kubectl", "get", "pvc", pvcName, "-n", namespace,
			"-o", "jsonpath={.status.phase}")
		output, err := Run(cmd)
		if err != nil {
			return err
		}
		if output != "Bound" {
			return fmt.Errorf("PVC %s not bound, status: %s", pvcName, output)
		}
		return nil
	}, timeout, 5*time.Second)
}

// VerifyPVCSize checks if PVC has the expected storage size
func VerifyPVCSize(pvcName, namespace, expectedSize string) error {
	cmd := exec.Command("kubectl", "get", "pvc", pvcName, "-n", namespace,
		"-o", "jsonpath={.spec.resources.requests.storage}")
	output, err := Run(cmd)
	if err != nil {
		return err
	}
	if output != expectedSize {
		return fmt.Errorf("expected PVC size %s, got %s", expectedSize, output)
	}
	return nil
}

// VerifyPVCOwnerReference checks if PVC has correct owner reference
func VerifyPVCOwnerReference(pvcName, namespace, ownerName string) error {
	cmd := exec.Command("kubectl", "get", "pvc", pvcName, "-n", namespace,
		"-o", "jsonpath={.metadata.ownerReferences[0].name}")
	output, err := Run(cmd)
	if err != nil {
		return err
	}
	if output != ownerName {
		return fmt.Errorf("expected owner reference %s, got %s", ownerName, output)
	}
	return nil
}

// VerifyVolumeMount checks if deployment has correct volume mount
func VerifyVolumeMount(deploymentName, namespace, volumeName, mountPath string) error {
	// Check volume mount exists
	cmd := exec.Command("kubectl", "get", "deployment", deploymentName, "-n", namespace,
		"-o", "jsonpath={.spec.template.spec.containers[0].volumeMounts}")
	output, err := Run(cmd)
	if err != nil {
		return err
	}
	if !strings.Contains(output, volumeName) {
		return fmt.Errorf("volume mount %s not found in deployment %s", volumeName, deploymentName)
	}

	// Check mount path if specified
	if mountPath != "" {
		jsonPath := fmt.Sprintf("jsonpath={.spec.template.spec.containers[0].volumeMounts[?(@.name=='%s')].mountPath}",
			volumeName)
		cmd = exec.Command("kubectl", "get", "deployment", deploymentName, "-n", namespace, "-o", jsonPath)
		pathOutput, err := Run(cmd)
		if err != nil {
			return err
		}
		if pathOutput != mountPath {
			return fmt.Errorf("expected mount path %s, got %s", mountPath, pathOutput)
		}
	}

	return nil
}

// VerifyVolumeDefinition checks if deployment has correct volume definition
func VerifyVolumeDefinition(deploymentName, namespace, volumeName, pvcName string) error {
	cmd := exec.Command("kubectl", "get", "deployment", deploymentName, "-n", namespace,
		"-o", "jsonpath={.spec.template.spec.volumes}")
	output, err := Run(cmd)
	if err != nil {
		return err
	}
	if !strings.Contains(output, volumeName) || !strings.Contains(output, pvcName) {
		return fmt.Errorf("volume definition for %s with PVC %s not found in deployment %s",
			volumeName, pvcName, deploymentName)
	}
	return nil
}

// TestVolumeWriteAccess tests if a pod can write to a mounted volume
func TestVolumeWriteAccess(podName, namespace, mountPath, testContent string) error {
	testFile := fmt.Sprintf("%s/test-write-%d.txt", mountPath, time.Now().Unix())

	// Write test content
	cmd := exec.Command("kubectl", "exec", podName, "-n", namespace, "--",
		"sh", "-c", fmt.Sprintf("echo '%s' > %s", testContent, testFile))
	_, err := Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to write to volume: %w", err)
	}

	// Read back content
	cmd = exec.Command("kubectl", "exec", podName, "-n", namespace, "--",
		"cat", testFile)
	output, err := Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to read from volume: %w", err)
	}

	if strings.TrimSpace(output) != testContent {
		return fmt.Errorf("expected content %s, got %s", testContent, strings.TrimSpace(output))
	}

	return nil
}

// WaitForPodRunning waits for a pod with given labels to be running
func WaitForPodRunning(labelSelector, namespace string, timeout time.Duration) (string, error) {
	var podName string

	err := WaitForCondition(func() error {
		cmd := exec.Command("kubectl", "get", "pods", "-l", labelSelector, "-n", namespace,
			"-o", "jsonpath={.items[0].metadata.name}")
		name, err := Run(cmd)
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("no pods found with label %s", labelSelector)
		}
		podName = name

		cmd = exec.Command("kubectl", "get", "pod", podName, "-n", namespace,
			"-o", "jsonpath={.status.phase}")
		phase, err := Run(cmd)
		if err != nil {
			return err
		}
		if phase != "Running" {
			return fmt.Errorf("pod %s not running, status: %s", podName, phase)
		}
		return nil
	}, timeout, 10*time.Second)

	return podName, err
}

// WaitForCondition waits for a condition to be true
func WaitForCondition(condition func() error, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := condition(); err == nil {
			return nil
		}
		time.Sleep(interval)
	}
	return condition() // Return the last error
}
