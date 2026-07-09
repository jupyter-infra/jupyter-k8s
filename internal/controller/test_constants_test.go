/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

// Shared test constants. Defined once at package scope so goconst does not flag
// the same literal repeated across the controller test files.
const (
	testNamespaceName            = "test-namespace"
	testStrategyName             = "test-strategy"
	accessStrategyNamespaceConst = "strategy-namespace"
	testServiceName              = "test-service"
	testRouteName                = "test-route"
	testRouteNameOne             = "test-route-1"
	testWorkspaceDisplayName     = "Test Workspace"
	testStrategyDisplayName      = "Test Strategy"
	testTemplateDisplayName      = "Test Template"

	imageMinimalNotebook     = "jupyter/minimal-notebook:latest"
	imageQuayMinimalNotebook = "quay.io/jupyter/minimal-notebook:latest"
	imageBaseNotebook        = "jupyter/base-notebook:latest"
	imageV2                  = "image:v2"

	envJupyterBaseURL = "JUPYTER_BASE_URL"
	envExistingVar    = "EXISTING_VAR"

	deploymentLabelTestWorkspace = "workspace-test-workspace"
	podNameWorkspaceSuffix       = "workspace-pod"

	shellBinSh              = "/bin/sh"
	volumeNameSharedStorage = "shared-storage"
	mountPathSharedData     = "/shared-data"

	testWorkspaceNameWS1        = "ws1"
	traefikGroup                = "traefik.io"
	resourceIngressRoutes       = "ingressroutes"
	accessStrategyNameWebAccess = "web-access"
	templateNameMy              = "my-template"
	templateNameTmpl            = "tmpl"

	responseBodyPathLastActivity = "last_activity"
	exampleURLTemplate           = "http://example.com/test"

	containerNameMain = "main"
	literalTest       = "test"

	conditionTypeExisting = "Existing"
	conditionTypeToUpdate = "ToUpdate"

	pathAPIStatus               = "/api/status"
	pathAPIIdle                 = "/api/idle"
	pathAPIStatusNoLeadingSlash = "api/status"
	pathWorkspacesAPIStatus     = "/workspaces/ns/ws/api/status"
	urlProbeAPIStatus           = "http://10.96.0.1:8888/api/status"
	testHostIP                  = "10.96.0.1"
	timestampFixed              = "2024-01-01T00:00:00Z"
)
