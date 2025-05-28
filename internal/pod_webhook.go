package internal

import (
	"context"
	"fmt"

	"github.com/spoditor/spoditor/internal/annotation"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.spoditor.io,admissionReviewVersions={v1,v1beta1}

var log = logf.Log.WithName("pod_webhook")

// PodArgumentor implements a Kubernetes mutating webhook for StatefulSet pods
// It processes annotations and applies modifications to pods during creation/update
type PodArgumentor struct {
	decoder   *admission.Decoder
	SSPodId   SSPodIdentifier
	handlers  []annotation.Handler
	Collector annotation.QualifiedAnnotationCollector
}

// Handle processes admission requests for Pod resources
// It identifies StatefulSet pods and applies modifications based on annotations
func (r *PodArgumentor) Handle(c context.Context, request admission.Request) admission.Response {
	pod := &v1.Pod{}
	err := r.decoder.Decode(request, pod)
	if err != nil {
		return admission.Allowed(fmt.Sprintf("failed to decode the input pod %v", err))
	}

	log.Info("start handling pod", "pod", pod)
	ss, ordinal, err := r.SSPodId.Extract(pod)
	if err != nil {
		return admission.Allowed(fmt.Sprintf("ignore none-statefulset pod %v", err))
	}
	log.Info("found statefulset pod", "statefulset name", ss, "ordinal", ordinal)

	// Apply each registered handler to process annotations and modify the pod
	for _, h := range r.handlers {
		c, err := h.GetParser().Parse(r.Collector.Collect(pod))
		if err != nil || c == nil {
			return admission.Allowed(fmt.Sprintf("can't parse ssarg annotation %v", err))
		}
		log.Info("parsed argumentation configuration", "configuration", c)
		err = h.Mutate(&pod.Spec, ordinal, c)
		if err != nil {
			return admission.Allowed(fmt.Sprintf("failed to mutate the pod %v", err))
		}
	}

	// Create the patch response by comparing original and modified pod
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Allowed(fmt.Sprintf("failed to marshal the mutated pod %v", err))
	}
	return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
}

// InjectDecoder implements the DecoderInjector interface
func (r *PodArgumentor) InjectDecoder(decoder *admission.Decoder) error {
	r.decoder = decoder
	return nil
}

// SetupWebhookWithManager registers the webhook with the controller manager
func (r *PodArgumentor) SetupWebhookWithManager(mgr ctrl.Manager) {
	log.Info("registering argumentor webhook")
	mgr.GetWebhookServer().
		Register("/mutate-v1-pod", &webhook.Admission{
			Handler: r,
		})
}

// Register adds a new annotation handler to the webhook
func (r *PodArgumentor) Register(h annotation.Handler) {
	r.handlers = append(r.handlers, h)
}
