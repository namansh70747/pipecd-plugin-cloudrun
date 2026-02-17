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

// Package main is the entry point for the PipeCD Cloud Run Plugin.
//
// This plugin enables PipeCD to deploy applications to Google Cloud Run
// with support for progressive delivery strategies like canary deployments
// and traffic splitting.
//
// Architecture Overview:
//
//	Control Plane (Web UI, API, Metadata Storage)
//	        |
//	        | gRPC
//	        v
//	Piped (Agent) - Manages plugins
//	+--------------------------------------------------+
//	|  +----------------+  +----------------+         |
//	|  | Cloud Run     |  | Other Plugins  |         |
//	|  | Plugin (gRPC) |  | (Kubernetes,   |         |
//	|  |               |  |  Terraform...) |         |
//	|  +----------------+  +----------------+         |
//	+--------------------------------------------------+
//
// The plugin runs as a gRPC server and is managed by piped.
// It implements the DeploymentPlugin interface to handle deployment stages.
package main

import (
	"log"

	sdk "github.com/pipe-cd/piped-plugin-sdk-go"

	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/config"
	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/plugin"
)

func main() {
	// Create the Cloud Run plugin instance
	cloudrunPlugin := plugin.NewCloudRunPlugin()

	// Create the plugin using the SDK
	// Parameters:
	//   - "cloudrun": Plugin name (must match piped config)
	//   - WithDeploymentPlugin: Registers this as a deployment plugin
	p, err := sdk.NewPlugin(
		"cloudrun",
		sdk.WithDeploymentPlugin[
			config.PluginConfig,
			config.DeployTargetConfig,
			config.ApplicationConfig,
		](cloudrunPlugin),
	)
	if err != nil {
		log.Fatalf("Failed to create plugin: %v", err)
	}

	// Run the plugin - this starts the gRPC server
	// The plugin will listen on the port specified by piped
	if err := p.Run(); err != nil {
		log.Fatalf("Failed to run plugin: %v", err)
	}
}
