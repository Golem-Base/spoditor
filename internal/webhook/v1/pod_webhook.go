package v1

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/golem-base/spoditor/internal/annotation"
	"github.com/golem-base/spoditor/internal/annotation/ports"
	"github.com/golem-base/spoditor/internal/annotation/volumes"
	"github.com/golem-base/spoditor/internal/identifier"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var podlog = logf.Log.WithName("pod-webhook")

// SetupPodWebhookWithManager registers the webhook for Pod in the manager.
func SetupPodWebhookWithManager(mgr ctrl.Manager) error {
	podlog.Info("Setting up pod mutating webhook")

	// Create a new Pod mutator
	mutator := &PodMutator{
		ssPodId:   identifier.LabelSSPodIdentifier,
		collector: annotation.Collector,
		handlers: []annotation.Handler{
			&volumes.MountHandler{},
			&ports.HostPortHandler{},
		},
	}

	// Set up the webhook server
	return ctrl.NewWebhookManagedBy(mgr).
		For(&corev1.Pod{}).
		WithDefaulter(mutator).
		Complete()
}

//+kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.spoditor.io,admissionReviewVersions=v1

// PodMutator mutates Pods
type PodMutator struct {
	ssPodId   identifier.SSPodIdentifier
	handlers  []annotation.Handler
	collector annotation.QualifiedAnnotationCollector
}

var _ webhook.CustomDefaulter = &PodMutator{}

// Default implements webhook.CustomDefaulter
func (m *PodMutator) Default(ctx context.Context, obj runtime.Object) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod but got %T", obj)
	}

	l := podlog.WithValues(
		"namespace", pod.Namespace,
		"name", pod.Name,
	)
	l.Info("Processing pod")

	// Check if this is a StatefulSet pod and extract information
	ss, ordinal, err := m.ssPodId.Extract(pod)
	if err != nil {
		l.Info("Not a StatefulSet pod, skipping mutation", "error", err)
		return nil
	}

	l = l.WithValues("statefulset", ss, "ordinal", ordinal)
	l.Info("Found StatefulSet pod")

	// Apply all registered handlers
	if err := m.applyHandlers(pod, ordinal, l); err != nil {
		l.Error(err, "Failed to apply handlers")
		return err
	}

	l.Info("Successfully processed pod")
	return nil
}

// applyHandlers processes all registered handlers against the pod
func (m *PodMutator) applyHandlers(pod *corev1.Pod, ordinal int, ll logr.Logger) error {
	// Collect annotations once for all handlers
	annotations := m.collector.Collect(pod)

	for i, handler := range m.handlers {
		l := ll.WithValues("handlerIndex", i, "handlerType", fmt.Sprintf("%T", handler))

		// Parse the configuration for this handler
		config, err := handler.GetParser().Parse(annotations)
		if err != nil {
			l.Error(err, "Failed to parse configuration")
			return fmt.Errorf("handler %T at index %d: parse error: %w", handler, i, err)
		}

		// Skip if no configuration was found for this handler
		if config == nil {
			l.Info("No configuration found for handler, skipping")
			continue
		}

		l.Info("Parsed mutation configuration", "config", config)
		if err := handler.Mutate(&pod.Spec, ordinal, config); err != nil {
			l.Error(err, "Handler failed to mutate pod")
			return fmt.Errorf("handler %d: mutation error: %w", i, err)
		}

		l.Info("Successfully applied handler")
	}

	return nil
}
