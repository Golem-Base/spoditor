package volumes

import (
	"fmt"
	"strconv"

	"github.com/spoditor/spoditor/internal/annotation"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// MountVolume is the annotation key for volume mounting configuration
	MountVolume = "mount-volume"
)

// Compile-time interface check
var _ annotation.Handler = (*MountHandler)(nil)

var log = logf.Log.WithName("mount_volume")

// mountConfig holds the volume mounting configuration with its pod qualifier
type mountConfig struct {
	qualifier string            // Which pods this applies to
	cfg       *mountConfigValue // The actual volume configuration
}

// mountConfigValue represents the JSON structure of the volume mount configuration
type mountConfigValue struct {
	Volumes    []corev1.Volume    `json:"volumes"`    // Volumes to be added to the pod
	Containers []corev1.Container `json:"containers"` // Container configurations for volume mounts
}

// MountHandler handles volume mount operations based on annotations
type MountHandler struct{}

// Mutate modifies the pod spec to add volumes and volume mounts as specified
func (h *MountHandler) Mutate(spec *corev1.PodSpec, ordinal int, cfg any) error {
	logger := log.WithValues("ordinal", ordinal)

	// Type assertion for our config
	m, ok := cfg.(*mountConfig)
	if !ok {
		return fmt.Errorf("unexpected config type %T, expected *mountConfig", cfg)
	}

	// Check if this pod matches the qualifier
	if !annotation.CommonPodQualifier(ordinal, m.qualifier) {
		logger.Info("qualifier excludes this pod")
		return nil
	}

	logger.Info("applying volume mounts to pod")

	// Process volumes, adding ordinal suffix to ConfigMap and Secret references
	volumes := make([]corev1.Volume, len(m.cfg.Volumes))
	for i, v := range m.cfg.Volumes {
		// Create a deep copy to avoid modifying the original
		volumes[i] = v

		// Suffix for ConfigMap and Secret names
		ordinalSuffix := "-" + strconv.Itoa(ordinal)

		// Handle ConfigMap references
		if v.ConfigMap != nil {
			originalName := v.ConfigMap.LocalObjectReference.Name
			newName := originalName + ordinalSuffix

			logger.Info("renaming configmap reference",
				"volume", v.Name,
				"from", originalName,
				"to", newName)

			volumes[i].ConfigMap = v.ConfigMap.DeepCopy()
			volumes[i].ConfigMap.LocalObjectReference.Name = newName
		}

		// Handle Secret references
		if v.Secret != nil {
			originalName := v.Secret.SecretName
			newName := originalName + ordinalSuffix

			logger.Info("renaming secret reference",
				"volume", v.Name,
				"from", originalName,
				"to", newName)

			volumes[i].Secret = v.Secret.DeepCopy()
			volumes[i].Secret.SecretName = newName
		}
	}

	// Add processed volumes to the pod spec
	spec.Volumes = append(spec.Volumes, volumes...)

	// Add volume mounts to matching containers
	for _, source := range m.cfg.Containers {
		for i := range spec.Containers {
			if source.Name == spec.Containers[i].Name {
				logger.Info("adding volume mounts to container",
					"container", source.Name,
					"mounts", len(source.VolumeMounts))

				spec.Containers[i].VolumeMounts = append(
					spec.Containers[i].VolumeMounts,
					source.VolumeMounts...)
			}
		}
	}

	return nil
}

// GetParser returns the parser for volume mount annotations
func (h *MountHandler) GetParser() annotation.Parser {
	return volumeMountParser
}

// Ensure MountHandler implements the annotation.Handler interface
var _ annotation.Handler = &MountHandler{}

// volumeMountParser parses volume mount annotations into a mountConfig
var volumeMountParser annotation.ParserFunc = func(annotations map[annotation.QualifiedName]string) (any, error) {
	for k, v := range annotations {
		if k.Name != MountVolume {
			continue
		}

		logger := log.WithValues("qualifiedName", k, "value", v)
		logger.Info("parsing volume mount configuration")

		// Attempt to unmarshal the JSON configuration
		config := &mountConfigValue{}
		if err := json.Unmarshal([]byte(v), config); err != nil {
			logger.Error(err, "failed to parse volume mount configuration")
			return nil, fmt.Errorf("invalid volume mount configuration: %w", err)
		}

		// Validate the configuration
		if len(config.Volumes) == 0 {
			logger.Info("configuration has no volumes, skipping")
			return nil, nil
		}

		return &mountConfig{
			qualifier: k.Qualifier,
			cfg:       config,
		}, nil
	}

	return nil, nil
}
