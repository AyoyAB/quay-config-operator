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

package quay

// MirrorConfig represents the GET response from the Quay mirror API.
type MirrorConfig struct {
	IsEnabled                bool                   `json:"is_enabled"`
	ExternalReference        string                 `json:"external_reference"`
	ExternalRegistryConfig   ExternalRegistryConfig `json:"external_registry_config"`
	SyncInterval             int                    `json:"sync_interval"`
	SyncStartDate            string                 `json:"sync_start_date"`
	RobotUsername            string                 `json:"robot_username"`
	RootRule                 RootRule               `json:"root_rule"`
	ExternalRegistryUsername *string                `json:"external_registry_username,omitempty"`
}

// ExternalRegistryConfig holds external registry configuration from the Quay API.
type ExternalRegistryConfig struct {
	VerifyTLS      *bool  `json:"verify_tls,omitempty"`
	UnsignedImages *bool  `json:"unsigned_images,omitempty"`
	Proxy          *Proxy `json:"proxy,omitempty"`
}

// Proxy holds proxy configuration for external registry access.
type Proxy struct {
	HTTPProxy  string `json:"http_proxy,omitempty"`
	HTTPSProxy string `json:"https_proxy,omitempty"`
	NoProxy    string `json:"no_proxy,omitempty"`
}

// RootRule holds the tag matching rule for mirroring.
type RootRule struct {
	RuleKind  string   `json:"rule_kind"`
	RuleValue []string `json:"rule_value"`
}

// MirrorCreateRequest is the POST body for creating a mirror configuration.
type MirrorCreateRequest struct {
	IsEnabled                bool                    `json:"is_enabled"`
	ExternalReference        string                  `json:"external_reference"`
	ExternalRegistryConfig   *ExternalRegistryConfig `json:"external_registry_config,omitempty"`
	SyncInterval             int                     `json:"sync_interval"`
	SyncStartDate            string                  `json:"sync_start_date"`
	RobotUsername            string                  `json:"robot_username"`
	RootRule                 *RootRule               `json:"root_rule,omitempty"`
	ExternalRegistryUsername string                  `json:"external_registry_username,omitempty"`
	ExternalRegistryPassword string                  `json:"external_registry_password,omitempty"`
}

// MirrorUpdateRequest is the PUT body for updating a mirror configuration.
type MirrorUpdateRequest struct {
	IsEnabled                *bool                   `json:"is_enabled,omitempty"`
	ExternalReference        string                  `json:"external_reference,omitempty"`
	ExternalRegistryConfig   *ExternalRegistryConfig `json:"external_registry_config,omitempty"`
	SyncInterval             *int                    `json:"sync_interval,omitempty"`
	SyncStartDate            string                  `json:"sync_start_date,omitempty"`
	RobotUsername            string                  `json:"robot_username,omitempty"`
	RootRule                 *RootRule               `json:"root_rule,omitempty"`
	ExternalRegistryUsername string                  `json:"external_registry_username,omitempty"`
	ExternalRegistryPassword string                  `json:"external_registry_password,omitempty"`
}
