package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	prismv1alpha1 "github.com/prism/prism-operator/api/v1alpha1"
	"github.com/prism/prism-operator/pkg/autoscaling"
)

// PrismPatternReconciler reconciles a PrismPattern object
type PrismPatternReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Auto-scaling reconcilers
	HPAReconciler  *autoscaling.HPAReconciler
	KEDAReconciler *autoscaling.KEDAReconciler
}

//+kubebuilder:rbac:groups=prism.io,resources=prismpatterns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=prism.io,resources=prismpatterns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=prism.io,resources=prismpatterns/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keda.sh,resources=scaledobjects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keda.sh,resources=scaledobjects/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop
func (r *PrismPatternReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the PrismPattern instance
	pattern := &prismv1alpha1.PrismPattern{}
	if err := r.Get(ctx, req.NamespacedName, pattern); err != nil {
		if errors.IsNotFound(err) {
			log.Info("PrismPattern resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get PrismPattern")
		return ctrl.Result{}, err
	}

	log.Info("Reconciling PrismPattern", "name", pattern.Name, "namespace", pattern.Namespace)

	// Reconcile deployment
	if err := r.reconcileDeployment(ctx, pattern); err != nil {
		log.Error(err, "Failed to reconcile deployment")
		return ctrl.Result{}, err
	}

	// Reconcile service
	if err := r.reconcileService(ctx, pattern); err != nil {
		log.Error(err, "Failed to reconcile service")
		return ctrl.Result{}, err
	}

	// Reconcile auto-scaling
	if err := r.reconcileAutoscaling(ctx, pattern); err != nil {
		log.Error(err, "Failed to reconcile auto-scaling")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.updateStatus(ctx, pattern); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled PrismPattern", "name", pattern.Name)
	return ctrl.Result{}, nil
}

// reconcileDeployment creates or updates the deployment for the pattern
func (r *PrismPatternReconciler) reconcileDeployment(ctx context.Context, pattern *prismv1alpha1.PrismPattern) error {
	log := log.FromContext(ctx)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pattern.Name,
			Namespace: pattern.Namespace,
			Labels:    r.labelsForPattern(pattern),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: r.getInitialReplicas(pattern),
			Selector: &metav1.LabelSelector{
				MatchLabels: r.labelsForPattern(pattern),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.labelsForPattern(pattern),
				},
				Spec: r.buildPodSpec(pattern),
			},
		},
	}

	// Set PrismPattern as owner
	if err := ctrl.SetControllerReference(pattern, deployment, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if deployment exists
	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create new deployment
		log.Info("Creating new deployment", "name", deployment.Name)
		if err := r.Create(ctx, deployment); err != nil {
			return fmt.Errorf("failed to create deployment: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Update existing deployment (but preserve replicas if auto-scaling is enabled)
	log.Info("Updating existing deployment", "name", deployment.Name)
	found.Spec.Template = deployment.Spec.Template
	found.Spec.Selector = deployment.Spec.Selector

	// Only update replicas if auto-scaling is not enabled
	if pattern.Spec.Autoscaling == nil || !pattern.Spec.Autoscaling.Enabled {
		found.Spec.Replicas = deployment.Spec.Replicas
	}

	if err := r.Update(ctx, found); err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
}

// reconcileService creates or updates the service for the pattern
func (r *PrismPatternReconciler) reconcileService(ctx context.Context, pattern *prismv1alpha1.PrismPattern) error {
	log := log.FromContext(ctx)

	if pattern.Spec.Service == nil {
		// No service configuration, skip
		return nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pattern.Name,
			Namespace: pattern.Namespace,
			Labels:    r.labelsForPattern(pattern),
		},
		Spec: corev1.ServiceSpec{
			Selector: r.labelsForPattern(pattern),
			Type:     pattern.Spec.Service.Type,
			Ports: []corev1.ServicePort{
				{
					Name:       "grpc",
					Port:       pattern.Spec.Service.Port,
					TargetPort: intstr.FromInt(int(pattern.Spec.Service.Port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Set PrismPattern as owner
	if err := ctrl.SetControllerReference(pattern, service, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if service exists
	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create new service
		log.Info("Creating new service", "name", service.Name)
		if err := r.Create(ctx, service); err != nil {
			return fmt.Errorf("failed to create service: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	// Update existing service
	log.Info("Updating existing service", "name", service.Name)
	found.Spec.Ports = service.Spec.Ports
	found.Spec.Type = service.Spec.Type
	found.Spec.Selector = service.Spec.Selector

	if err := r.Update(ctx, found); err != nil {
		return fmt.Errorf("failed to update service: %w", err)
	}

	return nil
}

// reconcileAutoscaling handles both HPA and KEDA auto-scaling
func (r *PrismPatternReconciler) reconcileAutoscaling(ctx context.Context, pattern *prismv1alpha1.PrismPattern) error {
	log := log.FromContext(ctx)

	if pattern.Spec.Autoscaling == nil || !pattern.Spec.Autoscaling.Enabled {
		// Auto-scaling disabled, ensure both HPA and KEDA are cleaned up
		log.Info("Auto-scaling disabled, cleaning up", "name", pattern.Name)

		// Clean up HPA
		if r.HPAReconciler != nil {
			if err := r.HPAReconciler.ReconcileHPA(
				ctx,
				pattern.Name,
				pattern.Namespace,
				nil, // nil autoscaling spec will trigger deletion
				autoscalingv2.CrossVersionObjectReference{},
				nil,
				pattern,
			); err != nil {
				return fmt.Errorf("failed to clean up HPA: %w", err)
			}
		}

		// Clean up KEDA
		if r.KEDAReconciler != nil {
			if err := r.KEDAReconciler.ReconcileKEDA(
				ctx,
				pattern.Name,
				pattern.Namespace,
				nil, // nil autoscaling spec will trigger deletion
				nil,
				pattern,
			); err != nil {
				return fmt.Errorf("failed to clean up KEDA: %w", err)
			}
		}

		return nil
	}

	// Determine scaler type (default to HPA)
	scaler := pattern.Spec.Autoscaling.Scaler
	if scaler == "" {
		scaler = "hpa"
	}

	targetRef := autoscalingv2.CrossVersionObjectReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       pattern.Name,
	}

	labels := r.labelsForPattern(pattern)

	switch scaler {
	case "hpa":
		log.Info("Reconciling HPA auto-scaling", "name", pattern.Name)
		if r.HPAReconciler != nil {
			if err := r.HPAReconciler.ReconcileHPA(
				ctx,
				pattern.Name,
				pattern.Namespace,
				pattern.Spec.Autoscaling,
				targetRef,
				labels,
				pattern,
			); err != nil {
				return fmt.Errorf("failed to reconcile HPA: %w", err)
			}
		}

		// Clean up KEDA if switching from KEDA to HPA
		if r.KEDAReconciler != nil {
			if err := r.KEDAReconciler.ReconcileKEDA(
				ctx,
				pattern.Name,
				pattern.Namespace,
				nil,
				nil,
				pattern,
			); err != nil {
				return fmt.Errorf("failed to clean up KEDA: %w", err)
			}
		}

	case "keda":
		log.Info("Reconciling KEDA auto-scaling", "name", pattern.Name)
		if r.KEDAReconciler != nil {
			if err := r.KEDAReconciler.ReconcileKEDA(
				ctx,
				pattern.Name,
				pattern.Namespace,
				pattern.Spec.Autoscaling,
				labels,
				pattern,
			); err != nil {
				return fmt.Errorf("failed to reconcile KEDA: %w", err)
			}
		}

		// Clean up HPA if switching from HPA to KEDA
		if r.HPAReconciler != nil {
			if err := r.HPAReconciler.ReconcileHPA(
				ctx,
				pattern.Name,
				pattern.Namespace,
				nil,
				autoscalingv2.CrossVersionObjectReference{},
				nil,
				pattern,
			); err != nil {
				return fmt.Errorf("failed to clean up HPA: %w", err)
			}
		}

	default:
		return fmt.Errorf("unknown scaler type: %s (must be 'hpa' or 'keda')", scaler)
	}

	return nil
}

// buildPodSpec constructs the pod spec for the pattern deployment
func (r *PrismPatternReconciler) buildPodSpec(pattern *prismv1alpha1.PrismPattern) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  fmt.Sprintf("%s-runner", pattern.Spec.Pattern),
				Image: pattern.Spec.Image,
				Args: []string{
					fmt.Sprintf("--pattern=%s", pattern.Spec.Pattern),
					fmt.Sprintf("--backend=%s", pattern.Spec.Backend),
				},
				Resources: pattern.Spec.Resources,
			},
		},
	}

	// Add service port if configured
	if pattern.Spec.Service != nil {
		podSpec.Containers[0].Ports = []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: pattern.Spec.Service.Port,
				Protocol:      corev1.ProtocolTCP,
			},
		}
	}

	// Apply placement configuration if specified
	if pattern.Spec.Placement != nil {
		podSpec.NodeSelector = pattern.Spec.Placement.NodeSelector
		podSpec.Affinity = pattern.Spec.Placement.Affinity
		podSpec.Tolerations = pattern.Spec.Placement.Tolerations
		podSpec.TopologySpreadConstraints = pattern.Spec.Placement.TopologySpreadConstraints
		podSpec.PriorityClassName = pattern.Spec.Placement.PriorityClassName
		podSpec.RuntimeClassName = pattern.Spec.Placement.RuntimeClassName
	}

	return podSpec
}

// getInitialReplicas returns the initial replica count
func (r *PrismPatternReconciler) getInitialReplicas(pattern *prismv1alpha1.PrismPattern) *int32 {
	// If auto-scaling is enabled, use minReplicas as initial
	if pattern.Spec.Autoscaling != nil && pattern.Spec.Autoscaling.Enabled {
		return &pattern.Spec.Autoscaling.MinReplicas
	}

	// Otherwise use the spec replicas
	return &pattern.Spec.Replicas
}

// updateStatus updates the PrismPattern status
func (r *PrismPatternReconciler) updateStatus(ctx context.Context, pattern *prismv1alpha1.PrismPattern) error {
	log := log.FromContext(ctx)

	// Get deployment status
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pattern.Name, Namespace: pattern.Namespace}, deployment); err != nil {
		if errors.IsNotFound(err) {
			// Deployment not created yet
			pattern.Status.Phase = "Pending"
			pattern.Status.Replicas = 0
			pattern.Status.AvailableReplicas = 0
		} else {
			return fmt.Errorf("failed to get deployment for status: %w", err)
		}
	} else {
		// Update status from deployment
		pattern.Status.Replicas = deployment.Status.Replicas
		pattern.Status.AvailableReplicas = deployment.Status.AvailableReplicas
		pattern.Status.ObservedGeneration = deployment.Status.ObservedGeneration

		// Determine phase
		if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas > 0 {
			if deployment.Status.AvailableReplicas == *deployment.Spec.Replicas {
				pattern.Status.Phase = "Running"
			} else if deployment.Status.Replicas > 0 {
				pattern.Status.Phase = "Progressing"
			} else {
				pattern.Status.Phase = "Pending"
			}
		} else {
			pattern.Status.Phase = "Pending"
		}
	}

	// Update conditions based on phase
	readyCondition := metav1.Condition{
		Type:               "Ready",
		ObservedGeneration: pattern.Generation,
		LastTransitionTime: metav1.Now(),
	}

	switch pattern.Status.Phase {
	case "Running":
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "DeploymentReady"
		readyCondition.Message = fmt.Sprintf("All %d replicas are available", pattern.Status.AvailableReplicas)
	case "Progressing":
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "DeploymentProgressing"
		readyCondition.Message = fmt.Sprintf("%d/%d replicas available", pattern.Status.AvailableReplicas, pattern.Status.Replicas)
	default:
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "DeploymentPending"
		readyCondition.Message = "Waiting for deployment to start"
	}

	pattern.Status.Conditions = []metav1.Condition{readyCondition}

	// Update status subresource
	if err := r.Status().Update(ctx, pattern); err != nil {
		log.Error(err, "Failed to update status subresource")
		return fmt.Errorf("failed to update status: %w", err)
	}

	log.V(1).Info("Updated status", "phase", pattern.Status.Phase, "replicas", pattern.Status.Replicas, "available", pattern.Status.AvailableReplicas)
	return nil
}

// labelsForPattern returns the labels for a pattern
func (r *PrismPatternReconciler) labelsForPattern(pattern *prismv1alpha1.PrismPattern) map[string]string {
	return map[string]string{
		"app":                       "prism",
		"prism.io/pattern":          pattern.Spec.Pattern,
		"prism.io/backend":          pattern.Spec.Backend,
		"prism.io/pattern-instance": pattern.Name,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PrismPatternReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize auto-scaling reconcilers
	r.HPAReconciler = &autoscaling.HPAReconciler{
		Client: r.Client,
		Scheme: r.Scheme,
	}

	r.KEDAReconciler = &autoscaling.KEDAReconciler{
		Client: r.Client,
		Scheme: r.Scheme,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&prismv1alpha1.PrismPattern{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Complete(r)
}
