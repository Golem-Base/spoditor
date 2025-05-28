package v1

import (
	"context"

	"github.com/golem-base/spoditor/internal/annotation"
	"github.com/golem-base/spoditor/internal/annotation/ports"
	"github.com/golem-base/spoditor/internal/annotation/volumes"
	"github.com/golem-base/spoditor/internal/identifier"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Pod Webhook", func() {
	var (
		ctx     context.Context
		mutator *PodMutator
		pod     *corev1.Pod
	)

	BeforeEach(func() {
		ctx = context.Background()
		mutator = &PodMutator{
			ssPodId:   identifier.LabelSSPodIdentifier,
			collector: annotation.Collector,
			handlers: []annotation.Handler{
				&volumes.MountHandler{},
				&ports.HostPortHandler{},
			},
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test-container",
						Image: "nginx",
					},
				},
			},
		}
	})

	Context("When mutating Pods", func() {
		It("Should ignore non-StatefulSet pods", func() {
			// A regular pod without StatefulSet labels should not be modified
			err := mutator.Default(ctx, pod)
			Expect(err).NotTo(HaveOccurred())

			// Verify no modifications were made
			Expect(pod.Spec.Volumes).To(BeEmpty())
			Expect(pod.Spec.Containers[0].VolumeMounts).To(BeEmpty())
		})

		It("Should process StatefulSet pods without annotations", func() {
			// Add StatefulSet label to make it a StatefulSet pod
			pod.ObjectMeta.Labels = map[string]string{
				"statefulset.kubernetes.io/pod-name": "test-statefulset-0",
			}

			err := mutator.Default(ctx, pod)
			Expect(err).NotTo(HaveOccurred())

			// Without annotations, no changes should be made
			Expect(pod.Spec.Volumes).To(BeEmpty())
			Expect(pod.Spec.Containers[0].VolumeMounts).To(BeEmpty())
		})

		It("Should mount volumes based on annotations", func() {
			// Create a StatefulSet pod with volume mount annotation
			pod.ObjectMeta.Labels = map[string]string{
				"statefulset.kubernetes.io/pod-name": "test-statefulset-0",
			}
			pod.ObjectMeta.Annotations = map[string]string{
				"spoditor.io/mount-volume": `{
					"volumes": [
						{
							"name": "config-volume",
							"configMap": {
								"name": "test-config"
							}
						}
					],
					"containers": [
						{
							"name": "test-container",
							"volumeMounts": [
								{
									"name": "config-volume",
									"mountPath": "/etc/config"
								}
							]
						}
					]
				}`,
			}

			err := mutator.Default(ctx, pod)
			Expect(err).NotTo(HaveOccurred())

			// Verify the volume was added with ordinal suffix
			Expect(pod.Spec.Volumes).To(HaveLen(1))
			Expect(pod.Spec.Volumes[0].Name).To(Equal("config-volume"))
			Expect(pod.Spec.Volumes[0].ConfigMap).NotTo(BeNil())
			Expect(pod.Spec.Volumes[0].ConfigMap.Name).To(Equal("test-config-0"))

			// Verify the volume mount was added
			Expect(pod.Spec.Containers[0].VolumeMounts).To(HaveLen(1))
			Expect(pod.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("config-volume"))
			Expect(pod.Spec.Containers[0].VolumeMounts[0].MountPath).To(Equal("/etc/config"))
		})

		It("Should configure port forwarding based on annotations", func() {
			// Create a StatefulSet pod with port forwarding annotation
			pod.ObjectMeta.Labels = map[string]string{
				"statefulset.kubernetes.io/pod-name": "test-statefulset-2", // Note the ordinal is 2
			}
			pod.ObjectMeta.Annotations = map[string]string{
				"spoditor.io/host-port": `{
					"containers": [
						{
							"name": "test-container",
							"ports": [
								{
									"name": "http",
									"containerPort": 8080,
									"hostPort": 30000
								}
							]
						}
					]
				}`,
			}

			err := mutator.Default(ctx, pod)
			Expect(err).NotTo(HaveOccurred())

			// Verify the port was configured with ordinal offset
			Expect(pod.Spec.Containers[0].Ports).To(HaveLen(1))
			Expect(pod.Spec.Containers[0].Ports[0].Name).To(Equal("http"))
			Expect(pod.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(8080)))
			Expect(pod.Spec.Containers[0].Ports[0].HostPort).To(Equal(int32(30002))) // 30000 + ordinal(2)

			// Verify environment variables were added
			hasOrdinalVar := false
			hasPortVar := false
			for _, env := range pod.Spec.Containers[0].Env {
				if env.Name == "POD_ORDINAL" && env.Value == "2" {
					hasOrdinalVar = true
				}
				if env.Name == "PORT_http" && env.Value == "30002" {
					hasPortVar = true
				}
			}
			Expect(hasOrdinalVar).To(BeTrue(), "POD_ORDINAL environment variable should be set")
			Expect(hasPortVar).To(BeTrue(), "PORT_http environment variable should be set")
		})

		It("Should respect pod ordinal qualifiers in annotations", func() {
			// Create a StatefulSet pod with a qualified annotation
			pod.ObjectMeta.Labels = map[string]string{
				"statefulset.kubernetes.io/pod-name": "test-statefulset-3", // Ordinal is 3
			}

			// Add annotation that should only apply to pods 0-2
			pod.ObjectMeta.Annotations = map[string]string{
				"spoditor.io/mount-volume_0-2": `{
					"volumes": [
						{
							"name": "qualified-volume",
							"configMap": {
								"name": "qualified-config"
							}
						}
					],
					"containers": [
						{
							"name": "test-container",
							"volumeMounts": [
								{
									"name": "qualified-volume",
									"mountPath": "/etc/qualified"
								}
							]
						}
					]
				}`,
			}

			err := mutator.Default(ctx, pod)
			Expect(err).NotTo(HaveOccurred())

			// This pod has ordinal 3, so the annotation with qualifier 0-2 should not apply
			Expect(pod.Spec.Volumes).To(BeEmpty())
			Expect(pod.Spec.Containers[0].VolumeMounts).To(BeEmpty())

			// Now change the pod ordinal to be within the qualifier range
			pod.ObjectMeta.Labels["statefulset.kubernetes.io/pod-name"] = "test-statefulset-1"

			err = mutator.Default(ctx, pod)
			Expect(err).NotTo(HaveOccurred())

			// Now the annotation should apply
			Expect(pod.Spec.Volumes).To(HaveLen(1))
			Expect(pod.Spec.Volumes[0].Name).To(Equal("qualified-volume"))
			Expect(pod.Spec.Volumes[0].ConfigMap.Name).To(Equal("qualified-config-1"))
		})
	})
})
