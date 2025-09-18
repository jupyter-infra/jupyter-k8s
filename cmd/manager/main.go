// entry point for the jupyter-k8s operator manager
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	serversv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	// Add Kubernetes core schemes
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Add custom resource schemes
	utilruntime.Must(serversv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var applicationImagesPullPolicy string
	var applicationImagesRegistry string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&applicationImagesPullPolicy, "application-images-pull-policy", "",
		"Image pull policy for Application containers (Always, IfNotPresent, or Never)")
	flag.StringVar(&applicationImagesRegistry, "application-images-registry", "",
		"Registry prefix for application images (e.g. example.com/my-registry)")
	flag.Parse()

	// Setup logger
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	fmt.Println("Starting Jupyter K8s Controller")

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Printf("Error getting kubeconfig: %v\n", err)
		os.Exit(1)
	}

	// Create a new manager
	mgr, err := manager.New(cfg, manager.Options{
		Scheme:         scheme,
		LeaderElection: enableLeaderElection,
		// TODO: Add hash/random suffix to LeaderElectionID to prevent conflicts
		// Other operators use patterns like "jupyter-k8s-controller-<hash>" to ensure
		// uniqueness when multiple operators might be deployed in the same cluster
		LeaderElectionID: "jupyter-k8s-controller",
	})
	if err != nil {
		fmt.Printf("Error creating manager: %v\n", err)
		os.Exit(1)
	}

	// Configure controller options
	controllerOpts := controller.JupyterServerControllerOptions{
		ApplicationImagesPullPolicy: getImagePullPolicy(applicationImagesPullPolicy),
		ApplicationImagesRegistry:   applicationImagesRegistry,
	}

	// Setup controllers
	if err = controller.SetupJupyterServerController(mgr, controllerOpts); err != nil {
		fmt.Printf("Error setting up controller: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		fmt.Printf("Error running manager: %v\n", err)
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
