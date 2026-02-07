/*
Copyright 2024 ayoy.

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

// LocalSecretRef references a Secret in the same namespace as the referencing resource.
type LocalSecretRef struct {
	// Name of the secret resource.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// RepositoryMirrorSpec defines the desired state of RepositoryMirror.
type RepositoryMirrorSpec struct {
	// ConnSecretRef references a Secret with `host`, `token`, and optional `validateCerts` keys.
	// The Secret must be in the same namespace as the RepositoryMirror resource.
	ConnSecretRef LocalSecretRef `json:"connSecretRef"`

	// Name is the repository in "namespace/shortname" format.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][a-zA-Z0-9._-]*/[a-zA-Z0-9][a-zA-Z0-9._-]*$`
	Name string `json:"name"`

	// ExternalReference is the path to the remote container repository to synchronize.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	ExternalReference string `json:"externalReference,omitempty"`

	// RobotUsername is the robot account used for synchronization.
	// +optional
	// +kubebuilder:validation:MaxLength=256
	RobotUsername string `json:"robotUsername,omitempty"`

	// IsEnabled defines whether the mirror configuration is active.
	// +optional
	IsEnabled *bool `json:"isEnabled,omitempty"`

	// ImageTags is a list of image tags to be synchronized from the remote repository.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	ImageTags []string `json:"imageTags,omitempty"`

	// SyncInterval is the synchronization interval. Supports s/m/h/d/w suffixes. Defaults to "86400s".
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9]+[smhdw]?$`
	SyncInterval string `json:"syncInterval,omitempty"`

	// SyncStartDate is the ISO 8601 UTC date/time for the first synchronization.
	// +optional
	// +kubebuilder:validation:MaxLength=64
	SyncStartDate string `json:"syncStartDate,omitempty"`

	// ExternalRegistryCredentialsRef references a Secret with `username` and `password` keys
	// for pulling from the external registry.
	// +optional
	ExternalRegistryCredentialsRef *LocalSecretRef `json:"externalRegistryCredentialsRef,omitempty"`

	// HttpProxy is the HTTP proxy for accessing the remote container registry.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	HttpProxy string `json:"httpProxy,omitempty"`

	// HttpsProxy is the HTTPS proxy for accessing the remote container registry.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	HttpsProxy string `json:"httpsProxy,omitempty"`

	// NoProxy is a comma-separated list of hosts for which the proxy should not be used.
	// +optional
	// +kubebuilder:validation:MaxLength=2048
	NoProxy string `json:"noProxy,omitempty"`

	// VerifyTls defines whether TLS of the external registry should be verified. Defaults to true.
	// +optional
	VerifyTls *bool `json:"verifyTls,omitempty"`

	// UnsignedImages allows unsigned images to be mirrored.
	// +optional
	UnsignedImages *bool `json:"unsignedImages,omitempty"`

	// ForceSync triggers an immediate image synchronization.
	// +optional
	ForceSync *bool `json:"forceSync,omitempty"`

	// PreserveInQuayOnDeletion keeps the mirror configuration in Quay when the CR is deleted.
	// +optional
	PreserveInQuayOnDeletion *bool `json:"preserveInQuayOnDeletion,omitempty"`
}

// RepositoryMirrorStatus defines the observed state of RepositoryMirror.
type RepositoryMirrorStatus struct {
	// Conditions represent the latest available observations of the resource's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ExistInQuay indicates whether the mirror configuration exists in Quay.
	// +optional
	ExistInQuay bool `json:"existInQuay,omitempty"`

	// Message provides additional information about the current state.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=repmirror
// +kubebuilder:printcolumn:name="Quay Name",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Success",type="string",JSONPath=".status.conditions[?(@.type=='Successful')].status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message",priority=1
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.conditions[?(@.type=='Running')].reason"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RepositoryMirror is the Schema for the repositorymirrors API.
type RepositoryMirror struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RepositoryMirrorSpec   `json:"spec,omitempty"`
	Status RepositoryMirrorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RepositoryMirrorList contains a list of RepositoryMirror.
type RepositoryMirrorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RepositoryMirror `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RepositoryMirror{}, &RepositoryMirrorList{})
}
