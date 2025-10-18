package autoscaling

import (
	"context"
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	prismv1alpha1 "github.com/prism/prism-operator/api/v1alpha1"
)

// HPAReconciler reconciles HorizontalPodAutoscaler resources for Prism components
type HPAReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ReconcileHPA creates or updates an HPA for the given deployment
func (r *HPAReconciler) ReconcileHPA(
	ctx context.Context,
	name, namespace string,
	autoscaling *prismv1alpha1.AutoscalingSpec,
	targetRef autoscalingv2.CrossVersionObjectReference,
	labels map[string]string,
	owner metav1.Object,
) error {
	log := log.FromContext(ctx)

	if autoscaling == nil || !autoscaling.Enabled {
		// Auto-scaling disabled, delete HPA if exists
		return r.deleteHPA(ctx, name, namespace)
	}

	if autoscaling.Scaler != "hpa" && autoscaling.Scaler != "" {
		// Not using HPA scaler
		return nil
	}

	log.Info("Reconciling HPA", "name", name, "namespace", namespace)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: targetRef,
			MinReplicas:    &autoscaling.MinReplicas,
			MaxReplicas:    autoscaling.MaxReplicas,
			Metrics:        r.buildMetrics(autoscaling),
			Behavior:       autoscaling.Behavior,
		},
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(owner, hpa, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if HPA already exists
	found := &autoscalingv2.HorizontalPodAutoscaler{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create new HPA
		log.Info("Creating new HPA", "name", name)
		if err := r.Create(ctx, hpa); err != nil {
			return fmt.Errorf("failed to create HPA: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get HPA: %w", err)
	}

	// Update existing HPA
	log.Info("Updating existing HPA", "name", name)
	found.Spec = hpa.Spec
	if err := r.Update(ctx, found); err != nil {
		return fmt.Errorf("failed to update HPA: %w", err)
	}

	return nil
}

// buildMetrics constructs the metrics for HPA
func (r *HPAReconciler) buildMetrics(autoscaling *prismv1alpha1.AutoscalingSpec) []autoscalingv2.MetricSpec {
	metrics := []autoscalingv2.MetricSpec{}

	// Add CPU metric if specified
	if autoscaling.TargetCPUUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: autoscaling.TargetCPUUtilizationPercentage,
				},
			},
		})
	}

	// Add memory metric if specified
	if autoscaling.TargetMemoryUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: autoscaling.TargetMemoryUtilizationPercentage,
				},
			},
		})
	}

	// Add custom metrics
	if len(autoscaling.Metrics) > 0 {
		metrics = append(metrics, autoscaling.Metrics...)
	}

	return metrics
}

// deleteHPA deletes an HPA if it exists
func (r *HPAReconciler) deleteHPA(ctx context.Context, name, namespace string) error {
	log := log.FromContext(ctx)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hpa)
	if err != nil {
		if errors.IsNotFound(err) {
			// HPA doesn't exist, nothing to do
			return nil
		}
		return fmt.Errorf("failed to get HPA for deletion: %w", err)
	}

	log.Info("Deleting HPA", "name", name)
	if err := r.Delete(ctx, hpa); err != nil {
		return fmt.Errorf("failed to delete HPA: %w", err)
	}

	return nil
}

// GetHPAStatus returns the current status of an HPA
func (r *HPAReconciler) GetHPAStatus(ctx context.Context, name, namespace string) (*autoscalingv2.HorizontalPodAutoscalerStatus, error) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hpa)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get HPA status: %w", err)
	}

	return &hpa.Status, nil
}
