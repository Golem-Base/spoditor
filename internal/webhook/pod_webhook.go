package webhook

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/spoditor/spoditor/internal/annotation"
	"github.com/spoditor/spoditor/internal/identifier"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.spoditor.io,admissionReviewVersions={v1,v1beta1}

var log = logf.Log.WithName("pod_webhook")

// PodArgumentor receives the admission request from API server when a Pod resource
// is created or updated. It applies mutations to StatefulSet pods based on
// annotations that define per-ordinal configurations.
type PodArgumentor struct {
	decoder   *admission.Decoder
	ssPodId   identifier.SSPodIdentifier
	handlers  []annotation.Handler
	collector annotation.QualifiedAnnotationCollector
}

// NewPodArgumentor creates a new PodArgumentor with the specified StatefulSet pod identifier
// and annotation collector.
func NewPodArgumentor(podId identifier.SSPodIdentifier, collector annotation.QualifiedAnnotationCollector) *PodArgumentor {
	return &PodArgumentor{
		ssPodId:   podId,
		collector: collector,
		handlers:  make([]annotation.Handler, 0),
	}
}

// Handle processes the admission request, applying mutations to StatefulSet pods
// based on registered handlers and annotations.
func (r *PodArgumentor) Handle(ctx context.Context, request admission.Request) admission.Response {
	logger := log.WithValues(
		"namespace", request.Namespace,
		"name", request.Name,
		"operation", request.Operation,
	)
	logger.Info("Processing admission request")

	// Decode the pod from the request
	pod := &v1.Pod{}
	if err := r.decoder.Decode(request, pod); err != nil {
		logger.Error(err, "Failed to decode pod")
		return admission.Allowed(fmt.Sprintf("failed to decode the input pod: %v", err))
	}

	logger = logger.WithValues("pod", pod.Name)
	logger.Info("Start handling pod")

	// Check if this is a StatefulSet pod and extract information
	ss, ordinal, err := r.ssPodId.Extract(pod)
	if err != nil {
		logger.Info("Not a StatefulSet pod, allowing without mutation", "error", err)
		return admission.Allowed(fmt.Sprintf("ignore non-statefulset pod: %v", err))
	}

	logger = logger.WithValues("statefulset", ss, "ordinal", ordinal)
	logger.Info("Found StatefulSet pod")

	// Apply all registered handlers
	if err := r.applyHandlers(pod, ordinal, logger); err != nil {
		logger.Error(err, "Failed to apply handlers")
		return admission.Allowed(fmt.Sprintf("mutation failed: %v", err))
	}

	// Create the patch response
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		logger.Error(err, "Failed to marshal mutated pod")
		return admission.Allowed(fmt.Sprintf("failed to marshal the mutated pod: %v", err))
	}

	logger.Info("Successfully processed pod")
	return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
}

// applyHandlers processes all registered handlers against the pod
func (r *PodArgumentor) applyHandlers(pod *v1.Pod, ordinal int, logger logr.Logger) error {
	// Collect annotations once for all handlers
	annotations := r.collector.Collect(pod)

	for i, handler := range r.handlers {
		handlerLogger := logger.WithValues("handlerIndex", i, "handlerType", fmt.Sprintf("%T", handler))

		// Parse the configuration for this handler
		config, err := handler.GetParser().Parse(annotations)
		if err != nil {
			handlerLogger.Error(err, "Failed to parse configuration")
			return fmt.Errorf("handler %T at index %d: parse error: %w", handler, i, err)
		}

		// Skip if no configuration was found for this handler
		if config == nil {
			handlerLogger.Info("No configuration found for handler, skipping")
			continue
		}

		handlerLogger.Info("Parsed argumentation configuration", "config", config)
		if err := handler.Mutate(&pod.Spec, ordinal, config); err != nil {
			handlerLogger.Error(err, "Handler failed to mutate pod")
			return fmt.Errorf("handler %d: mutation error: %w", i, err)
		}

		handlerLogger.Info("Successfully applied handler")
	}

	return nil
}

// InjectDecoder implements the admission.DecoderInjector interface
func (r *PodArgumentor) InjectDecoder(decoder *admission.Decoder) error {
	r.decoder = decoder
	return nil
}

// SetupWebhookWithManager registers the webhook with the controller manager
func (r *PodArgumentor) SetupWebhookWithManager(mgr ctrl.Manager) {
	log.Info("Registering pod argumentor webhook")
	mgr.GetWebhookServer().Register(
		"/mutate-v1-pod",
		&webhook.Admission{Handler: r},
	)
}

// Register adds a new handler to the webhook processor
func (r *PodArgumentor) Register(handler annotation.Handler) {
	if handler == nil {
		log.Error(nil, "Attempted to register nil handler, ignoring")
		return
	}

	log.Info("Registering new handler", "handlerType", fmt.Sprintf("%T", handler))
	r.handlers = append(r.handlers, handler)
}
