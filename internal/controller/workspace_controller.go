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
	"strings"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	builderPkg "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	mngr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	// DefaultTemplateNamespace is the default namespace for WorkspaceTemplate resolution
	// when templateRef.namespace is not specified
	DefaultTemplateNamespace string
}

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	stateMachine    StateMachineInterface
	statusManager   *StatusManager
	podEventHandler *PodEventHandler
	options         WorkspaceControllerOptions
}

// SetStateMachine sets the state machine for testing purposes
func (r *WorkspaceReconciler) SetStateMachine(sm StateMachineInterface) {
	r.stateMachine = sm
}

// +kubebuilder:rbac:groups=workspace.jupyter.org,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workspace.jupyter.org,resources=workspaces/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods;serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=traefik.io,resources=ingressroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=traefik.io,resources=middlewares,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
//
// nolint:gocyclo
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

	// Handle deletion if DeletionTimestamp is set
	if !workspace.DeletionTimestamp.IsZero() {
		return r.stateMachine.ReconcileDeletion(ctx, workspace)
	}

	// Consolidated function to ensure labels are set correctly
	// and perform at most one update
	needsUpdate := false
	finalizerAdded := false
	labelsChanged := map[string]string{}
	labelsRemoved := []string{}

	// Add finalizer if missing (for new workspaces)
	if !controllerutil.ContainsFinalizer(workspace, WorkspaceFinalizerName) {
		logger.Info("Adding finalizer to workspace")
		controllerutil.AddFinalizer(workspace, WorkspaceFinalizerName)
		needsUpdate = true
		finalizerAdded = true
	}

	// Initialize labels map if needed
	if workspace.Labels == nil {
		workspace.Labels = make(map[string]string)
	}

	// Handle template labels
	if workspace.Spec.TemplateRef != nil && workspace.Spec.TemplateRef.Name != "" {
		// Template is referenced - ensure both labels are set
		templateName := workspace.Spec.TemplateRef.Name
		templateNamespace := workspaceutil.GetTemplateRefNamespace(workspace)

		if workspace.Labels[workspaceutil.LabelWorkspaceTemplate] != templateName {
			workspace.Labels[workspaceutil.LabelWorkspaceTemplate] = templateName
			labelsChanged[workspaceutil.LabelWorkspaceTemplate] = templateName
			needsUpdate = true
		}
		if workspace.Labels[workspaceutil.LabelWorkspaceTemplateNamespace] != templateNamespace {
			workspace.Labels[workspaceutil.LabelWorkspaceTemplateNamespace] = templateNamespace
			labelsChanged[workspaceutil.LabelWorkspaceTemplateNamespace] = templateNamespace
			needsUpdate = true
		}
	} else {
		// Template is not referenced - ensure labels are removed
		if _, hasTemplateLabel := workspace.Labels[workspaceutil.LabelWorkspaceTemplate]; hasTemplateLabel {
			delete(workspace.Labels, workspaceutil.LabelWorkspaceTemplate)
			labelsRemoved = append(labelsRemoved, workspaceutil.LabelWorkspaceTemplate)
			needsUpdate = true
		}
		if _, hasNamespaceLabel := workspace.Labels[workspaceutil.LabelWorkspaceTemplateNamespace]; hasNamespaceLabel {
			delete(workspace.Labels, workspaceutil.LabelWorkspaceTemplateNamespace)
			labelsRemoved = append(labelsRemoved, workspaceutil.LabelWorkspaceTemplateNamespace)
			needsUpdate = true
		}
	}

	// Handle AccessStrategy labels
	if workspace.Spec.AccessStrategy != nil && workspace.Spec.AccessStrategy.Name != "" {
		// AccessStrategy is referenced - ensure both labels are set
		accessStrategyName := workspace.Spec.AccessStrategy.Name
		accessStrategyNamespace := workspace.Namespace
		if workspace.Spec.AccessStrategy.Namespace != "" {
			accessStrategyNamespace = workspace.Spec.AccessStrategy.Namespace
		}

		if workspace.Labels[LabelAccessStrategyName] != accessStrategyName {
			workspace.Labels[LabelAccessStrategyName] = accessStrategyName
			labelsChanged[LabelAccessStrategyName] = accessStrategyName
			needsUpdate = true
		}
		if workspace.Labels[LabelAccessStrategyNamespace] != accessStrategyNamespace {
			workspace.Labels[LabelAccessStrategyNamespace] = accessStrategyNamespace
			labelsChanged[LabelAccessStrategyNamespace] = accessStrategyNamespace
			needsUpdate = true
		}
	} else {
		// AccessStrategy is not referenced - ensure labels are removed
		if _, hasAccessStrategyLabel := workspace.Labels[LabelAccessStrategyName]; hasAccessStrategyLabel {
			delete(workspace.Labels, LabelAccessStrategyName)
			labelsRemoved = append(labelsRemoved, LabelAccessStrategyName)
			needsUpdate = true
		}
		if _, hasNamespaceLabel := workspace.Labels[LabelAccessStrategyNamespace]; hasNamespaceLabel {
			delete(workspace.Labels, LabelAccessStrategyNamespace)
			labelsRemoved = append(labelsRemoved, LabelAccessStrategyNamespace)
			needsUpdate = true
		}
	}

	// Perform a single update if any labels or finalizer have changed
	if needsUpdate {
		logger.Info("Updating workspace labels",
			"finalizerAdded", finalizerAdded,
			"labelsChanged", labelsChanged,
			"labelsRemoved", labelsRemoved,
		)

		if err := r.Update(ctx, workspace); err != nil {
			logger.Error(err, "Failed to update workspace labels or finalizers")
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		logger.Info("Successfully updated workspace labels or finalizers")
		// Requeue to process with updated labels and/or finalizer
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
	}

	// Get desired status to decide if we need to fetch AccessStrategy
	desiredStatus := r.stateMachine.getDesiredStatus(workspace)

	// Only fetch AccessStrategy if desiredStatus is not Stopped and workspace has AccessStrategy defined
	var accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
	if desiredStatus != DesiredStateStopped && workspace.Spec.AccessStrategy != nil {
		accessStrategy, err = r.stateMachine.GetAccessStrategyForWorkspace(ctx, workspace)
		if err != nil {
			logger.Error(err, "Failed to get AccessStrategy")
			return ctrl.Result{}, err
		}
	}

	// Delegate to state machine for business logic, passing the accessStrategy
	result, err := r.stateMachine.ReconcileDesiredState(ctx, workspace, accessStrategy)
	if err != nil {
		logger.Error(err, "Failed to reconcile desired state")
		return ctrl.Result{}, err
	}

	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&workspacev1alpha1.Workspace{}).
		Named("workspace").
		// Watch for standard Kubernetes resources
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{})

	// Watch for changes to AccessStrategy resources to trigger reconciliation
	// of Workspaces that reference them
	builder.Watches(
		&workspacev1alpha1.WorkspaceAccessStrategy{},
		handler.EnqueueRequestsFromMapFunc(r.accessStrategyEventHandler),
	)

	// Conditionally watch pods based on configuration
	if r.options.EnableWorkspacePodWatching {
		builder.Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.podEventHandler.HandleWorkspacePodEvents),
			builderPkg.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				// Only watch pods with workspace labels
				_, hasWorkspace := obj.GetLabels()[workspaceutil.LabelWorkspaceName]
				return hasWorkspace
			})),
		)

		// Also watch Events to detect preemption
		builder.Watches(
			&corev1.Event{},
			handler.EnqueueRequestsFromMapFunc(r.podEventHandler.HandleKubernetesEvents),
			builderPkg.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				// Only watch preemption-related events to avoid processing all events
				event, ok := obj.(*corev1.Event)
				if !ok {
					return false
				}
				// Handle both workload preemption events and pod stopped events due to preemption
				return (event.InvolvedObject.Kind == KindPod &&
					event.Reason == "Stopped" &&
					strings.Contains(event.Message, "Preempted")) ||
					(event.Reason == "Preempted")
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

		// Watch NetworkPolicy resources using typed API
		builder.Owns(&networkingv1.NetworkPolicy{}).Owns(ingressRouteGVK).Owns(middlewareGVK)
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
	eventRecorder := mgr.GetEventRecorderFor("workspace-controller")
	idleChecker := NewWorkspaceIdleChecker(k8sClient)
	stateMachine := NewStateMachine(resourceManager, statusManager, eventRecorder, idleChecker)

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
func (r *WorkspaceReconciler) getWorkspace(ctx context.Context, req ctrl.Request) (*workspacev1alpha1.Workspace, error) {
	workspace := &workspacev1alpha1.Workspace{}
	err := r.Get(ctx, req.NamespacedName, workspace)
	return workspace, err
}

// accessStrategyEventHandler maps AccessStrategy events to Workspace reconciliation requests
func (r *WorkspaceReconciler) accessStrategyEventHandler(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := logf.FromContext(ctx)
	accessStrategy, ok := obj.(*workspacev1alpha1.WorkspaceAccessStrategy)
	if !ok {
		// Not an AccessStrategy
		return nil
	}

	logger.Info("Handling AccessStrategy event",
		"accessStrategy", accessStrategy.Name,
		"namespace", accessStrategy.Namespace)

	// Find all Workspaces that reference this AccessStrategy
	requests, listErr := workspaceutil.GetWorkspaceReconciliationRequestsForAccessStrategy(
		ctx,
		r.Client,
		accessStrategy.Name,
		accessStrategy.Namespace)
	if listErr != nil {
		logger.Error(
			listErr,
			"Failed to list Workspaces associated with access strategy",
			"accessStrategy", accessStrategy.Name,
			"namespace", accessStrategy.Namespace)
		return nil
	}

	if len(requests) > 0 {
		logger.Info("Found active workspaces referencing access strategy",
			"accessStrategy", accessStrategy.Name,
			"namespace", accessStrategy.Namespace,
			"workspaceCount", len(requests))
	} else {
		logger.Info("Found no active workspace referencing access strategy",
			"accessStrategy", accessStrategy.Name,
			"namespace", accessStrategy.Namespace)
	}

	return requests
}
