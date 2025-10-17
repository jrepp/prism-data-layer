package v1alpha1

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrismStackSpec defines the desired state of PrismStack
type PrismStackSpec struct {
	// Proxy configuration
	Proxy ProxySpec `json:"proxy"`

	// Admin control plane configuration
	Admin AdminSpec `json:"admin"`

	// Pattern runners to provision
	Patterns []PatternSpec `json:"patterns,omitempty"`

	// Backend configurations
	Backends []BackendSpec `json:"backends,omitempty"`

	// Observability configuration
	Observability ObservabilitySpec `json:"observability,omitempty"`
}

// ProxySpec defines the proxy configuration
type ProxySpec struct {
	// Image for the proxy
	Image string `json:"image"`

	// Number of replicas (when auto-scaling disabled)
	Replicas int32 `json:"replicas"`

	// Port for the proxy gRPC server
	Port int32 `json:"port"`

	// Resource requirements
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Auto-scaling configuration
	Autoscaling *AutoscalingSpec `json:"autoscaling,omitempty"`

	// Placement configuration
	Placement *PlacementSpec `json:"placement,omitempty"`
}

// AdminSpec defines the admin control plane configuration
type AdminSpec struct {
	// Enable admin control plane
	Enabled bool `json:"enabled"`

	// Port for the admin gRPC server
	Port int32 `json:"port"`

	// Number of replicas
	Replicas int32 `json:"replicas"`

	// Placement configuration
	Placement *PlacementSpec `json:"placement,omitempty"`

	// Leader election configuration
	LeaderElection *LeaderElectionSpec `json:"leaderElection,omitempty"`

	// Service configuration
	Service *ServiceSpec `json:"service,omitempty"`
}

// AutoscalingSpec defines auto-scaling configuration
type AutoscalingSpec struct {
	// Enable auto-scaling
	Enabled bool `json:"enabled"`

	// Scaler type: "hpa" or "keda"
	Scaler string `json:"scaler,omitempty"`

	// Minimum number of replicas
	MinReplicas int32 `json:"minReplicas"`

	// Maximum number of replicas
	MaxReplicas int32 `json:"maxReplicas"`

	// Target CPU utilization percentage (for HPA)
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`

	// Target memory utilization percentage (for HPA)
	TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`

	// Custom metrics (for HPA)
	Metrics []autoscalingv2.MetricSpec `json:"metrics,omitempty"`

	// Scaling behavior (for HPA)
	Behavior *autoscalingv2.HorizontalPodAutoscalerBehavior `json:"behavior,omitempty"`

	// KEDA triggers (for KEDA scaler)
	Triggers []KEDATrigger `json:"triggers,omitempty"`

	// Polling interval for KEDA (in seconds)
	PollingInterval *int32 `json:"pollingInterval,omitempty"`

	// Cooldown period for KEDA (in seconds)
	CooldownPeriod *int32 `json:"cooldownPeriod,omitempty"`
}

// KEDATrigger defines a KEDA scaling trigger
type KEDATrigger struct {
	// Trigger type (kafka, nats-jetstream, aws-sqs-queue, etc.)
	Type string `json:"type"`

	// Trigger metadata
	Metadata map[string]string `json:"metadata"`

	// Authentication reference
	AuthenticationRef *AuthenticationRef `json:"authenticationRef,omitempty"`
}

// AuthenticationRef references a secret for authentication
type AuthenticationRef struct {
	// Name of the secret
	Name string `json:"name"`
}

// PlacementSpec defines pod placement configuration
type PlacementSpec struct {
	// Placement strategy
	Strategy string `json:"strategy,omitempty"`

	// Node selector
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Affinity rules
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Topology spread constraints
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// Priority class name
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// Runtime class name
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`
}

// LeaderElectionSpec defines leader election configuration
type LeaderElectionSpec struct {
	// Enable leader election
	Enabled bool `json:"enabled"`

	// Lease duration
	LeaseDuration string `json:"leaseDuration,omitempty"`

	// Renew deadline
	RenewDeadline string `json:"renewDeadline,omitempty"`

	// Retry period
	RetryPeriod string `json:"retryPeriod,omitempty"`
}

// ServiceSpec defines service configuration
type ServiceSpec struct {
	// Service type
	Type corev1.ServiceType `json:"type,omitempty"`

	// Port
	Port int32 `json:"port,omitempty"`

	// Annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PatternSpec defines a pattern runner configuration
type PatternSpec struct {
	// Name of the pattern
	Name string `json:"name"`

	// Pattern type
	Type string `json:"type"`

	// Backend to use
	Backend string `json:"backend"`

	// Number of replicas (when auto-scaling disabled)
	Replicas int32 `json:"replicas"`

	// Configuration
	Config map[string]string `json:"config,omitempty"`

	// Runner placement specification
	RunnerSpec *RunnerSpec `json:"runnerSpec,omitempty"`
}

// RunnerSpec defines pattern runner placement and resources
type RunnerSpec struct {
	// Node selector
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Resource requirements
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Affinity rules
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// BackendSpec defines backend configuration
type BackendSpec struct {
	// Name of the backend
	Name string `json:"name"`

	// Backend type
	Type string `json:"type"`

	// Connection string
	ConnectionString string `json:"connectionString,omitempty"`

	// Secret reference
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// SecretRef references a secret
type SecretRef struct {
	// Name of the secret
	Name string `json:"name"`

	// Namespace of the secret
	Namespace string `json:"namespace,omitempty"`
}

// ObservabilitySpec defines observability configuration
type ObservabilitySpec struct {
	// Enable observability
	Enabled bool `json:"enabled"`

	// Tracing configuration
	Tracing *TracingSpec `json:"tracing,omitempty"`

	// Metrics configuration
	Metrics *MetricsSpec `json:"metrics,omitempty"`
}

// TracingSpec defines tracing configuration
type TracingSpec struct {
	// Endpoint
	Endpoint string `json:"endpoint"`
}

// MetricsSpec defines metrics configuration
type MetricsSpec struct {
	// Port
	Port int32 `json:"port"`
}

// PrismStackStatus defines the observed state of PrismStack
type PrismStackStatus struct {
	// Phase of the stack
	Phase string `json:"phase,omitempty"`

	// Observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced,shortName=pstack
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PrismStack is the Schema for the prismstacks API
type PrismStack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrismStackSpec   `json:"spec,omitempty"`
	Status PrismStackStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PrismStackList contains a list of PrismStack
type PrismStackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrismStack `json:"items"`
}

// Temporarily disabled - needs proper deepcopy implementation
// func init() {
// 	SchemeBuilder.Register(&PrismStack{}, &PrismStackList{})
// }
