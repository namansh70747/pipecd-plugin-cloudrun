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

// Package plugin implements the PipeCD Cloud Run Plugin.
//
// This plugin enables PipeCD to deploy applications to Google Cloud Run
// with support for progressive delivery strategies like canary deployments
// and traffic splitting.
//
// The plugin implements the DeploymentPlugin interface from the PipeCD SDK,
// which allows it to be managed by piped and communicate with the control plane.
//
// Deployment Flow:
//
//	1. DetermineVersions - Extract version info from the deployment
//	2. DetermineStrategy - Decide between quick sync and pipeline sync
//	3. BuildQuickSyncStages / BuildPipelineSyncStages - Define deployment stages
//	4. ExecuteStage - Execute each stage (SYNC, PROMOTE, ROLLBACK, CLEANUP)
//
// Example Pipeline (Canary Deployment):
//
//	┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
//	│ CLOUDRUN    │───▶│ CLOUDRUN    │───▶│ CLOUDRUN    │───▶│ CLOUDRUN    │
//	│ SYNC        │    │ PROMOTE     │    │ PROMOTE     │    │ PROMOTE     │
//	│ (0% traffic)│    │ (10% traffic│    │ (50% traffic│    │ (100% traffic│
//	└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
//	                                                         │
//	                                                         ▼
//	                                              ┌─────────────┐
//	                                              │ CANARY      │
//	                                              │ CLEANUP     │
//	                                              └─────────────┘
package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/pipe-cd/piped-plugin-sdk-go"

	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/config"
)

// Ensure cloudrunPlugin implements the DeploymentPlugin interface.
var _ sdk.DeploymentPlugin[config.PluginConfig, config.DeployTargetConfig, config.ApplicationConfig] = (*cloudrunPlugin)(nil)

// cloudrunPlugin implements the PipeCD DeploymentPlugin interface for Cloud Run.
type cloudrunPlugin struct {
	// stageExecutor handles the execution of individual stages
	stageExecutor *StageExecutor
}

// NewCloudRunPlugin creates a new Cloud Run plugin instance.
func NewCloudRunPlugin() *cloudrunPlugin {
	return &cloudrunPlugin{
		stageExecutor: NewStageExecutor(),
	}
}

// FetchDefinedStages returns the list of stages this plugin can execute.
// This is called by piped to discover what stages the plugin supports.
//
// Returns: ["CLOUDRUN_SYNC", "CLOUDRUN_PROMOTE", "CLOUDRUN_ROLLBACK", "CLOUDRUN_CANARY_CLEANUP"]
func (p *cloudrunPlugin) FetchDefinedStages() []string {
	return []string{
		StageCloudRunSync,
		StageCloudRunPromote,
		StageCloudRunRollback,
		StageCloudRunCanaryCleanup,
	}
}

// DetermineVersions determines the versions of the application being deployed.
// This information is displayed in the PipeCD UI.
//
// For Cloud Run, the version is typically derived from the container image tag.
// Example: "gcr.io/project/app:v1.0.0" -> version "v1.0.0"
func (p *cloudrunPlugin) DetermineVersions(
	ctx context.Context,
	cfg *config.PluginConfig,
	input *sdk.DetermineVersionsInput[config.ApplicationConfig],
) (*sdk.DetermineVersionsResponse, error) {
	// Extract version from container image
	image := input.DeploymentSource.ApplicationConfig.Spec.Input.Image
	version := extractVersionFromImage(image)

	return &sdk.DetermineVersionsResponse{
		Versions: []*sdk.Version{
			{
				Kind:    "ContainerImage",
				Version: version,
				Name:    image,
			},
		},
	}, nil
}

// DetermineStrategy determines the deployment strategy to use.
//
// If the application configuration specifies a pipeline, it uses PipelineSync.
// Otherwise, it defaults to QuickSync (immediate 100% traffic shift).
func (p *cloudrunPlugin) DetermineStrategy(
	ctx context.Context,
	cfg *config.PluginConfig,
	input *sdk.DetermineStrategyInput[config.ApplicationConfig],
) (*sdk.DetermineStrategyResponse, error) {
	// Check if pipeline is defined in app config
	if input.ApplicationConfig.Spec.PipelineSync != nil {
		return &sdk.DetermineStrategyResponse{
			Strategy: sdk.SyncStrategyPipelineSync,
		}, nil
	}

	// Default to quick sync
	return &sdk.DetermineStrategyResponse{
		Strategy: sdk.SyncStrategyQuickSync,
	}, nil
}

// BuildQuickSyncStages builds stages for quick sync deployment.
//
// Quick sync deploys the new revision and immediately routes 100% traffic to it.
// This is the simplest deployment strategy.
//
// Pipeline: [CLOUDRUN_SYNC]
func (p *cloudrunPlugin) BuildQuickSyncStages(
	ctx context.Context,
	cfg *config.PluginConfig,
	input *sdk.BuildQuickSyncStagesInput,
) (*sdk.BuildQuickSyncStagesResponse, error) {
	stages := []sdk.PipelineStage{
		{
			Index:              0,
			Name:               StageCloudRunSync,
			Rollback:           false,
			Metadata:           map[string]string{},
			AvailableOperation: sdk.ManualOperationNone,
			Description:        StageDescriptionCloudRunSync,
		},
	}

	return &sdk.BuildQuickSyncStagesResponse{
		Stages: stages,
	}, nil
}

// BuildPipelineSyncStages builds stages for pipeline sync deployment.
//
// Pipeline sync allows for custom deployment pipelines with progressive delivery.
// The stages are defined in the application's .pipe.yaml file.
//
// Example pipeline:
//   - CLOUDRUN_SYNC (deploy with 0% traffic)
//   - CLOUDRUN_PROMOTE (10% traffic)
//   - WAIT (5 minutes)
//   - CLOUDRUN_PROMOTE (100% traffic)
//   - CLOUDRUN_CANARY_CLEANUP
func (p *cloudrunPlugin) BuildPipelineSyncStages(
	ctx context.Context,
	cfg *config.PluginConfig,
	input *sdk.BuildPipelineSyncStagesInput,
) (*sdk.BuildPipelineSyncStagesResponse, error) {
	stages := make([]sdk.PipelineStage, 0, len(input.Request.Stages))

	for _, rs := range input.Request.Stages {
		stage := sdk.PipelineStage{
			Index:              rs.Index,
			Name:               rs.Name,
			Rollback:           rs.Rollback,
			Metadata:           map[string]string{},
			AvailableOperation: sdk.ManualOperationNone,
			Description:        getStageDescription(rs.Name),
		}
		stages = append(stages, stage)
	}

	return &sdk.BuildPipelineSyncStagesResponse{
		Stages: stages,
	}, nil
}

// ExecuteStage executes the given deployment stage.
//
// This is the main entry point for stage execution. It dispatches to the
// appropriate stage handler based on the stage name.
//
// Supported stages:
//   - CLOUDRUN_SYNC: Deploy a new revision
//   - CLOUDRUN_PROMOTE: Adjust traffic split
//   - CLOUDRUN_ROLLBACK: Rollback to previous revision
//   - CLOUDRUN_CANARY_CLEANUP: Clean up old revisions
func (p *cloudrunPlugin) ExecuteStage(
	ctx context.Context,
	cfg *config.PluginConfig,
	deployTargets []*sdk.DeployTarget[config.DeployTargetConfig],
	input *sdk.ExecuteStageInput[config.ApplicationConfig],
) (*sdk.ExecuteStageResponse, error) {
	// Get log persister for logging stage execution
	lp := input.Client.LogPersister()

	lp.Infof("Executing stage: %s", input.Request.StageName)

	// Dispatch to appropriate stage handler
	switch input.Request.StageName {
	case StageCloudRunSync:
		return p.stageExecutor.ExecuteSyncStage(ctx, cfg, deployTargets, input, lp)
	case StageCloudRunPromote:
		return p.stageExecutor.ExecutePromoteStage(ctx, cfg, deployTargets, input, lp)
	case StageCloudRunRollback:
		return p.stageExecutor.ExecuteRollbackStage(ctx, cfg, deployTargets, input, lp)
	case StageCloudRunCanaryCleanup:
		return p.stageExecutor.ExecuteCanaryCleanupStage(ctx, cfg, deployTargets, input, lp)
	default:
		lp.Errorf("Unsupported stage: %s", input.Request.StageName)
		return nil, fmt.Errorf("unsupported stage: %s", input.Request.StageName)
	}
}

// extractVersionFromImage extracts the version from a container image URL.
// Example: "gcr.io/project/app:v1.0.0" -> "v1.0.0"
func extractVersionFromImage(image string) string {
	if image == "" {
		return "unknown"
	}

	// Find the last colon which typically separates image name from tag
	for i := len(image) - 1; i >= 0; i-- {
		if image[i] == ':' {
			return image[i+1:]
		}
	}

	return image
}

// getStageDescription returns the description for a stage.
func getStageDescription(stageName string) string {
	switch stageName {
	case StageCloudRunSync:
		return StageDescriptionCloudRunSync
	case StageCloudRunPromote:
		return StageDescriptionCloudRunPromote
	case StageCloudRunRollback:
		return StageDescriptionCloudRunRollback
	case StageCloudRunCanaryCleanup:
		return StageDescriptionCloudRunCanaryCleanup
	default:
		return "Unknown stage"
	}
}

// StageExecutor handles the execution of individual deployment stages.
type StageExecutor struct {
	// Add any shared dependencies here
}

// NewStageExecutor creates a new StageExecutor.
func NewStageExecutor() *StageExecutor {
	return &StageExecutor{}
}

// parseStageConfig parses stage configuration from JSON.
func parseStageConfig(data []byte, v interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}
