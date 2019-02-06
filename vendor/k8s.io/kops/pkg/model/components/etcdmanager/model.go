/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package etcdmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	scheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/assets"
	"k8s.io/kops/pkg/dns"
	"k8s.io/kops/pkg/flagbuilder"
	"k8s.io/kops/pkg/k8scodecs"
	"k8s.io/kops/pkg/kubemanifest"
	"k8s.io/kops/pkg/model"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/gce"
	"k8s.io/kops/upup/pkg/fi/fitasks"
	"k8s.io/kops/util/pkg/exec"
)

const metaFilename = "_etcd_backup.meta"

// EtcdManagerBuilder builds the manifest for the etcd-manager
type EtcdManagerBuilder struct {
	*model.KopsModelContext
	Lifecycle    *fi.Lifecycle
	AssetBuilder *assets.AssetBuilder
}

var _ fi.ModelBuilder = &EtcdManagerBuilder{}

// Build creates the tasks
func (b *EtcdManagerBuilder) Build(c *fi.ModelBuilderContext) error {
	for _, etcdCluster := range b.Cluster.Spec.EtcdClusters {
		if etcdCluster.Provider != kops.EtcdProviderTypeManager {
			continue
		}

		name := etcdCluster.Name
		version := etcdCluster.Version

		backupStore := ""
		if etcdCluster.Backups != nil {
			backupStore = etcdCluster.Backups.BackupStore
		}
		if backupStore == "" {
			return fmt.Errorf("backupStore must be set for use with etcd-manager")
		}

		manifest, err := b.buildManifest(etcdCluster)
		if err != nil {
			return err
		}

		manifestYAML, err := k8scodecs.ToVersionedYaml(manifest)
		if err != nil {
			return fmt.Errorf("error marshaling manifest to yaml: %v", err)
		}

		c.AddTask(&fitasks.ManagedFile{
			Contents:  fi.WrapResource(fi.NewBytesResource(manifestYAML)),
			Lifecycle: b.Lifecycle,
			Location:  fi.String("manifests/etcd/" + name + ".yaml"),
			Name:      fi.String("manifests-etcdmanager-" + name),
		})

		info := &etcdClusterSpec{
			EtcdVersion: version,
			MemberCount: int32(len(etcdCluster.Members)),
		}

		d, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return err
		}

		c.AddTask(&fitasks.ManagedFile{
			Contents:  fi.WrapResource(fi.NewBytesResource(d)),
			Lifecycle: b.Lifecycle,
			// TODO: We need this to match the backup base (currently)
			Location: fi.String("backups/etcd/" + etcdCluster.Name + "/control/etcd-cluster-spec"),
			Name:     fi.String("etcd-cluster-spec-" + name),
		})
	}

	return nil
}

type etcdClusterSpec struct {
	MemberCount int32  `json:"member_count,omitempty"`
	EtcdVersion string `json:"etcd_version,omitempty"`
}

func (b *EtcdManagerBuilder) buildManifest(etcdCluster *kops.EtcdClusterSpec) (*v1.Pod, error) {
	return b.buildPod(etcdCluster)
}

// parseManifest parses a set of objects from a []byte
func parseManifest(data []byte) ([]runtime.Object, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	deser := scheme.Codecs.UniversalDeserializer()

	var objects []runtime.Object

	for {
		ext := runtime.RawExtension{}
		if err := decoder.Decode(&ext); err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "%s", string(data))
			glog.Infof("manifest: %s", string(data))
			return nil, fmt.Errorf("error parsing manifest: %v", err)
		}

		obj, _, err := deser.Decode([]byte(ext.Raw), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("error parsing object in manifest: %v", err)
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

// Until we introduce the bundle, we hard-code the manifest
var defaultManifest = `
apiVersion: v1
kind: Pod
metadata:
  name: etcd-manager
  namespace: kube-system
spec:
  containers:
  - image: kopeio/etcd-manager:1.0.20181001
    name: etcd-manager
    resources:
      requests:
        cpu: 100m
    # TODO: Would be nice to reduce these permissions; needed for volume mounting
    securityContext:
      privileged: true
    volumeMounts:
    # TODO: Would be nice to scope this more tightly, but needed for volume mounting
    - mountPath: /rootfs
      name: rootfs
    # We write artificial hostnames into etc hosts for the etcd nodes, so they have stable names
    - mountPath: /etc/hosts
      name: hosts
  hostNetwork: true
  hostPID: true # helps with mounting volumes from inside a container
  volumes:
  - hostPath:
      path: /
      type: Directory
    name: rootfs
  - hostPath:
      path: /etc/hosts
      type: File
    name: hosts
`

// buildPod creates the pod spec, based on the EtcdClusterSpec
func (b *EtcdManagerBuilder) buildPod(etcdCluster *kops.EtcdClusterSpec) (*v1.Pod, error) {
	var pod *v1.Pod
	var container *v1.Container

	var manifest []byte

	// TODO: pull from bundle
	bundle := "(embedded etcd manifest)"
	manifest = []byte(defaultManifest)

	{
		objects, err := parseManifest(manifest)
		if err != nil {
			return nil, err
		}
		if len(objects) != 1 {
			return nil, fmt.Errorf("expected exactly one object in manifest %s, found %d", bundle, len(objects))
		}
		if podObject, ok := objects[0].(*v1.Pod); !ok {
			return nil, fmt.Errorf("expected v1.Pod object in manifest %s, found %T", bundle, objects[0])
		} else {
			pod = podObject
		}

		if len(pod.Spec.Containers) != 1 {
			return nil, fmt.Errorf("expected exactly one container in etcd-manager Pod, found %d", len(pod.Spec.Containers))
		}
		container = &pod.Spec.Containers[0]

		if etcdCluster.Manager != nil && etcdCluster.Manager.Image != "" {
			glog.Warningf("overloading image in manifest %s with images %s", bundle, etcdCluster.Manager.Image)
			container.Image = etcdCluster.Manager.Image
		}
	}

	// Remap image via AssetBuilder
	{
		remapped, err := b.AssetBuilder.RemapImage(container.Image)
		if err != nil {
			return nil, fmt.Errorf("unable to remap container image %q: %v", container.Image, err)
		}
		container.Image = remapped
	}

	isTLS := etcdCluster.EnableEtcdTLS

	cpuRequest := resource.MustParse("100m")
	clientPort := 4001

	clusterName := "etcd-" + etcdCluster.Name
	peerPort := 2380
	backupStore := ""
	if etcdCluster.Backups != nil {
		backupStore = etcdCluster.Backups.BackupStore
	}

	pod.Name = "etcd-manager-" + etcdCluster.Name
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels["k8s-app"] = pod.Name

	// TODO: Use a socket file for the quarantine port
	quarantinedClientPort := 3994

	grpcPort := 3996

	// The dns suffix logic mirrors the existing logic, so we should be compatible with existing clusters
	// (etcd makes it difficult to change peer urls, treating it as a cluster event, for reasons unknown)
	dnsInternalSuffix := ""
	if dns.IsGossipHostname(b.Cluster.Spec.MasterInternalName) {
		// @TODO: This is hacky, but we want it so that we can have a different internal & external name
		dnsInternalSuffix = b.Cluster.Spec.MasterInternalName
		dnsInternalSuffix = strings.TrimPrefix(dnsInternalSuffix, "api.")
	}

	if dnsInternalSuffix == "" {
		dnsInternalSuffix = ".internal." + b.Cluster.ObjectMeta.Name
	}

	switch etcdCluster.Name {
	case "main":
		clusterName = "etcd"
		cpuRequest = resource.MustParse("200m")

	case "events":
		clientPort = 4002
		peerPort = 2381
		grpcPort = 3997
		quarantinedClientPort = 3995

	default:
		return nil, fmt.Errorf("unknown etcd cluster key %q", etcdCluster.Name)
	}

	if backupStore == "" {
		return nil, fmt.Errorf("backupStore must be set for use with etcd-manager")
	}

	name := clusterName
	if !strings.HasPrefix(name, "etcd") {
		// For sanity, and to avoid collisions in directories / dns
		return nil, fmt.Errorf("unexpected name for etcd cluster (must start with etcd): %q", name)
	}
	logFile := "/var/log/" + name + ".log"

	config := &config{
		Containerized: true,
		ClusterName:   clusterName,
		BackupStore:   backupStore,
		GrpcPort:      grpcPort,
		DNSSuffix:     dnsInternalSuffix,
	}

	config.LogVerbosity = 8

	{
		// @check if we are using TLS
		scheme := "http"
		if isTLS {
			scheme = "https"
		}

		config.PeerUrls = fmt.Sprintf("%s://__name__:%d", scheme, peerPort)
		config.ClientUrls = fmt.Sprintf("%s://__name__:%d", scheme, clientPort)
		config.QuarantineClientUrls = fmt.Sprintf("%s://__name__:%d", scheme, quarantinedClientPort)

		// TODO: We need to wire these into the etcd-manager spec
		// // add timeout/heartbeat settings
		if etcdCluster.LeaderElectionTimeout != nil {
			// 	envs = append(envs, v1.EnvVar{Name: "ETCD_ELECTION_TIMEOUT", Value: convEtcdSettingsToMs(etcdClusterSpec.LeaderElectionTimeout)})
			return nil, fmt.Errorf("LeaderElectionTimeout not supported by etcd-manager")
		}
		if etcdCluster.HeartbeatInterval != nil {
			// 	envs = append(envs, v1.EnvVar{Name: "ETCD_HEARTBEAT_INTERVAL", Value: convEtcdSettingsToMs(etcdClusterSpec.HeartbeatInterval)})
			return nil, fmt.Errorf("HeartbeatInterval not supported by etcd-manager")
		}

		if isTLS {
			return nil, fmt.Errorf("TLS not supported for etcd-manager")
		}
	}

	{
		switch kops.CloudProviderID(b.Cluster.Spec.CloudProvider) {
		case kops.CloudProviderAWS:
			config.VolumeProvider = "aws"

			config.VolumeTag = []string{
				fmt.Sprintf("kubernetes.io/cluster/%s=owned", b.Cluster.Name),
				awsup.TagNameEtcdClusterPrefix + etcdCluster.Name,
				awsup.TagNameRolePrefix + "master=1",
			}
			config.VolumeNameTag = awsup.TagNameEtcdClusterPrefix + etcdCluster.Name

		case kops.CloudProviderGCE:
			config.VolumeProvider = "gce"

			config.VolumeTag = []string{
				gce.GceLabelNameKubernetesCluster + "=" + gce.SafeClusterName(b.Cluster.Name),
				gce.GceLabelNameEtcdClusterPrefix + etcdCluster.Name,
				gce.GceLabelNameRolePrefix + "master=master",
			}
			config.VolumeNameTag = gce.GceLabelNameEtcdClusterPrefix + etcdCluster.Name

		default:
			return nil, fmt.Errorf("CloudProvider %q not supported with etcd-manager", b.Cluster.Spec.CloudProvider)
		}
	}

	args, err := flagbuilder.BuildFlagsList(config)
	if err != nil {
		return nil, err
	}

	{
		container.Command = exec.WithTee("/etcd-manager", args, "/var/log/etcd.log")

		// TODO: Should we try to incorporate the resources in the manifest?
		container.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceCPU: cpuRequest,
			},
		}

		// TODO: Use helper function here
		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			Name:      "varlogetcd",
			MountPath: "/var/log/etcd.log",
			ReadOnly:  false,
		})
		hostPathFileOrCreate := v1.HostPathFileOrCreate
		pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
			Name: "varlogetcd",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: logFile,
					Type: &hostPathFileOrCreate,
				},
			},
		})

		if isTLS {
			return nil, fmt.Errorf("TLS not supported for etcd-manager")
		}
	}

	kubemanifest.MarkPodAsCritical(pod)

	return pod, nil
}

// config defines the flags for etcd-manager
type config struct {
	// LogVerbosity sets the log verbosity level
	LogVerbosity int `flag:"v"`

	// Containerized is set if etcd-manager is running in a container
	Containerized bool `flag:"containerized"`

	Address              string   `flag:"address"`
	PeerUrls             string   `flag:"peer-urls"`
	GrpcPort             int      `flag:"grpc-port"`
	ClientUrls           string   `flag:"client-urls"`
	QuarantineClientUrls string   `flag:"quarantine-client-urls"`
	ClusterName          string   `flag:"cluster-name"`
	BackupStore          string   `flag:"backup-store"`
	DataDir              string   `flag:"data-dir"`
	VolumeProvider       string   `flag:"volume-provider"`
	VolumeTag            []string `flag:"volume-tag,repeat"`
	VolumeNameTag        string   `flag:"volume-name-tag"`
	DNSSuffix            string   `flag:"dns-suffix"`
}
