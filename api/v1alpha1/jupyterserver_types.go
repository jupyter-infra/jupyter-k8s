package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JupyterServerSpec defines the desired state of JupyterServer
type JupyterServerSpec struct {
	// Name of the server
	Name string `json:"name"`
	
	// Image specifies the container image to use
	Image string `json:"image"`
	
	// DesiredStatus specifies the desired operational status
	// +kubebuilder:validation:Enum=Running;Stopped
	DesiredStatus string `json:"desiredStatus,omitempty"`
	
	// ServiceAccountName specifies the ServiceAccount used by the pod
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	
	// Resources specifies the resource requirements
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// JupyterServerStatus defines the observed state of JupyterServer
type JupyterServerStatus struct {
	// Phase represents the current phase of the JupyterServer
	Phase string `json:"phase,omitempty"`
	
	// Error contains error information if any
	Error string `json:"error,omitempty"`
	
	// Message contains human readable message
	Message string `json:"message,omitempty"`
	
	// DeploymentName is the name of the created deployment
	DeploymentName string `json:"deploymentName,omitempty"`
	
	// ServiceName is the name of the created service
	ServiceName string `json:"serviceName,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=jps

// JupyterServer is the Schema for the jupyterservers API
type JupyterServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JupyterServerSpec   `json:"spec,omitempty"`
	Status JupyterServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// JupyterServerList contains a list of JupyterServer
type JupyterServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []JupyterServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&JupyterServer{}, &JupyterServerList{})
}
