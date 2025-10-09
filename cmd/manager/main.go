// entry point for the jupyter-k8s operator manager
package main

import (
	"flag"
	"net/http"
	"os"
	"strings"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
	workspacemutator "github.com/jupyter-ai-contrib/jupyter-k8s/internal/webhook"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	// Add Kubernetes core schemes
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Add custom resource schemes
	utilruntime.Must(workspacesv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var applicationImagesPullPolicy string
	var applicationImagesRegistry string
	var webhookPort int
	var requireTemplate bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&applicationImagesPullPolicy, "application-images-pull-policy", "",
		"Image pull policy for Application containers (Always, IfNotPresent, or Never)")
	flag.StringVar(&applicationImagesRegistry, "application-images-registry", "",
		"Registry prefix for application images (e.g. example.com/my-registry)")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "The port the webhook endpoint binds to.")
	flag.BoolVar(&requireTemplate, "require-template", false,
		"Require all workspaces to reference a WorkspaceTemplate")
	flag.Parse()

	// Setup logger
	logger := zap.New(zap.UseDevMode(true))
	log.SetLogger(logger)
	setupLog := log.Log.WithName("setup")

	setupLog.Info("Starting Jupyter K8s Controller")

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		setupLog.Error(err, "Error getting kubeconfig")
		os.Exit(1)
	}

	// Create a new manager
	mgr, err := manager.New(cfg, manager.Options{
		Scheme:                 scheme,
		LeaderElection:         enableLeaderElection,
		HealthProbeBindAddress: probeAddr,
		// TODO: Add hash/random suffix to LeaderElectionID to prevent conflicts
		// Other operators use patterns like "jupyter-k8s-controller-<hash>" to ensure
		// uniqueness when multiple operators might be deployed in the same cluster
		LeaderElectionID: "jupyter-k8s-controller",
	})
	if err != nil {
		setupLog.Error(err, "Error creating manager")
		os.Exit(1)
	}

	// Setup health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Error setting up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Error setting up ready check")
		os.Exit(1)
	}

	// Configure controller options
	controllerOpts := controller.WorkspaceControllerOptions{
		ApplicationImagesPullPolicy: getImagePullPolicy(applicationImagesPullPolicy),
		ApplicationImagesRegistry:   applicationImagesRegistry,
	}

	// Setup controllers
	if err = controller.SetupWorkspaceController(mgr, controllerOpts); err != nil {
		setupLog.Error(err, "Error setting up workspace controller")
		os.Exit(1)
	}

	if err = controller.SetupWorkspaceTemplateController(mgr); err != nil {
		setupLog.Error(err, "Error setting up workspace template controller")
		os.Exit(1)
	}

	// Setup webhook
	setupLog.Info("Registering webhooks")
	mutator := &workspacemutator.WorkspaceMutator{}
	mgr.GetWebhookServer().Register("/mutate-workspace", &admission.Webhook{Handler: mutator})
	setupLog.Info("Registered webhook", "path", "/mutate-workspace", "type", "WorkspaceMutator")
	mgr.GetWebhookServer().Register("/webhook-health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	setupLog.Info("Registered webhook", "path", "/webhook-health", "type", "HealthCheck")
	setupLog.Info("All webhooks registered successfully")

	setupLog.Info("Starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Error running manager")
		os.Exit(1)
	}
}

// getImagePullPolicy converts a string pull policy to a Kubernetes PullPolicy
func getImagePullPolicy(policyStr string) corev1.PullPolicy {
	switch strings.ToLower(policyStr) {
	case "always":
		return corev1.PullAlways
	case "never":
		return corev1.PullNever
	case "ifnotpresent":
		return corev1.PullIfNotPresent
	default:
		// Default to IfNotPresent which is a good balance for most deployments
		return corev1.PullIfNotPresent
	}
}
