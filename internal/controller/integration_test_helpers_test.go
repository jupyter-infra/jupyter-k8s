/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Shared integration template test fixtures, used across the integration test files in this
// package (resolver, baker). Kept in this dedicated helpers file so they're discoverable
// without relying on source-file ordering.

const (
	testParamClusterName = "clusterName"
	testKindRayCluster   = "RayCluster"
	testEnvRayAddress    = "RAY_ADDRESS"
	testRefIDRayCluster  = "rayCluster"
)

func testResource() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"headGroupSpec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"image": "rayproject/ray:2.9.0",
								},
							},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"head": map[string]interface{}{
					"serviceName": "my-cluster-head-svc",
				},
			},
		},
	}
}

// testResources returns the fetched-resources map keyed by resourceRef id, as the
// resolver consumes it. The single ray cluster is registered under "rayCluster".
func testResources() map[string]*unstructured.Unstructured {
	return map[string]*unstructured.Unstructured{
		testRefIDRayCluster: testResource(),
	}
}

func testTemplateData() IntegrationTemplateData {
	return IntegrationTemplateData{
		Workspace: IntegrationWorkspaceData{
			Name:      "my-workspace",
			Namespace: "user-ns",
		},
		Parameters: map[string]string{
			testParamClusterName: "my-ray-cluster",
			"region":             "us-west-2",
		},
	}
}
