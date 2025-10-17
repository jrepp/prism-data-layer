package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrismPatternSpec defines the desired state of PrismPattern
type PrismPatternSpec struct {
	// Pattern type (keyvalue, pubsub, consumer, producer, etc.)
	Pattern string `json:"pattern"`

	// Backend to use
	Backend string `json:"backend"`

	// Image for the pattern runner
	Image string `json:"image"`

	// Number of replicas (when auto-scaling disabled)
	Replicas int32 `json:"replicas"`

	// Resource requirements
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Backend configuration reference
	BackendConfig *BackendConfigRef `json:"backendConfig,omitempty"`

	// Pattern-specific configuration
	Config map[string]string `json:"config,omitempty"`

	// Service exposure configuration
	Service *PatternServiceSpec `json:"service,omitempty"`

	// Auto-scaling configuration
	Autoscaling *AutoscalingSpec `json:"autoscaling,omitempty"`

	// Placement configuration
	Placement *PlacementSpec `json:"placement,omitempty"`
}

// BackendConfigRef references backend configuration
type BackendConfigRef struct {
	// Name of the backend config
	Name string `json:"name"`

	// Namespace of the backend config
	Namespace string `json:"namespace,omitempty"`
}

// PatternServiceSpec defines service configuration for a pattern
type PatternServiceSpec struct {
	// Service type
	Type corev1.ServiceType `json:"type,omitempty"`

	// Port
	Port int32 `json:"port"`
}

// PrismPatternStatus defines the observed state of PrismPattern
type PrismPatternStatus struct {
	// Phase of the pattern
	Phase string `json:"phase,omitempty"`

	// Number of replicas
	Replicas int32 `json:"replicas,omitempty"`

	// Available replicas
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced,shortName=ppattern
//+kubebuilder:printcolumn:name="Pattern",type=string,JSONPath=`.spec.pattern`
//+kubebuilder:printcolumn:name="Backend",type=string,JSONPath=`.spec.backend`
//+kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`
//+kubebuilder:printcolumn:name="Available",type=integer,JSONPath=`.status.availableReplicas`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PrismPattern is the Schema for the prismpatterns API
type PrismPattern struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrismPatternSpec   `json:"spec,omitempty"`
	Status PrismPatternStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PrismPatternList contains a list of PrismPattern
type PrismPatternList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrismPattern `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrismPattern{}, &PrismPatternList{})
}
