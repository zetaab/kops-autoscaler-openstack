/*
Copyright 2016 The Kubernetes Authors.

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

package cloudup

import (
	"fmt"

	channelsapi "k8s.io/kops/channels/pkg/api"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/assets"
	"k8s.io/kops/pkg/featureflag"
	"k8s.io/kops/pkg/templates"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/fitasks"
	"k8s.io/kops/upup/pkg/fi/utils"
)

// BootstrapChannelBuilder is responsible for handling the addons in channels
type BootstrapChannelBuilder struct {
	cluster      *kops.Cluster
	Lifecycle    *fi.Lifecycle
	templates    *templates.Templates
	assetBuilder *assets.AssetBuilder
}

var _ fi.ModelBuilder = &BootstrapChannelBuilder{}

// Build is responsible for adding the addons to the channel
func (b *BootstrapChannelBuilder) Build(c *fi.ModelBuilderContext) error {
	addons, manifests, err := b.buildManifest()
	if err != nil {
		return err
	}

	addonsYAML, err := utils.YamlMarshal(addons)
	if err != nil {
		return fmt.Errorf("error serializing addons yaml: %v", err)
	}

	name := b.cluster.ObjectMeta.Name + "-addons-bootstrap"
	tasks := c.Tasks

	tasks[name] = &fitasks.ManagedFile{
		Contents:  fi.WrapResource(fi.NewBytesResource(addonsYAML)),
		Lifecycle: b.Lifecycle,
		Location:  fi.String("addons/bootstrap-channel.yaml"),
		Name:      fi.String(name),
	}

	for key, manifest := range manifests {
		name := b.cluster.ObjectMeta.Name + "-addons-" + key

		manifestResource := b.templates.Find(manifest)
		if manifestResource == nil {
			return fmt.Errorf("unable to find manifest %s", manifest)
		}

		manifestBytes, err := fi.ResourceAsBytes(manifestResource)
		if err != nil {
			return fmt.Errorf("error reading manifest %s: %v", manifest, err)
		}

		manifestBytes, err = b.assetBuilder.RemapManifest(manifestBytes)
		if err != nil {
			return fmt.Errorf("error remapping manifest %s: %v", manifest, err)
		}

		tasks[name] = &fitasks.ManagedFile{
			Contents:  fi.WrapResource(fi.NewBytesResource(manifestBytes)),
			Lifecycle: b.Lifecycle,
			Location:  fi.String(manifest),
			Name:      fi.String(name),
		}
	}

	return nil
}

func (b *BootstrapChannelBuilder) buildManifest() (*channelsapi.Addons, map[string]string, error) {
	addons := &channelsapi.Addons{}
	addons.Kind = "Addons"
	addons.ObjectMeta.Name = "bootstrap"
	manifests := make(map[string]string)

	{
		key := "core.addons.k8s.io"
		version := "1.4.0"
		location := key + "/v" + version + ".yaml"

		addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
			Name:     fi.String(key),
			Version:  fi.String(version),
			Selector: map[string]string{"k8s-addon": key},
			Manifest: fi.String(location),
		})
		manifests[key] = "addons/" + location
	}

	// @check if podsecuritypolicies are enabled and if so, push the default kube-system policy
	if b.cluster.Spec.KubeAPIServer != nil && b.cluster.Spec.KubeAPIServer.HasAdmissionController("PodSecurityPolicy") {
		key := "podsecuritypolicy.addons.k8s.io"
		version := "0.0.4"

		{
			location := key + "/k8s-1.9.yaml"
			id := "k8s-1.9"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.9.0 <1.10.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		// In k8s v1.10, the PodSecurityPolicy API has been moved to the policy/v1beta1 API group
		{
			location := key + "/k8s-1.10.yaml"
			id := "k8s-1.10"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.10.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if b.cluster.Spec.NodeAuthorization != nil {
		{
			key := "node-authorizer.addons.k8s.io"
			version := "v0.0.4"

			{
				location := key + "/k8s-1.10.yaml"
				id := "k8s-1.10.yaml"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          map[string]string{"k8s-addon": key},
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.10.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		}
	}

	kubeDNS := b.cluster.Spec.KubeDNS
	if kubeDNS.Provider == "KubeDNS" || kubeDNS.Provider == "" {

		{
			key := "kube-dns.addons.k8s.io"
			version := "1.14.10"

			{
				location := key + "/pre-k8s-1.6.yaml"
				id := "pre-k8s-1.6"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          map[string]string{"k8s-addon": key},
					Manifest:          fi.String(location),
					KubernetesVersion: "<1.6.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}

			{
				location := key + "/k8s-1.6.yaml"
				id := "k8s-1.6"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          map[string]string{"k8s-addon": key},
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.6.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		}
	}

	if kubeDNS.Provider == "CoreDNS" {
		{
			key := "coredns.addons.k8s.io"
			version := "1.3.0-kops.1"

			{
				location := key + "/k8s-1.6.yaml"
				id := "k8s-1.6"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          map[string]string{"k8s-addon": key},
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.6.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		}
	}

	// @check if node authorization or bootstrap tokens are enabled an if so we can forgo applying
	// this manifest. For clusters whom are upgrading from RBAC to Node,RBAC the clusterrolebinding
	// will remain and have to be deleted manually once all the nodes have been upgraded.
	enableRBACAddon := true
	if b.cluster.Spec.NodeAuthorization != nil {
		enableRBACAddon = false
	}
	if b.cluster.Spec.KubeAPIServer != nil {
		if b.cluster.Spec.KubeAPIServer.EnableBootstrapAuthToken != nil && *b.cluster.Spec.KubeAPIServer.EnableBootstrapAuthToken == true {
			enableRBACAddon = false
		}
	}

	if enableRBACAddon {
		{
			key := "rbac.addons.k8s.io"
			version := "1.8.0"

			{
				location := key + "/k8s-1.8.yaml"
				id := "k8s-1.8"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          map[string]string{"k8s-addon": key},
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.8.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		}
	}

	{
		// Adding the kubelet-api-admin binding: this is required when switching to webhook authorization on the kubelet
		// docs: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#other-component-roles
		// issue: https://github.com/kubernetes/kops/issues/5176
		key := "kubelet-api.rbac.addons.k8s.io"
		version := "v0.0.1"

		{
			location := key + "/k8s-1.9.yaml"
			id := "k8s-1.9"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.9.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	{
		key := "limit-range.addons.k8s.io"
		version := "1.5.0"
		location := key + "/v" + version + ".yaml"

		addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
			Name:     fi.String(key),
			Version:  fi.String(version),
			Selector: map[string]string{"k8s-addon": key},
			Manifest: fi.String(location),
		})
		manifests[key] = "addons/" + location
	}

	// @check the dns-controller has not been disabled
	externalDNS := b.cluster.Spec.ExternalDNS
	if externalDNS == nil || !externalDNS.Disable {
		{
			key := "dns-controller.addons.k8s.io"
			version := "1.12.0-alpha.1"

			{
				location := key + "/pre-k8s-1.6.yaml"
				id := "pre-k8s-1.6"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          map[string]string{"k8s-addon": key},
					Manifest:          fi.String(location),
					KubernetesVersion: "<1.6.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}

			{
				location := key + "/k8s-1.6.yaml"
				id := "k8s-1.6"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          map[string]string{"k8s-addon": key},
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.6.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		}
	}

	if featureflag.EnableExternalDNS.Enabled() {
		{
			key := "external-dns.addons.k8s.io"
			version := "0.4.4"

			{
				location := key + "/pre-k8s-1.6.yaml"
				id := "pre-k8s-1.6"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          map[string]string{"k8s-addon": key},
					Manifest:          fi.String(location),
					KubernetesVersion: "<1.6.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}

			{
				location := key + "/k8s-1.6.yaml"
				id := "k8s-1.6"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          map[string]string{"k8s-addon": key},
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.6.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		}
	}

	if kops.CloudProviderID(b.cluster.Spec.CloudProvider) == kops.CloudProviderAWS {
		key := "storage-aws.addons.k8s.io"
		version := "1.7.0"

		{
			id := "v1.7.0"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.7.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			id := "v1.6.0"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: "<1.7.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if kops.CloudProviderID(b.cluster.Spec.CloudProvider) == kops.CloudProviderDO {
		key := "digitalocean-cloud-controller.addons.k8s.io"
		version := "1.8"

		{
			id := "k8s-1.8"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.8.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if kops.CloudProviderID(b.cluster.Spec.CloudProvider) == kops.CloudProviderGCE {
		key := "storage-gce.addons.k8s.io"
		version := "1.7.0"

		{
			id := "v1.6.0"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: "<1.7.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			id := "v1.7.0"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.7.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if featureflag.Spotinst.Enabled() {
		key := "spotinst-kubernetes-cluster-controller.addons.k8s.io"
		version := "1.0.18"

		{
			id := "v1.8.0"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: "<1.9.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			id := "v1.9.0"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.9.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	// The role.kubernetes.io/networking is used to label anything related to a networking addin,
	// so that if we switch networking plugins (e.g. calico -> weave or vice-versa), we'll replace the
	// old networking plugin, and there won't be old pods "floating around".

	// This means whenever we create or update a networking plugin, we should be sure that:
	// 1. the selector is role.kubernetes.io/networking=1
	// 2. every object in the manifest is labeleled with role.kubernetes.io/networking=1

	// TODO: Some way to test/enforce this?

	// TODO: Create "empty" configurations for others, so we can delete e.g. the kopeio configuration
	// if we switch to kubenet?

	// TODO: Create configuration object for cni providers (maybe create it but orphan it)?

	// NOTE: we try to suffix with -kops.1, so that we can increment versions even if the upstream version
	// hasn't changed.  The problem with semver is that there is nothing > 1.0.0 other than 1.0.1-pre.1
	networkingSelector := map[string]string{"role.kubernetes.io/networking": "1"}

	if b.cluster.Spec.Networking.Kopeio != nil {
		key := "networking.kope.io"
		version := "1.0.20181028-kops.1"

		{
			location := key + "/pre-k8s-1.6.yaml"
			id := "pre-k8s-1.6"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: "<1.6.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			location := key + "/k8s-1.6.yaml"
			id := "k8s-1.6"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.6.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if b.cluster.Spec.Networking.Weave != nil {
		key := "networking.weave"
		versions := map[string]string{
			"pre-k8s-1.6": "2.3.0-kops.2",
			"k8s-1.6":     "2.3.0-kops.2",
			"k8s-1.7":     "2.5.0-kops.1",
			"k8s-1.8":     "2.5.0-kops.1",
		}

		{
			location := key + "/pre-k8s-1.6.yaml"
			id := "pre-k8s-1.6"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(versions[id]),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: "<1.6.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			location := key + "/k8s-1.6.yaml"
			id := "k8s-1.6"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(versions[id]),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.6.0 <1.7.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			location := key + "/k8s-1.7.yaml"
			id := "k8s-1.7"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(versions[id]),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.7.0 <1.8.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			location := key + "/k8s-1.8.yaml"
			id := "k8s-1.8"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(versions[id]),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.8.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if b.cluster.Spec.Networking.Flannel != nil {
		key := "networking.flannel"
		version := "0.10.0-kops.1"

		{
			location := key + "/pre-k8s-1.6.yaml"
			id := "pre-k8s-1.6"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: "<1.6.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			location := key + "/k8s-1.6.yaml"
			id := "k8s-1.6"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.6.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if b.cluster.Spec.Networking.Calico != nil {
		key := "networking.projectcalico.org"
		versions := map[string]string{
			"pre-k8s-1.6": "2.4.2-kops.1",
			"k8s-1.6":     "2.6.9-kops.1",
			"k8s-1.7":     "2.6.9-kops.1",
			"k8s-1.7-v3":  "3.4.0-kops.3",
		}

		if b.cluster.Spec.Networking.Calico.MajorVersion == "v3" {
			{
				id := "k8s-1.7-v3"
				location := key + "/" + id + ".yaml"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(versions[id]),
					Selector:          networkingSelector,
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.7.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		} else {
			{
				id := "pre-k8s-1.6"
				location := key + "/" + id + ".yaml"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(versions[id]),
					Selector:          networkingSelector,
					Manifest:          fi.String(location),
					KubernetesVersion: "<1.6.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}

			{
				id := "k8s-1.6"
				location := key + "/" + id + ".yaml"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(versions[id]),
					Selector:          networkingSelector,
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.6.0 <1.7.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}

			{
				id := "k8s-1.7"
				location := key + "/" + id + ".yaml"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(versions[id]),
					Selector:          networkingSelector,
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.7.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		}
	}

	if b.cluster.Spec.Networking.Canal != nil {
		key := "networking.projectcalico.org.canal"
		// 2.6.3-kops.1 = 2.6.2 with kops manifest tweaks.  This should go away with the next version bump.
		versions := map[string]string{
			"pre-k8s-1.6": "2.4.2-kops.2",
			"k8s-1.6":     "2.4.2-kops.2",
			"k8s-1.8":     "2.6.7-kops.3",
			"k8s-1.9":     "3.2.3-kops.1",
			"k8s-1.12":    "3.3.0-kops.1",
		}
		{
			id := "pre-k8s-1.6"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(versions[id]),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: "<1.6.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			id := "k8s-1.6"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(versions[id]),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.6.0 <1.8.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			id := "k8s-1.8"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(versions[id]),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.8.0 <1.9.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
		{
			id := "k8s-1.9"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(versions[id]),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.9.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
		{
			id := "k8s-1.12"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(versions[id]),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.12.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if b.cluster.Spec.Networking.Kuberouter != nil {
		key := "networking.kuberouter"
		version := "0.1.1-kops.3"

		{
			location := key + "/k8s-1.6.yaml"
			id := "k8s-1.6"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.6.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if b.cluster.Spec.Networking.Romana != nil {
		key := "networking.romana"
		version := "v2.0.2-kops.2"

		{
			location := key + "/k8s-1.7.yaml"
			id := "k8s-1.7"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.7.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if b.cluster.Spec.Networking.AmazonVPC != nil {
		key := "networking.amazon-vpc-routed-eni"
		version := "1.3.0-kops.1"

		{
			id := "k8s-1.7"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.7.0 <1.8.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			id := "k8s-1.8"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.8.0 <1.10.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}

		{
			id := "k8s-1.10"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.10.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if b.cluster.Spec.Networking.Cilium != nil {
		key := "networking.cilium.io"
		version := "v1.0-kops.2"

		{
			id := "k8s-1.7"
			location := key + "/" + id + ".yaml"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          networkingSelector,
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.7.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	authenticationSelector := map[string]string{"role.kubernetes.io/authentication": "1"}

	if b.cluster.Spec.Authentication != nil {
		if b.cluster.Spec.Authentication.Kopeio != nil {
			key := "authentication.kope.io"
			version := "1.0.20171125"

			{
				location := key + "/k8s-1.8.yaml"
				id := "k8s-1.8"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          authenticationSelector,
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.8.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		}
		if b.cluster.Spec.Authentication.Aws != nil {
			key := "authentication.aws"
			version := "0.3.0"

			{
				location := key + "/k8s-1.10.yaml"
				id := "k8s-1.10"

				addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
					Name:              fi.String(key),
					Version:           fi.String(version),
					Selector:          authenticationSelector,
					Manifest:          fi.String(location),
					KubernetesVersion: ">=1.10.0",
					Id:                id,
				})
				manifests[key+"-"+id] = "addons/" + location
			}
		}
	}

	if featureflag.EnableExternalCloudController.Enabled() && b.cluster.Spec.ExternalCloudControllerManager != nil {
		{
			key := "core.addons.k8s.io"
			version := "1.7.0"

			location := key + "/k8s-1.7.yaml"
			id := "k8s-1.7-ccm"

			addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
				Name:              fi.String(key),
				Version:           fi.String(version),
				Selector:          map[string]string{"k8s-addon": key},
				Manifest:          fi.String(location),
				KubernetesVersion: ">=1.7.0",
				Id:                id,
			})
			manifests[key+"-"+id] = "addons/" + location
		}
	}

	if b.cluster.Spec.KubeScheduler.UsePolicyConfigMap != nil {
		key := "scheduler.addons.k8s.io"
		version := "1.7.0"
		location := key + "/v" + version + ".yaml"

		addons.Spec.Addons = append(addons.Spec.Addons, &channelsapi.AddonSpec{
			Name:     fi.String(key),
			Version:  fi.String(version),
			Selector: map[string]string{"k8s-addon": key},
			Manifest: fi.String(location),
		})
		manifests[key] = "addons/" + location
	}

	return addons, manifests, nil
}
