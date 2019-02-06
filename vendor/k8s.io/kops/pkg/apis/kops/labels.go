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

package kops

// AnnotationNameManagement is the annotation that indicates that a cluster is under external or non-standard management
const AnnotationNameManagement = "kops.kubernetes.io/management"

// AnnotationValueManagementImported is the annotation value that indicates a cluster was imported, typically as part of an upgrade
const AnnotationValueManagementImported = "imported"

// UpdatePolicyExternal is a value for ClusterSpec.UpdatePolicy indicating that upgrades are done externally, and we should disable automatic upgrades
const UpdatePolicyExternal = "external"
