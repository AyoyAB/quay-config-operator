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

// ConnSecretRef references a Secret containing Quay connection details.
type ConnSecretRef struct {
	// Name of the secret resource.
	Name string `json:"name"`

	// Namespace of the secret resource. Defaults to the namespace of the RepositoryMirror resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RepositoryMirrorSpec defines the desired state of RepositoryMirror.
type RepositoryMirrorSpec struct {
	// ConnSecretRef references a Secret with `host`, `token`, and optional `validateCerts` keys.
	ConnSecretRef ConnSecretRef `json:"connSecretRef"`

	// Name is the repository in "namespace/shortname" format.
	Name string `json:"name"`

	// ExternalReference is the path to the remote container repository to synchronize.
	// +optional
	ExternalReference string `json:"externalReference,omitempty"`

	// RobotUsername is the robot account used for synchronization.
	// +optional
	RobotUsername string `json:"robotUsername,omitempty"`

	// IsEnabled defines whether the mirror configuration is active.
	// +optional
	IsEnabled *bool `json:"isEnabled,omitempty"`

	// ImageTags is a list of image tags to be synchronized from the remote repository.
	// +optional
	ImageTags []string `json:"imageTags,omitempty"`

	// SyncInterval is the synchronization interval. Supports s/m/h/d/w suffixes. Defaults to "86400s".
	// +optional
	SyncInterval string `json:"syncInterval,omitempty"`

	// SyncStartDate is the ISO 8601 UTC date/time for the first synchronization.
	// +optional
	SyncStartDate string `json:"syncStartDate,omitempty"`

	// ExternalRegistryUsername is the username for pulling from the remote registry.
	// +optional
	ExternalRegistryUsername string `json:"externalRegistryUsername,omitempty"`

	// ExternalRegistryPassword is the password for pulling from the remote registry.
	// +optional
	ExternalRegistryPassword string `json:"externalRegistryPassword,omitempty"`

	// HttpProxy is the HTTP proxy for accessing the remote container registry.
	// +optional
	HttpProxy string `json:"httpProxy,omitempty"`

	// HttpsProxy is the HTTPS proxy for accessing the remote container registry.
	// +optional
	HttpsProxy string `json:"httpsProxy,omitempty"`

	// NoProxy is a comma-separated list of hosts for which the proxy should not be used.
	// +optional
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
