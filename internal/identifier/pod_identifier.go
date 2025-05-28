package identifier

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("pod_identifier")

var statefulsetPodNameRegex = regexp.MustCompile(`^(.+)-(\d+)$`)

var (
	ErrMissingLabel      = errors.New("missing StatefulSet pod name label")
	ErrInvalidLabelValue = errors.New("invalid StatefulSet pod name format")
	ErrParsingOrdinal    = errors.New("failed to parse pod ordinal")
)

// SSPodIdentifier defines the interface for extracting StatefulSet information from a pod
type SSPodIdentifier interface {
	// Extract returns the StatefulSet name, pod ordinal, and any error encountered
	// If the pod is not part of a StatefulSet, an error is returned
	Extract(accessor v1.ObjectMetaAccessor) (ssName string, ordinal int, err error)
}

// SSPodIdentifierFunc is a function type that implements the SSPodIdentifier interface
type SSPodIdentifierFunc func(v1.ObjectMetaAccessor) (string, int, error)

// Extract implements the SSPodIdentifier interface for SSPodIdentifierFunc
func (f SSPodIdentifierFunc) Extract(accessor v1.ObjectMetaAccessor) (string, int, error) {
	return f(accessor)
}

// Ensure SSPodIdentifierFunc implements SSPodIdentifier
var _ SSPodIdentifier = SSPodIdentifierFunc(nil)

// LabelSSPodIdentifier extracts StatefulSet information from Kubernetes-standard pod labels
// It looks for the "statefulset.kubernetes.io/pod-name" label which contains the StatefulSet
// name and pod ordinal in the format "<statefulset-name>-<ordinal>"
var LabelSSPodIdentifier SSPodIdentifierFunc = func(accessor v1.ObjectMetaAccessor) (string, int, error) {
	l := log.WithValues("accessor", accessor.GetObjectMeta().GetName())

	// Get the pod name from the standard Kubernetes label
	podName, hasLabel := accessor.GetObjectMeta().GetLabels()["statefulset.kubernetes.io/pod-name"]
	if !hasLabel {
		l.Info("StatefulSet label not found")
		return "", -1, ErrMissingLabel
	}

	l = l.WithValues("podName", podName)
	l.V(1).Info("Found StatefulSet pod name label")

	// Use regex to extract StatefulSet name and ordinal
	matches := statefulsetPodNameRegex.FindStringSubmatch(podName)
	if matches == nil {
		l.Info("Pod name does not match expected StatefulSet format",
			"pattern", statefulsetPodNameRegex.String())
		return "", -1, ErrInvalidLabelValue
	}

	// Extract components from regex match
	ssName := matches[1]
	ordinalStr := matches[2]

	// Parse the ordinal as an integer
	ordinal, err := strconv.Atoi(ordinalStr)
	if err != nil {
		l.Error(err, "Failed to parse ordinal as integer", "ordinalStr", ordinalStr)
		return "", -1, fmt.Errorf("%w: %v", ErrParsingOrdinal, err)
	}

	l.Info("Successfully extracted StatefulSet information",
		"statefulSet", ssName, "ordinal", ordinal)

	return ssName, ordinal, nil
}
