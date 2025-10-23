/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package controller

import (
	"context"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	builderPkg "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	mngr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// GVKWatch represents a Group-Version-Kind to watch
type GVKWatch struct {
	Group   string
	Version string
	Kind    string
}

// WorkspaceControllerOptions contains configuration options for the Workspace controller
type WorkspaceControllerOptions struct {
	// ApplicationImagesPullPolicy defines how application container images should be pulled
	ApplicationImagesPullPolicy corev1.PullPolicy

	// Registry is the prefix to use for all application images
	ApplicationImagesRegistry string

	// Flag to indicate whether to watch traefik resource (for AccessStrategy)
	// Deprecated: Use ResourceWatches instead
	WatchTraefik bool

	// ResourceWatches defines custom Group-Version-Kind resources to watch
	ResourceWatches []GVKWatch

	// EnableWorkspacePodWatching controls whether workspace pod events should be watched
	EnableWorkspacePodWatching bool
}

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	stateMachine    *StateMachine
	statusManager   *StatusManager
	podEventHandler *PodEventHandler
	options         WorkspaceControllerOptions
}

// SetStateMachine sets the state machine for testing purposes
func (r *WorkspaceReconciler) SetStateMachine(sm *StateMachine) {
	r.stateMachine = sm
}

// +kubebuilder:rbac:groups=workspaces.jupyter.org,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=traefik.io,resources=ingressroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=traefik.io,resources=middlewares,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Workspace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Starting reconciliation", "workspace", req.NamespacedName)

	// Fetch the Workspace instance
	workspace, err := r.getWorkspace(ctx, req)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Workspace not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Workspace")
		return ctrl.Result{}, err
	}

	// Ensure template label is set if workspace uses a template
	if workspace.Spec.TemplateRef != nil && *workspace.Spec.TemplateRef != "" {
		if workspace.Labels == nil {
			workspace.Labels = make(map[string]string)
		}
		expectedLabel := "workspaces.jupyter.org/template"
		if workspace.Labels[expectedLabel] != *workspace.Spec.TemplateRef {
			logger.Info("Adding template label to workspace", "template", *workspace.Spec.TemplateRef)
			workspace.Labels[expectedLabel] = *workspace.Spec.TemplateRef
			if err := r.Update(ctx, workspace); err != nil {
				logger.Error(err, "Failed to update workspace with template label")
				return ctrl.Result{}, err
			}
			logger.Info("Successfully added template label to workspace")
			// Requeue to process with updated labels
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Delegate to state machine for business logic
	result, err := r.stateMachine.ReconcileDesiredState(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to reconcile desired state")
		return ctrl.Result{}, err
	}

	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&workspacesv1alpha1.Workspace{}).
		Named("workspace").
		// Watch for standard Kubernetes resources
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{})

	// Conditionally watch pods based on configuration
	if r.options.EnableWorkspacePodWatching {
		builder.Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.podEventHandler.HandleWorkspacePodEvents),
			builderPkg.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				// Only watch pods with workspace labels
				_, hasWorkspace := obj.GetLabels()[LabelWorkspaceName]
				return hasWorkspace
			})),
		)
	}

	// Optional traefik configuration (backward compatibility)
	if r.options.WatchTraefik {
		// Create an IngressRoute unstructured object for watching
		ingressRouteGVK := &unstructured.Unstructured{}
		ingressRouteGVK.SetAPIVersion("traefik.io/v1alpha1")
		ingressRouteGVK.SetKind("IngressRoute")

		// Create a Middleware unstructured object for watching
		middlewareGVK := &unstructured.Unstructured{}
		middlewareGVK.SetAPIVersion("traefik.io/v1alpha1")
		middlewareGVK.SetKind("Middleware")

		builder.Owns(ingressRouteGVK).Owns(middlewareGVK)
	}

	// Add additional resource watches from ResourceWatches config
	for _, gvk := range r.options.ResourceWatches {
		obj := &unstructured.Unstructured{}

		var apiVersion string
		if gvk.Group == "" {
			// Core API group
			apiVersion = gvk.Version
		} else {
			apiVersion = gvk.Group + "/" + gvk.Version
		}

		obj.SetAPIVersion(apiVersion)
		obj.SetKind(gvk.Kind)
		builder.Owns(obj)
	}

	return builder.Complete(r)
}

// SetupWorkspaceController sets up the controller with the Manager and specified options
func SetupWorkspaceController(mgr mngr.Manager, options WorkspaceControllerOptions) error {
	k8sClient := mgr.GetClient()
	scheme := mgr.GetScheme()

	// Create managers
	statusManager := NewStatusManager(k8sClient)
	resourceManager := NewResourceManager(
		k8sClient,
		scheme,
		NewDeploymentBuilder(scheme, options, k8sClient),
		NewServiceBuilder(scheme),
		NewPVCBuilder(scheme),
		NewAccessResourcesBuilder(),
		statusManager,
	)

	// Create state machine
	templateResolver := NewTemplateResolver(k8sClient)
	eventRecorder := mgr.GetEventRecorderFor("workspace-controller")
	stateMachine := NewStateMachine(resourceManager, statusManager, templateResolver, eventRecorder)

	// Create pod event handler
	podEventHandler := NewPodEventHandler(k8sClient, resourceManager)

	// Create reconciler with dependencies
	reconciler := &WorkspaceReconciler{
		Client:          k8sClient,
		Scheme:          scheme,
		stateMachine:    stateMachine,
		statusManager:   statusManager,
		podEventHandler: podEventHandler,
		options:         options,
	}

	return reconciler.SetupWithManager(mgr)
}

// getWorkspace retrieves the Workspace resource
func (r *WorkspaceReconciler) getWorkspace(ctx context.Context, req ctrl.Request) (*workspacesv1alpha1.Workspace, error) {
	workspace := &workspacesv1alpha1.Workspace{}
	err := r.Get(ctx, req.NamespacedName, workspace)
	return workspace, err
}
