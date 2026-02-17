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

package config

// ApplicationConfig defines the application-specific configuration.
// This is specified in the application's .pipe.yaml file.
//
// Example .pipe.yaml configuration:
//
//	apiVersion: pipecd.dev/v1beta1
//	kind: CloudRunApp
//	spec:
//	  name: my-cloudrun-app
//	  input:
//	    serviceManifestPath: service.yaml
//	    image: gcr.io/my-project/my-app:v1.0.0
//	  pipeline:
//	    stages:
//	      - name: CLOUDRUN_SYNC
//	        with:
//	          skipTrafficShift: true
//	      - name: CLOUDRUN_PROMOTE
//	        with:
//	          percent: 10
//	      - name: WAIT
//	        with:
//	          duration: 5m
//	      - name: CLOUDRUN_PROMOTE
//	        with:
//	          percent: 100
type ApplicationConfig struct {
	// Name is the name of the application.
	Name string `json:"name"`

	// Labels are key-value pairs for organizing applications.
	Labels map[string]string `json:"labels,omitempty"`

	// ServiceManifestPath is the path to the Cloud Run service manifest file
	// relative to the application directory.
	// Default: "service.yaml"
	ServiceManifestPath string `json:"serviceManifestPath"`

	// Input configuration for the deployment.
	Input InputConfig `json:"input"`

	// QuickSync defines the quick sync strategy options.
	// Used when no pipeline is specified.
	QuickSync *QuickSyncConfig `json:"quickSync,omitempty"`

	// PipelineSync defines the pipeline sync strategy options.
	// Used when a custom pipeline is specified.
	PipelineSync *PipelineSyncConfig `json:"pipelineSync,omitempty"`
}

// InputConfig defines input parameters for Cloud Run deployment.
type InputConfig struct {
	// ServiceName is the name of the Cloud Run service.
	// If not specified, the service name from the manifest is used.
	ServiceName string `json:"serviceName,omitempty"`

	// Image is the container image to deploy.
	// This overrides the image specified in the service manifest.
	// Example: "gcr.io/my-project/my-app:v1.0.0"
	Image string `json:"image,omitempty"`

	// ProjectID is the GCP project ID.
	// This overrides the deploy target configuration.
	ProjectID string `json:"projectID,omitempty"`

	// Region is the GCP region.
	// This overrides the deploy target configuration.
	Region string `json:"region,omitempty"`
}

// QuickSyncConfig defines quick sync strategy options.
// Quick sync deploys the new revision and immediately routes 100% traffic to it.
type QuickSyncConfig struct {
	// Prune indicates whether to remove unused revisions after deployment.
	Prune bool `json:"prune"`
}

// PipelineSyncConfig defines pipeline sync strategy options.
// Pipeline sync allows for progressive delivery with custom stages.
type PipelineSyncConfig struct {
	// Stages defines the deployment pipeline stages.
	Stages []PipelineStage `json:"stages,omitempty"`
}

// PipelineStage defines a single stage in the deployment pipeline.
type PipelineStage struct {
	// Name is the stage name.
	// Supported stages: CLOUDRUN_SYNC, CLOUDRUN_PROMOTE, CLOUDRUN_ROLLBACK, CLOUDRUN_CANARY_CLEANUP
	Name string `json:"name"`

	// With contains stage-specific configuration.
	With map[string]interface{} `json:"with,omitempty"`
}
