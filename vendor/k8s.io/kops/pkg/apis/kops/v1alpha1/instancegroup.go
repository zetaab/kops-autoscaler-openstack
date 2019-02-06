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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// InstanceGroup represents a group of instances (either nodes or masters) with the same configuration
type InstanceGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec InstanceGroupSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// InstanceGroupList is a collection in instancegroups
type InstanceGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []InstanceGroup `json:"items"`
}

// InstanceGroupRole string describes the roles of the nodes in this InstanceGroup (master or nodes)
type InstanceGroupRole string

const (
	// InstanceGroupRoleMaster defines the node as a master
	InstanceGroupRoleMaster InstanceGroupRole = "Master"
	// InstanceGroupRoleNode defines the node as a worker
	InstanceGroupRoleNode InstanceGroupRole = "Node"
	// InstanceGroupRoleBastion defines the node as a bastion host
	InstanceGroupRoleBastion InstanceGroupRole = "Bastion"
)

// AllInstanceGroupRoles is a slice of all valid InstanceGroupRole values
var AllInstanceGroupRoles = []InstanceGroupRole{
	InstanceGroupRoleNode,
	InstanceGroupRoleMaster,
	InstanceGroupRoleBastion,
}

const (
	// BtfsFilesystem indicates a btfs filesystem
	BtfsFilesystem = "btfs"
	// Ext4Filesystem indicates a ext3 filesystem
	Ext4Filesystem = "ext4"
	// XFSFilesystem indicates a xfs filesystem
	XFSFilesystem = "xfs"
)

var (
	// SupportedFilesystems is a list of supported filesystems to format as
	SupportedFilesystems = []string{BtfsFilesystem, Ext4Filesystem, XFSFilesystem}
)

// InstanceGroupSpec is the specification for a instanceGroup
type InstanceGroupSpec struct {
	// Type determines the role of instances in this group: masters or nodes
	Role InstanceGroupRole `json:"role,omitempty"`
	// Image is the instance (ami etc) we should use
	Image string `json:"image,omitempty"`
	// MinSize is the minimum size of the pool
	MinSize *int32 `json:"minSize,omitempty"`
	// MaxSize is the maximum size of the pool
	MaxSize *int32 `json:"maxSize,omitempty"`
	// MachineType is the instance class
	MachineType string `json:"machineType,omitempty"`
	// RootVolumeSize is the size of the EBS root volume to use, in GB
	RootVolumeSize *int32 `json:"rootVolumeSize,omitempty"`
	// RootVolumeType is the type of the EBS root volume to use (e.g. gp2)
	RootVolumeType *string `json:"rootVolumeType,omitempty"`
	// If volume type is io1, then we need to specify the number of Iops.
	RootVolumeIops *int32 `json:"rootVolumeIops,omitempty"`
	// RootVolumeOptimization enables EBS optimization for an instance
	RootVolumeOptimization *bool `json:"rootVolumeOptimization,omitempty"`
	// Volumes is a collection of additional volumes to create for instances within this InstanceGroup
	Volumes []*VolumeSpec `json:"volumes,omitempty"`
	// VolumeMounts a collection of volume mounts
	VolumeMounts []*VolumeMountSpec `json:"volumeMounts,omitempty"`
	// Hooks is a list of hooks for this instanceGroup, note: these can override the cluster wide ones if required
	Hooks []HookSpec `json:"hooks,omitempty"`
	// MaxPrice indicates this is a spot-pricing group, with the specified value as our max-price bid
	MaxPrice *string `json:"maxPrice,omitempty"`
	// AssociatePublicIP is true if we want instances to have a public IP
	AssociatePublicIP *bool `json:"associatePublicIp,omitempty"`
	// AdditionalSecurityGroups attaches additional security groups (e.g. i-123456)
	AdditionalSecurityGroups []string `json:"additionalSecurityGroups,omitempty"`
	// CloudLabels indicates the labels for instances in this group, at the AWS level
	CloudLabels map[string]string `json:"cloudLabels,omitempty"`
	// NodeLabels indicates the kubernetes labels for nodes in this group
	NodeLabels map[string]string `json:"nodeLabels,omitempty"`
	// A collection of files assets for deployed cluster wide
	FileAssets []FileAssetSpec `json:"fileAssets,omitempty"`
	// Describes the tenancy of the instance group. Can be either default or dedicated.
	// Currently only applies to AWS.
	Tenancy string `json:"tenancy,omitempty"`
	// Kubelet overrides kubelet config from the ClusterSpec
	Kubelet *KubeletConfigSpec `json:"kubelet,omitempty"`
	// Taints indicates the kubernetes taints for nodes in this group
	Taints []string `json:"taints,omitempty"`
	// AdditionalUserData is any additional user-data to be passed to the host
	AdditionalUserData []UserData `json:"additionalUserData,omitempty"`
	// Zones is the names of the Zones where machines in this instance group should be placed
	// This is needed for regional subnets (e.g. GCE), to restrict placement to particular zones
	Zones []string `json:"zones,omitempty"`
	// SuspendProcesses disables the listed Scaling Policies
	SuspendProcesses []string `json:"suspendProcesses,omitempty"`
	// ExternalLoadBalancers define loadbalancers that should be attached to the instancegroup
	ExternalLoadBalancers []LoadBalancer `json:"externalLoadBalancers,omitempty"`
	// DetailedInstanceMonitoring defines if detailed-monitoring is enabled (AWS only)
	DetailedInstanceMonitoring *bool `json:"detailedInstanceMonitoring,omitempty"`
	// IAMProfileSpec defines the identity of the cloud group IAM profile (AWS only).
	IAM *IAMProfileSpec `json:"iam,omitempty"`
	// SecurityGroupOverride overrides the default security group created by Kops for this IG (AWS only).
	SecurityGroupOverride *string `json:"securityGroupOverride,omitempty"`
}

// IAMProfileSpec is the AWS IAM Profile to attach to instances in this instance
// group. Specify the ARN for the IAM instance profile (AWS only).
type IAMProfileSpec struct {
	// Profile of the cloud group IAM profile. In aws this is the arn
	// for the iam instance profile
	Profile *string `json:"profile,omitempty"`
}

// UserData defines a user-data section
type UserData struct {
	// Name is the name of the user-data
	Name string `json:"name,omitempty"`
	// Type is the type of user-data
	Type string `json:"type,omitempty"`
	// Content is the user-data content
	Content string `json:"content,omitempty"`
}

// VolumeSpec defined the spec for an additional volume attached to the instance group
type VolumeSpec struct {
	// Device is an optional device name of the block device
	Device string `json:"device,omitempty"`
	// Encrypted indicates you want to encrypt the volume
	Encrypted *bool `json:"encrypted,omitempty"`
	// Iops is the provision iops for this iops (think io1 in aws)
	Iops *int64 `json:"iops,omitempty"`
	// Size is the size of the volume in GB
	Size int64 `json:"size,omitempty"`
	// Type is the type of volume to create and is cloud specific
	Type string `json:"type,omitempty"`
}

// VolumeMountSpec defines the specification for mounting a device
type VolumeMountSpec struct {
	// Device is the device name to provision and mount
	Device string `json:"device,omitempty"`
	// Filesystem is the filesystem to mount
	Filesystem string `json:"filesystem,omitempty"`
	// FormatOptions is a collection of options passed when formatting the device
	FormatOptions []string `json:"formatOptions,omitempty"`
	// MountOptions is a collection of mount options
	MountOptions []string `json:"mountOptions,omitempty"`
	// Path is the location to mount the device
	Path string `json:"path,omitempty"`
}

// Ext4FileSystemSpec defines a specification for a ext4 filesystem on a instancegroup volume
type Ext4FileSystemSpec struct{}

// LoadBalancer defines a load balancer
type LoadBalancer struct {
	// LoadBalancerName to associate with this instance group (AWS ELB)
	LoadBalancerName *string `json:"loadBalancerName,omitempty"`
	// TargetGroupARN to associate with this instance group (AWS ALB/NLB)
	TargetGroupARN *string `json:"targetGroupArn,omitempty"`
}
