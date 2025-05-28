package ports

import (
	"fmt"
	"strconv"

	"github.com/spoditor/spoditor/internal/annotation"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// HostPort is the annotation name for configuring host ports
	HostPort = "host-port"
	// PodOrdinalEnvVarName is the environment variable name for the pod ordinal
	PodOrdinalEnvVarName = "POD_ORDINAL"
	// PortEnvVarPrefix is the prefix for port environment variable names
	PortEnvVarPrefix = "PORT_"
)

var log = logf.Log.WithName("host_port")

// portConfig stores the parsed annotation configuration
type portConfig struct {
	qualifier string
	cfg       *portConfigValue
}

// portConfigValue defines the JSON structure for port configuration
type portConfigValue struct {
	Containers []containerPortsConfig `json:"containers"`
}

// containerPortsConfig defines port settings for a specific container
type containerPortsConfig struct {
	Name  string                 `json:"name"`
	Ports []corev1.ContainerPort `json:"ports"`
}

var _ annotation.Handler = (*HostPortHandler)(nil)

// HostPortHandler implements the handler interface for modifying container ports
type HostPortHandler struct {
}

// Mutate updates the Pod specification to set host ports and related environment variables
func (h *HostPortHandler) Mutate(spec *corev1.PodSpec, ordinal int, cfg interface{}) error {
	ll := log.WithValues("ordinal", ordinal)
	m, ok := cfg.(*portConfig)
	if !ok {
		return fmt.Errorf("unexpected config type %T", m)
	}

	if !annotation.CommonPodQualifier(ordinal, m.qualifier) {
		ll.Info("qualifier excludes this pod")
		return nil
	}

	ll.Info("modifying container ports for pod", "ordinal", ordinal)

	// Map to collect port assignments to inject as environment variables
	portEnvVars := make(map[string]map[string]string)

	// For each container in the config
	for _, containerConfig := range m.cfg.Containers {
		portEnvVars[containerConfig.Name] = make(map[string]string)

		// Find the matching container in the pod spec
		for i := range spec.Containers {
			if spec.Containers[i].Name == containerConfig.Name {
				ll.Info("processing container", "container", containerConfig.Name)

				// Process each port in the config
				for _, portConfig := range containerConfig.Ports {
					foundPort := false

					// Look for ports with the same name
					for j := range spec.Containers[i].Ports {
						if spec.Containers[i].Ports[j].Name == portConfig.Name {
							// Found matching port, update hostPort value with ordinal offset
							if portConfig.HostPort > 0 {
								newHostPort := int32(portConfig.HostPort) + int32(ordinal)
								ll.Info("modifying hostPort",
									"container", containerConfig.Name,
									"port", portConfig.Name,
									"oldValue", spec.Containers[i].Ports[j].HostPort,
									"newValue", newHostPort)
								spec.Containers[i].Ports[j].HostPort = newHostPort

								// Store for environment variable
								portVarName := fmt.Sprintf("%s%s", PortEnvVarPrefix, portConfig.Name)
								portEnvVars[containerConfig.Name][portVarName] = strconv.Itoa(int(newHostPort))
							}
							foundPort = true
							break
						}
					}

					// If port wasn't found, add it
					if !foundPort && portConfig.HostPort > 0 {
						newPort := portConfig.DeepCopy()
						newPort.HostPort = int32(portConfig.HostPort) + int32(ordinal)
						ll.Info("adding new port with hostPort",
							"container", containerConfig.Name,
							"port", newPort.Name,
							"hostPort", newPort.HostPort)
						spec.Containers[i].Ports = append(spec.Containers[i].Ports, *newPort)

						// Store for environment variable
						portVarName := fmt.Sprintf("%s%s", PortEnvVarPrefix, newPort.Name)
						portEnvVars[containerConfig.Name][portVarName] = strconv.Itoa(int(newPort.HostPort))
					}
				}

				// Add pod ordinal as an environment variable
				ordinalEnvVar := corev1.EnvVar{
					Name:  PodOrdinalEnvVarName,
					Value: strconv.Itoa(ordinal),
				}
				spec.Containers[i].Env = appendEnvVarIfNotExists(spec.Containers[i].Env, ordinalEnvVar)

				// Add port environment variables
				for varName, varValue := range portEnvVars[containerConfig.Name] {
					portEnvVar := corev1.EnvVar{
						Name:  varName,
						Value: varValue,
					}
					spec.Containers[i].Env = appendEnvVarIfNotExists(spec.Containers[i].Env, portEnvVar)
				}
			}
		}
	}

	return nil
}

// appendEnvVarIfNotExists adds or updates an environment variable in a list
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

// GetParser returns the annotation parser for host port configurations
func (h *HostPortHandler) GetParser() annotation.Parser {
	return parser
}

// parser parses host port annotations and returns a portConfig
var parser annotation.ParserFunc = func(annotations map[annotation.QualifiedName]string) (interface{}, error) {
	for k, v := range annotations {
		ll := log.WithValues("qualifiedName", k, "value", v)
		if k.Name == HostPort {
			ll.Info("parse config for modifying host ports")
			c := &portConfigValue{}
			if err := json.Unmarshal([]byte(v), c); err == nil {
				return &portConfig{
					qualifier: k.Qualifier,
					cfg:       c,
				}, nil
			} else {
				return nil, err
			}
		}
	}
	return nil, nil
}
