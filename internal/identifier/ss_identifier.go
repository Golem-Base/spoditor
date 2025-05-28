package identifier

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var podIdentifierLog = logf.Log.WithName("pod_identifier")

// SSPodIdentifier defines the interface for extracting StatefulSet information from pods
type SSPodIdentifier interface {
	// Extract returns the StatefulSet name and pod ordinal from the object metadata
	// Returns an error if the pod is not part of a StatefulSet or if information is missing
	Extract(accessor v1.ObjectMetaAccessor) (string, int, error)
}

// SSPodIdentifierFunc is a function type that implements SSPodIdentifier
type SSPodIdentifierFunc func(v1.ObjectMetaAccessor) (string, int, error)

// Extract implements the SSPodIdentifier interface for SSPodIdentifierFunc
func (f SSPodIdentifierFunc) Extract(accessor v1.ObjectMetaAccessor) (string, int, error) {
	return f(accessor)
}

var _ SSPodIdentifier = SSPodIdentifierFunc(nil)

// LabelSSPodIdentifier extracts StatefulSet information from pod labels
// It looks for the standard Kubernetes label "statefulset.kubernetes.io/pod-name"
// which follows the format "<statefulset-name>-<ordinal>"
var LabelSSPodIdentifier SSPodIdentifierFunc = func(accessor v1.ObjectMetaAccessor) (string, int, error) {
	l, ok := accessor.GetObjectMeta().GetLabels()["statefulset.kubernetes.io/pod-name"]
	if !ok {
		podIdentifierLog.Info("statefulset label not found", "label", "statefulset.kubernetes.io/pod-name")
		return "", -1, errors.New("missing statefulset label")
	}
	podIdentifierLog.Info("stateful pod name", "name", l)

	// Validate label format: must be "<name>-<number>"
	if b, err := regexp.MatchString(".+-\\d+", l); err != nil || !b {
		return "", -1, errors.New("unexpected label value")
	}

	// Split the pod name to extract the StatefulSet name and ordinal
	i := strings.LastIndex(l, "-")
	ordinal, _ := strconv.Atoi(l[i+1:])
	return l[:i], ordinal, nil
}
