// Copyright 2025 The PipeCD Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package config contains configuration structures for the Cloud Run plugin.
//
// These structures define how the plugin is configured in piped.yaml and
// how applications specify their deployment settings.
package config

// PluginConfig defines the plugin-level configuration in piped config.
// This is specified under the `plugins` section in piped.yaml.
//
// Example piped.yaml configuration:
//
//	plugins:
//	  - name: cloudrun
//	    port: 7001
//	    url: https://github.com/your-org/plugin/releases/download/v0.1.0/plugin_cloudrun_linux_amd64
//	    config:
//	      projectID: my-default-project
//	      region: us-central1
//	    deployTargets:
//	      - name: staging
//	        config:
//	          projectID: staging-project
//	          region: us-central1
//	          credentialsFile: /etc/piped/gcp-staging-key.json
//	      - name: production
//	        config:
//	          projectID: production-project
//	          region: us-east1
//	          credentialsFile: /etc/piped/gcp-prod-key.json
//
type PluginConfig struct {
	// ProjectID is the default GCP project ID for Cloud Run services.
	// This can be overridden per deploy target.
	// Example: "my-gcp-project"
	ProjectID string `json:"projectID"`

	// Region is the default GCP region for Cloud Run services.
	// This can be overridden per deploy target.
	// Example: "us-central1"
	Region string `json:"region"`

	// CredentialsFile is the path to the GCP service account key file.
	// If not specified, the plugin will use Application Default Credentials.
	// Example: "/etc/piped/gcp-key.json"
	CredentialsFile string `json:"credentialsFile"`
}

// DeployTargetConfig defines deploy target specific configuration.
// Each deploy target represents a different environment (staging, production, etc.)
// with its own GCP project and settings.
type DeployTargetConfig struct {
	// Name is the identifier for this deploy target.
	// Example: "staging", "production"
	Name string `json:"name"`

	// ProjectID is the GCP project ID for this deploy target.
	// Overrides the plugin-level projectID if specified.
	ProjectID string `json:"projectID"`

	// Region is the GCP region for this deploy target.
	// Overrides the plugin-level region if specified.
	Region string `json:"region"`

	// CredentialsFile is the path to the GCP service account key file.
	// Overrides the plugin-level credentialsFile if specified.
	CredentialsFile string `json:"credentialsFile"`
}
