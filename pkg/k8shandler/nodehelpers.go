package k8shandler

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	elasticsearchCertsPath    = "/etc/elasticsearch/secret"
	clusterHealthURL          = "/_nodes/_local"
	elasticsearchConfigPath   = "/usr/share/java/elasticsearch/config"
	elasticsearchDefaultImage = "docker.io/t0ffel/elasticsearch5"
	defaultMasterCPULimit     = "100m"
	defaultMasterCPURequest   = "100m"
	defaultCPULimit           = "4000m"
	defaultCPURequest         = "100m"
	defaultMemoryLimit        = "4Gi"
	defaultMemoryRequest      = "1Gi"
	heapDumpLocation          = "/elasticsearch/persistent/heapdump.hprof"
	promUser                  = "prometheus"
)

func getReadinessProbe() v1.Probe {
	return v1.Probe{
		TimeoutSeconds:      30,
		InitialDelaySeconds: 10,
		FailureThreshold:    15,
		Handler: v1.Handler{
			TCPSocket: &v1.TCPSocketAction{
				Port: intstr.FromInt(9300),
			},
		},
	}
}

func (cfg *elasticsearchNode) getAffinity() v1.Affinity {
	return v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: v1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "role",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{cfg.NodeType},
								},
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}

func (cfg *elasticsearchNode) getEnvVars() []v1.EnvVar {
	return []v1.EnvVar{
		v1.EnvVar{
			Name:  "DC_NAME",
			Value: cfg.DeployName,
		},
		v1.EnvVar{
			Name: "NAMESPACE",
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		v1.EnvVar{
			Name:  "KUBERNETES_TRUST_CERT",
			Value: "true",
		},
		v1.EnvVar{
			Name:  "SERVICE_DNS",
			Value: fmt.Sprintf("%s-cluster", cfg.ClusterName),
		},
		v1.EnvVar{
			Name:  "CLUSTER_NAME",
			Value: cfg.ClusterName,
		},
		v1.EnvVar{
			Name:  "INSTANCE_RAM",
			Value: cfg.getInstanceRAM(),
		},
		v1.EnvVar{
			Name:  "HEAP_DUMP_LOCATION",
			Value: heapDumpLocation,
		},
		v1.EnvVar{
			Name:  "NODE_QUORUM",
			Value: "1",
		},
		v1.EnvVar{
			Name:  "RECOVER_EXPECTED_NODES",
			Value: "1",
		},
		v1.EnvVar{
			Name:  "RECOVER_AFTER_TIME",
			Value: "5m",
		},
		v1.EnvVar{
			Name:  "READINESS_PROBE_TIMEOUT",
			Value: "30",
		},
		v1.EnvVar{
			Name:  "POD_LABEL",
			Value: fmt.Sprintf("cluster=%s", cfg.ClusterName),
		},
		v1.EnvVar{
			Name:  "IS_MASTER",
			Value: cfg.isNodeMaster(),
		},
		v1.EnvVar{
			Name:  "HAS_DATA",
			Value: cfg.isNodeData(),
		},
		v1.EnvVar{
			Name:  "PROMETHEUS_USER",
			Value: promUser,
		},
		v1.EnvVar{
			Name:  "PRIMARY_SHARDS",
			Value: "1",
		},
		v1.EnvVar{
			Name:  "REPLICA_SHARDS",
			Value: "0",
		},
	}
}

func (cfg *elasticsearchNode) getInstanceRAM() string {
	memory := cfg.ESNodeSpec.Resources.Limits.Memory()
	if !memory.IsZero() {
		return memory.String()
	}
	return defaultMemoryLimit
}

func (cfg *elasticsearchNode) getResourceRequirements() v1.ResourceRequirements {
	limitCPU := cfg.ESNodeSpec.Resources.Limits.Cpu()
	if limitCPU.IsZero() {
		CPU, _ := resource.ParseQuantity(defaultCPULimit)
		limitCPU = &CPU
	}
	limitMem, _ := resource.ParseQuantity(cfg.getInstanceRAM())
	requestCPU := cfg.ESNodeSpec.Resources.Requests.Cpu()
	if requestCPU.IsZero() {
		CPU, _ := resource.ParseQuantity(defaultCPURequest)
		requestCPU = &CPU
	}
	requestMem := cfg.ESNodeSpec.Resources.Requests.Memory()
	if requestMem.IsZero() {
		Mem, _ := resource.ParseQuantity(defaultMemoryRequest)
		requestMem = &Mem
	}
	logrus.Infof("Using  memory limit: %v, for node %v", limitMem.String(), cfg.DeployName)

	return v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu":    *limitCPU,
			"memory": limitMem,
		},
		Requests: v1.ResourceList{
			"cpu":    *requestCPU,
			"memory": *requestMem,
		},
	}

}

func (cfg *elasticsearchNode) getESContainer() v1.Container {
	var image string
	if cfg.ESNodeSpec.Config.Image == "" {
		image = elasticsearchDefaultImage
	} else {
		image = cfg.ESNodeSpec.Config.Image
	}
	probe := getReadinessProbe()
	return v1.Container{
		Name:            "elasticsearch",
		Image:           image,
		ImagePullPolicy: "Always",
		Env:             cfg.getEnvVars(),
		Ports: []v1.ContainerPort{
			v1.ContainerPort{
				Name:          "cluster",
				ContainerPort: 9300,
				Protocol:      v1.ProtocolTCP,
			},
			v1.ContainerPort{
				Name:          "restapi",
				ContainerPort: 9200,
				Protocol:      v1.ProtocolTCP,
			},
		},
		ReadinessProbe: &probe,
		LivenessProbe:  &probe,
		VolumeMounts:   cfg.getVolumeMounts(),
		Resources:      cfg.getResourceRequirements(),
	}
}
func (cfg *elasticsearchNode) getVolumeMounts() []v1.VolumeMount {
	mounts := []v1.VolumeMount{
		v1.VolumeMount{
			Name:      "elasticsearch-storage",
			MountPath: "/elasticsearch/persistent",
		},
		v1.VolumeMount{
			Name:      "elasticsearch-config",
			MountPath: elasticsearchConfigPath,
		},
	}
	if cfg.ElasticsearchSecure.Enabled {
		mounts = append(mounts, v1.VolumeMount{
			Name:      "certificates",
			MountPath: elasticsearchCertsPath,
		})
	}
	return mounts
}

func (cfg *elasticsearchNode) generatePersistentStorage() v1.VolumeSource {
	volSource := v1.VolumeSource{}
	specVol := cfg.ESNodeSpec.Storage
	switch {
	case specVol.HostPath != nil:
		volSource.HostPath = specVol.HostPath
	case specVol.EmptyDir != nil:
		volSource.EmptyDir = specVol.EmptyDir
	case specVol.VolumeClaimTemplate != nil:
		claimName := fmt.Sprintf("%s-%s", specVol.VolumeClaimTemplate.Name, cfg.DeployName)
		volClaim := v1.PersistentVolumeClaimVolumeSource{
			ClaimName: claimName,
		}
		volSource.PersistentVolumeClaim = &volClaim
		err := createOrUpdatePersistentVolumeClaim(specVol.VolumeClaimTemplate.Spec, claimName, cfg.Namespace)
		if err != nil {
			logrus.Errorf("Unable to create PersistentVolumeClaim: %v", err)
		}
	case specVol.PersistentVolumeClaim != nil:
		volSource.PersistentVolumeClaim = specVol.PersistentVolumeClaim
	default:
		logrus.Infof("Unknown volume source")
	}
	return volSource
}

func (cfg *elasticsearchNode) getVolumes() []v1.Volume {
	vols := []v1.Volume{
		v1.Volume{
			Name:         "elasticsearch-storage",
			VolumeSource: cfg.generatePersistentStorage(),
		},
		v1.Volume{
			Name: "elasticsearch-config",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: cfg.ClusterName,
					},
				},
			},
		},
	}
	if cfg.ElasticsearchSecure.Enabled {
		secretName := fmt.Sprintf("%s-certs", cfg.ClusterName)

		vols = append(vols, v1.Volume{
			Name: "certificates",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		})
	}
	return vols

}

func (cfg *elasticsearchNode) getSelector() (map[string]string, bool) {
	if len(cfg.ESNodeSpec.NodeSelector) == 0 {
		return nil, false
	}
	return cfg.ESNodeSpec.NodeSelector, true
}
