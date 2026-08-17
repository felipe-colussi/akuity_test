/*
Copyright 2026 felipe_colussi.

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

package v1

import (
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NamespaceClassSpec defines the desired state of NamespaceClass
type NamespaceClassSpec struct {
	// +optional
	NetworkPolicies []NetworkPolicyTemplate `json:"networkPolicies,omitempty"`
	// +optional
	ConfigMaps []ConfigMapTemplate `json:"configMaps,omitempty"`
	// +optional
	ClusterRoles []ClusterRoleTemplate `json:"clusterRoles,omitempty"`

	// +optional
	// AnyApproach is the generic implementation that I disagree on.
	// Just adding the example. If this was a real case I would defend this to not be taken.
	AnyApproach []TargetResource `json:"anyApproach,omitempty"`
}

// TargetResource Is how I would implement the generic implementation suggested on the code
// I DO NOT LIKE THIS IDEA AND SOME EXTRA CODE CHANGES WOULD NEED TO BE DONE.
// ADDING TO THE CODE AS AN EXAMPLE SO I SHOW THE REQUESTED APPROACH (That I hardly disagree on).
type TargetResource struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`

	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	// Metadata holds the raw, arbitrary JSON payload for the target resource's metadata (name, labels, etc.)
	Metadata runtime.RawExtension `json:"metadata"`

	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	// Spec holds the raw, arbitrary JSON payload for the target resource's spec
	Spec runtime.RawExtension `json:"spec"`
}

type NetworkPolicyTemplate struct {
	// +optional
	Name string `json:"name"`
	// spec represents the specification of the desired behavior for this NetworkPolicy.
	// +optional
	Spec networkingv1.NetworkPolicySpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

func (n NetworkPolicyTemplate) GetName() string {
	return n.Name
}

type ClusterRoleTemplate struct {
	// +optional
	Name string `json:"name"`
	// Rules holds all the PolicyRules for this ClusterRole
	// +optional
	// +listType=atomic
	// +k8s:alpha(since: "1.36")=+k8s:optional
	Rules []rbacv1.PolicyRule `json:"rules" protobuf:"bytes,2,rep,name=rules"`

	// AggregationRule is an optional field that describes how to build the Rules for this ClusterRole.
	// If AggregationRule is set, then the Rules are controller managed and direct changes to Rules will be
	// stomped by the controller.
	// +optional
	AggregationRule *rbacv1.AggregationRule `json:"aggregationRule,omitempty" protobuf:"bytes,3,opt,name=aggregationRule"`
}

func (c ClusterRoleTemplate) GetName() string {
	return c.Name
}

type ConfigMapTemplate struct {
	// +optional
	Name string `json:"name"`

	// Immutable, if set to true, ensures that data stored in the ConfigMap cannot
	// be updated (only object metadata can be modified).
	// If not set to true, the field can be modified at any time.
	// Defaulted to nil.
	// +optional
	Immutable *bool `json:"immutable,omitempty" protobuf:"varint,4,opt,name=immutable"`

	// Data contains the configuration data.
	// Each key must consist of alphanumeric characters, '-', '_' or '.'.
	// Values with non-UTF-8 byte sequences must use the BinaryData field.
	// The keys stored in Data must not overlap with the keys in
	// the BinaryData field, this is enforced during validation process.
	// +optional
	Data map[string]string `json:"data,omitempty" protobuf:"bytes,2,rep,name=data"`

	// BinaryData contains the binary data.
	// Each key must consist of alphanumeric characters, '-', '_' or '.'.
	// BinaryData can contain byte sequences that are not in the UTF-8 range.
	// The keys stored in BinaryData must not overlap with the ones in
	// the Data field, this is enforced during validation process.
	// Using this field will require 1.10+ apiserver and
	// kubelet.
	// +optional
	BinaryData map[string][]byte `json:"binaryData,omitempty" protobuf:"bytes,3,rep,name=binaryData"`
}

func (c ConfigMapTemplate) GetName() string {
	return c.Name
}

// NamespaceClassStatus defines the observed state of NamespaceClass.
type NamespaceClassStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the NamespaceClass resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NamespaceClass is the Schema for the namespaceclasses API
type NamespaceClass struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NamespaceClass
	// +required
	Spec NamespaceClassSpec `json:"spec"`

	// status defines the observed state of NamespaceClass
	// +optional
	Status NamespaceClassStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NamespaceClassList contains a list of NamespaceClass
type NamespaceClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NamespaceClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &NamespaceClass{}, &NamespaceClassList{})
		return nil
	})
}
