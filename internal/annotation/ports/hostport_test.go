package ports

import (
	"reflect"
	"testing"

	"github.com/golem-base/spoditor/internal/annotation"
	corev1 "k8s.io/api/core/v1"
)

func TestPortModifierHandler_Mutate(t *testing.T) {
	type args struct {
		spec    *corev1.PodSpec
		ordinal int
		cfg     any
	}
	tests := []struct {
		name    string
		args    args
		want    *corev1.PodSpec
		wantErr bool
	}{
		{
			name: "wrong config type",
			args: args{
				spec:    nil,
				ordinal: 0,
				cfg:     nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "do nothing because ordinal doesn't qualify",
			args: args{
				spec:    &corev1.PodSpec{},
				ordinal: 0,
				cfg: &portConfig{
					qualifier: "1-2",
					cfg:       nil,
				},
			},
			want:    &corev1.PodSpec{},
			wantErr: false,
		},
		{
			name: "modify hostPort and add env vars",
			args: args{
				spec: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "web",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									HostPort:      30000,
								},
							},
						},
					},
				},
				ordinal: 2,
				cfg: &portConfig{
					qualifier: "",
					cfg: &portConfigValue{
						Containers: []containerPortsConfig{
							{
								Name: "web",
								Ports: []corev1.ContainerPort{
									{
										Name:          "http",
										ContainerPort: 8080,
										HostPort:      30000,
									},
								},
							},
						},
					},
				},
			},
			want: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "web",
						Ports: []corev1.ContainerPort{
							{
								Name:          "http",
								ContainerPort: 8080,
								HostPort:      30002, // 30000 + ordinal(2)
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "POD_ORDINAL",
								Value: "2",
							},
							{
								Name:  "PORT_http",
								Value: "30002",
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &HostPortHandler{}
			if err := h.Mutate(tt.args.spec, tt.args.ordinal, tt.args.cfg); (err != nil) != tt.wantErr {
				t.Errorf("Mutate() error = %v, wantErr %v", err, tt.wantErr)
			} else if !reflect.DeepEqual(tt.args.spec, tt.want) {
				t.Errorf("Mutate() = %v, want %v", tt.args.spec, tt.want)
			}
		})
	}
}

func Test_parserFunc_Parse(t *testing.T) {
	type args struct {
		annotations map[annotation.QualifiedName]string
	}

	tests := []struct {
		name    string
		p       annotation.ParserFunc
		args    args
		want    any
		wantErr bool
	}{
		{
			name:    "no expected annotation",
			p:       parser,
			args:    args{annotations: map[annotation.QualifiedName]string{}},
			want:    nil,
			wantErr: false,
		},
		{
			name: "valid config",
			p:    parser,
			args: args{annotations: map[annotation.QualifiedName]string{
				{
					Name: HostPort,
				}: `{"containers":[{"name":"web","ports":[{"name":"http","containerPort":8080,"hostPort":30000}]}]}`,
			}},
			want: &portConfig{
				qualifier: "",
				cfg: &portConfigValue{
					Containers: []containerPortsConfig{
						{
							Name: "web",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									HostPort:      30000,
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid json",
			p:    parser,
			args: args{annotations: map[annotation.QualifiedName]string{
				{
					Name: HostPort,
				}: `{"containers":[{"name":`,
			}},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.p.Parse(tt.args.annotations)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() got = %v, want %v", got, tt.want)
			}
		})
	}
}
