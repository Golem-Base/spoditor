package annotation

import (
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	Prefix    = "spoditor.io/"
	Separator = "_"
)

var log = logf.Log.WithName("annotations")

// Handler defines operations for mutating pod specs based on annotations
type Handler interface {
	Mutate(spec *corev1.PodSpec, ordinal int, cfg any) error
	GetParser() Parser
}

// Parser converts annotation maps to configuration objects
type Parser interface {
	Parse(annotations map[QualifiedName]string) (any, error)
}

// ParserFunc is a function adapter that implements the Parser interface
type ParserFunc func(map[QualifiedName]string) (any, error)

func (p ParserFunc) Parse(annotations map[QualifiedName]string) (any, error) {
	return p(annotations)
}

// QualifiedName represents a parsed annotation key with optional qualifier
type QualifiedName struct {
	Qualifier string // Optional pod ordinal selector
	Name      string // Feature name
}

// QualifiedAnnotationCollector extracts qualified annotations from k8s objects
type QualifiedAnnotationCollector interface {
	Collect(accessor metav1.ObjectMetaAccessor) map[QualifiedName]string
}

// CollectorFunc is a function adapter that implements QualifiedAnnotationCollector
type CollectorFunc func(metav1.ObjectMetaAccessor) map[QualifiedName]string

func (c CollectorFunc) Collect(accessor metav1.ObjectMetaAccessor) map[QualifiedName]string {
	return c(accessor)
}

// Collector is the global annotation collector instance
var Collector QualifiedAnnotationCollector = defaultCollector

// defaultCollector implements annotation collection logic
var defaultCollector = CollectorFunc(func(accessor metav1.ObjectMetaAccessor) map[QualifiedName]string {
	result := make(map[QualifiedName]string)

	for k, v := range accessor.GetObjectMeta().GetAnnotations() {
		// Use V(1) for more detailed logging that's not needed in normal operation
		logger := log.V(1).WithValues("key", k, "value", v)

		if !strings.HasPrefix(k, Prefix) {
			// Skip irrelevant annotations silently - only log at high verbosity
			logger.Info("skipping irrelevant annotation")
			continue
		}

		// Log finding relevant annotations at regular level, but with less detail
		log.V(0).Info("found annotation", "key", k)

		name := strings.TrimPrefix(k, Prefix)
		separatorIndex := strings.LastIndex(name, Separator)

		if separatorIndex == -1 {
			logger.Info("dynamic argumentation")
			result[QualifiedName{Name: name}] = v
		} else {
			logger.Info("designated argumentation")
			result[QualifiedName{
				Qualifier: name[separatorIndex+1:],
				Name:      name[:separatorIndex],
			}] = v
		}
	}

	return result
})

// PodQualifier determines if a pod with given ordinal matches a qualifier
type PodQualifier func(ordinal int, qualifier string) bool

// Compile regular expressions once to improve performance
var (
	rangeRegex       = regexp.MustCompile(`^\d+-\d+$`)
	exactNumberRegex = regexp.MustCompile(`^\d+$`)
	lowerBoundRegex  = regexp.MustCompile(`^\d+-$`)
	upperBoundRegex  = regexp.MustCompile(`^-\d+$`)
)

// CommonPodQualifier is the standard implementation of PodQualifier
var CommonPodQualifier PodQualifier = func(ordinal int, qualifier string) bool {
	logger := log.WithValues("ordinal", ordinal, "qualifier", qualifier)

	// Empty qualifier means apply to all pods
	if qualifier == "" {
		logger.Info("pod is always included for dynamic argumentation")
		return true
	}

	// Handle ranges: "1-5"
	if rangeRegex.MatchString(qualifier) {
		bounds := strings.Split(qualifier, "-")
		min, _ := strconv.Atoi(bounds[0])
		max, _ := strconv.Atoi(bounds[1])
		logger.Info("checking ordinal against range", "min", min, "max", max)
		return ordinal >= min && ordinal <= max
	}

	// Handle exact match: "3"
	if exactNumberRegex.MatchString(qualifier) {
		target, _ := strconv.Atoi(qualifier)
		logger.Info("checking ordinal against exact number", "number", target)
		return ordinal == target
	}

	// Handle lower bound: "3-"
	if lowerBoundRegex.MatchString(qualifier) {
		min, _ := strconv.Atoi(strings.TrimSuffix(qualifier, "-"))
		logger.Info("checking ordinal against lower bound", "min", min)
		return ordinal >= min
	}

	// Handle upper bound: "-5"
	if upperBoundRegex.MatchString(qualifier) {
		max, _ := strconv.Atoi(strings.TrimPrefix(qualifier, "-"))
		logger.Info("checking ordinal against upper bound", "max", max)
		return ordinal <= max
	}

	return false
}
