package ports

import (
	"fmt"
	"strconv"

	"github.com/golem-base/spoditor/internal/annotation"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// HostPort is the annotation key for port modification configuration
	HostPort = "host-port"
	// PodOrdinal is the environment variable name for pod ordinal
	PodOrdinal = "POD_ORDINAL"
	// PortPrefix is the prefix for port environment variables
	PortPrefix = "PORT_"
)

// Compile-time interface check
var _ annotation.Handler = (*HostPortHandler)(nil)

var log = logf.Log.WithName("host_port")

// portConfig holds the port modification configuration with its pod qualifier
type portConfig struct {
	qualifier string
	cfg       *portConfigValue
}

// portConfigValue represents the JSON structure of the port modification configuration
type portConfigValue struct {
	Containers []containerPortsConfig `json:"containers"`
}

// containerPortsConfig defines the ports to modify for a specific container
type containerPortsConfig struct {
	Name  string                 `json:"name"`
	Ports []corev1.ContainerPort `json:"ports"`
}

// HostPortHandler implements the handler interface for modifying container ports
type HostPortHandler struct{}

// Mutate modifies the container ports in the pod spec based on the configuration
func (h *HostPortHandler) Mutate(spec *corev1.PodSpec, ordinal int, cfg any) error {
	logger := log.WithValues("ordinal", ordinal)

	// Type assertion for our config
	m, ok := cfg.(*portConfig)
	if !ok {
		return fmt.Errorf("unexpected config type %T", cfg)
	}

	// Check if this pod matches the qualifier
	if !annotation.CommonPodQualifier(ordinal, m.qualifier) {
		logger.Info("qualifier excludes this pod")
		return nil
	}

	logger.Info("modifying container ports for pod")

	// Map to collect port assignments to inject as environment variables
	portEnvVars := make(map[string]map[string]string)

	// For each container in the config
	for _, containerConfig := range m.cfg.Containers {
		portEnvVars[containerConfig.Name] = make(map[string]string)

		// Find the matching container in the pod spec
		for i := range spec.Containers {
			container := &spec.Containers[i]

			if container.Name != containerConfig.Name {
				continue
			}

			containerLogger := logger.WithValues("container", containerConfig.Name)
			containerLogger.Info("processing container")

			// Process each port in the config
			for _, portConfig := range containerConfig.Ports {
				// Skip ports with no hostPort defined
				if portConfig.HostPort <= 0 {
					continue
				}

				// Calculate new hostPort with ordinal offset
				newHostPort := int32(portConfig.HostPort) + int32(ordinal)
				portVarName := fmt.Sprintf("%s%s", PortPrefix, portConfig.Name)

				// Find if this port already exists in the container
				foundPort := false

				// Look for ports with the same name
				for j := range container.Ports {
					if container.Ports[j].Name == portConfig.Name {
						// Found matching port, update hostPort value
						containerLogger.Info("modifying hostPort",
							"port", portConfig.Name,
							"oldValue", container.Ports[j].HostPort,
							"newValue", newHostPort)
						container.Ports[j].HostPort = newHostPort

						// Store for environment variable
						portEnvVars[containerConfig.Name][portVarName] = strconv.Itoa(int(newHostPort))
						foundPort = true
						break
					}
				}

				// If port wasn't found, add it
				if !foundPort {
					newPort := portConfig.DeepCopy()
					newPort.HostPort = newHostPort
					containerLogger.Info("adding new port",
						"port", newPort.Name,
						"hostPort", newPort.HostPort)
					container.Ports = append(container.Ports, *newPort)

					// Store for environment variable
					portEnvVars[containerConfig.Name][portVarName] = strconv.Itoa(int(newPort.HostPort))
				}
			}

			// Add pod ordinal as an environment variable
			container.Env = appendEnvVarIfNotExists(container.Env, corev1.EnvVar{
				Name:  PodOrdinal,
				Value: strconv.Itoa(ordinal),
			})

			// Add port environment variables
			for varName, varValue := range portEnvVars[containerConfig.Name] {
				container.Env = appendEnvVarIfNotExists(container.Env, corev1.EnvVar{
					Name:  varName,
					Value: varValue,
				})
			}
		}
	}

	return nil
}

// appendEnvVarIfNotExists adds or updates an environment variable
func appendEnvVarIfNotExists(envVars []corev1.EnvVar, envVar corev1.EnvVar) []corev1.EnvVar {
	for i, existing := range envVars {
		if existing.Name == envVar.Name {
			envVars[i] = envVar
			return envVars
		}
	}
	// Append if not found
	return append(envVars, envVar)
}

// GetParser returns the parser for port modification annotations
func (h *HostPortHandler) GetParser() annotation.Parser {
	return parser
}

// parser parses port modification annotations into a portConfig
var parser annotation.ParserFunc = func(annotations map[annotation.QualifiedName]string) (any, error) {
	for k, v := range annotations {
		if k.Name != HostPort {
			continue
		}

		logger := log.WithValues("qualifiedName", k, "value", v)
		logger.Info("parsing port modification configuration")

		c := &portConfigValue{}
		if err := json.Unmarshal([]byte(v), c); err != nil {
			return nil, fmt.Errorf("failed to parse port configuration: %w", err)
		}

		return &portConfig{
			qualifier: k.Qualifier,
			cfg:       c,
		}, nil
	}
	return nil, nil
}
