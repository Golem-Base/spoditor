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
	ModifyHostPorts = "modify-host-ports"
)

var log = logf.Log.WithName("modify_host_ports")

type portConfig struct {
	qualifier string
	cfg       *portConfigValue
}

type portConfigValue struct {
	Containers []containerPortsConfig `json:"containers"`
}

type containerPortsConfig struct {
	Name  string                 `json:"name"`
	Ports []corev1.ContainerPort `json:"ports"`
}

// PortModifierHandler implements the handler interface for modifying container ports
type PortModifierHandler struct {
}

func (h *PortModifierHandler) Mutate(spec *corev1.PodSpec, ordinal int, cfg interface{}) error {
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
					// Find if this port already exists in the container
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
								portVarName := fmt.Sprintf("PORT_%s", portConfig.Name)
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
						portVarName := fmt.Sprintf("PORT_%s", newPort.Name)
						portEnvVars[containerConfig.Name][portVarName] = strconv.Itoa(int(newPort.HostPort))
					}
				}

				// Add pod ordinal as an environment variable
				ordinalEnvVar := corev1.EnvVar{
					Name:  "POD_ORDINAL",
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

// Helper function to append an environment variable if it doesn't already exist
func appendEnvVarIfNotExists(envVars []corev1.EnvVar, envVar corev1.EnvVar) []corev1.EnvVar {
	for i, existing := range envVars {
		if existing.Name == envVar.Name {
			// Replace the existing value
			envVars[i] = envVar
			return envVars
		}
	}
	// Append if not found
	return append(envVars, envVar)
}

func (h *PortModifierHandler) GetParser() annotation.Parser {
	return parser
}

var _ annotation.Handler = &PortModifierHandler{}

var parser annotation.ParserFunc = func(annotations map[annotation.QualifiedName]string) (interface{}, error) {
	for k, v := range annotations {
		ll := log.WithValues("qualifiedName", k, "value", v)
		if k.Name == ModifyHostPorts {
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
