package autoscaling

import (
	"context"
	"fmt"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	prismv1alpha1 "github.com/prism/prism-operator/api/v1alpha1"
)

// KEDAReconciler reconciles KEDA ScaledObject resources for Prism components
type KEDAReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ReconcileKEDA creates or updates a KEDA ScaledObject for the given deployment
func (r *KEDAReconciler) ReconcileKEDA(
	ctx context.Context,
	name, namespace string,
	autoscaling *prismv1alpha1.AutoscalingSpec,
	labels map[string]string,
	owner metav1.Object,
) error {
	log := log.FromContext(ctx)

	if autoscaling == nil || !autoscaling.Enabled {
		// Auto-scaling disabled, delete ScaledObject if exists
		return r.deleteScaledObject(ctx, name, namespace)
	}

	if autoscaling.Scaler != "keda" {
		// Not using KEDA scaler
		return nil
	}

	if len(autoscaling.Triggers) == 0 {
		return fmt.Errorf("KEDA scaler requires at least one trigger")
	}

	log.Info("Reconciling KEDA ScaledObject", "name", name, "namespace", namespace)

	scaledObject := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       name,
			},
			MinReplicaCount: &autoscaling.MinReplicas,
			MaxReplicaCount: &autoscaling.MaxReplicas,
			PollingInterval: r.getPollingInterval(autoscaling),
			CooldownPeriod:  r.getCooldownPeriod(autoscaling),
			Triggers:        r.buildTriggers(autoscaling),
		},
	}

	// Add advanced configuration if behavior is specified
	if autoscaling.Behavior != nil {
		scaledObject.Spec.Advanced = &kedav1alpha1.AdvancedConfig{
			HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
				Behavior: autoscaling.Behavior,
			},
		}
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(owner, scaledObject, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if ScaledObject already exists
	found := &kedav1alpha1.ScaledObject{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create new ScaledObject
		log.Info("Creating new KEDA ScaledObject", "name", name)
		if err := r.Create(ctx, scaledObject); err != nil {
			return fmt.Errorf("failed to create ScaledObject: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get ScaledObject: %w", err)
	}

	// Update existing ScaledObject
	log.Info("Updating existing KEDA ScaledObject", "name", name)
	found.Spec = scaledObject.Spec
	if err := r.Update(ctx, found); err != nil {
		return fmt.Errorf("failed to update ScaledObject: %w", err)
	}

	return nil
}

// buildTriggers constructs KEDA triggers from AutoscalingSpec
func (r *KEDAReconciler) buildTriggers(autoscaling *prismv1alpha1.AutoscalingSpec) []kedav1alpha1.ScaleTriggers {
	triggers := []kedav1alpha1.ScaleTriggers{}

	for _, trigger := range autoscaling.Triggers {
		scaleTrigger := kedav1alpha1.ScaleTriggers{
			Type:     trigger.Type,
			Metadata: trigger.Metadata,
		}

		if trigger.AuthenticationRef != nil {
			scaleTrigger.AuthenticationRef = &kedav1alpha1.ScaledObjectAuthRef{
				Name: trigger.AuthenticationRef.Name,
			}
		}

		triggers = append(triggers, scaleTrigger)
	}

	return triggers
}

// getPollingInterval returns the polling interval or default (10 seconds)
func (r *KEDAReconciler) getPollingInterval(autoscaling *prismv1alpha1.AutoscalingSpec) *int32 {
	if autoscaling.PollingInterval != nil {
		return autoscaling.PollingInterval
	}
	defaultInterval := int32(10)
	return &defaultInterval
}

// getCooldownPeriod returns the cooldown period or default (300 seconds)
func (r *KEDAReconciler) getCooldownPeriod(autoscaling *prismv1alpha1.AutoscalingSpec) *int32 {
	if autoscaling.CooldownPeriod != nil {
		return autoscaling.CooldownPeriod
	}
	defaultCooldown := int32(300)
	return &defaultCooldown
}

// deleteScaledObject deletes a KEDA ScaledObject if it exists
func (r *KEDAReconciler) deleteScaledObject(ctx context.Context, name, namespace string) error {
	log := log.FromContext(ctx)

	scaledObject := &kedav1alpha1.ScaledObject{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, scaledObject)
	if err != nil {
		if errors.IsNotFound(err) {
			// ScaledObject doesn't exist, nothing to do
			return nil
		}
		return fmt.Errorf("failed to get ScaledObject for deletion: %w", err)
	}

	log.Info("Deleting KEDA ScaledObject", "name", name)
	if err := r.Delete(ctx, scaledObject); err != nil {
		return fmt.Errorf("failed to delete ScaledObject: %w", err)
	}

	return nil
}

// GetScaledObjectStatus returns the current status of a ScaledObject
func (r *KEDAReconciler) GetScaledObjectStatus(ctx context.Context, name, namespace string) (*kedav1alpha1.ScaledObjectStatus, error) {
	scaledObject := &kedav1alpha1.ScaledObject{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, scaledObject)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ScaledObject status: %w", err)
	}

	return &scaledObject.Status, nil
}
