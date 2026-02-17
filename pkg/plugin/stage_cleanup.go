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

package plugin

import (
	"context"
	"fmt"

	sdk "github.com/pipe-cd/piped-plugin-sdk-go"

	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/cloudrun"
	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/config"
)

// ExecuteCanaryCleanupStage executes the CLOUDRUN_CANARY_CLEANUP stage.
//
// This stage cleans up old revisions that are no longer receiving traffic.
// It's typically the final stage in a canary deployment pipeline.
//
// Cleanup Strategy:
//   - Keep the specified number of recent revisions (default: 5)
//   - Always keep the latest revision (configurable)
//   - Only delete revisions with 0% traffic
//
// Example Pipeline:
//
//	┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
//	│ SYNC        │────▶│ PROMOTE     │────▶│ PROMOTE     │────▶│ CLEANUP     │
//	│ (0%)        │     │ (10%)       │     │ (100%)      │     │ (old revs)  │
//	└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
func (e *StageExecutor) ExecuteCanaryCleanupStage(
	ctx context.Context,
	cfg *config.PluginConfig,
	deployTargets []*sdk.DeployTarget[config.DeployTargetConfig],
	input *sdk.ExecuteStageInput[config.ApplicationConfig],
	lp sdk.StageLogPersister,
) (*sdk.ExecuteStageResponse, error) {
	// Parse stage configuration
	stageCfg := DefaultCanaryCleanupStageConfig()
	if err := parseStageConfig(input.Request.StageConfig, stageCfg); err != nil {
		lp.Errorf("Failed to parse stage config: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	// Get deploy target config
	if len(deployTargets) == 0 {
		lp.Errorf("No deploy targets configured")
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, fmt.Errorf("no deploy targets configured")
	}
	dt := deployTargets[0]

	// Resolve project and region
	project := dt.Config.ProjectID
	if project == "" {
		project = cfg.ProjectID
	}
	region := dt.Config.Region
	if region == "" {
		region = cfg.Region
	}

	// Get service name
	serviceName := input.Request.TargetDeploymentSource.ApplicationConfig.Spec.Input.ServiceName
	if serviceName == "" {
		serviceName = input.Request.Deployment.ApplicationID
	}

	lp.Infof("Cleaning up revisions for service: %s", serviceName)
	lp.Infof("Keep count: %d, Keep latest: %v", stageCfg.KeepCount, stageCfg.KeepLatest)

	// Create Cloud Run client
	client, err := cloudrun.NewClient(ctx, dt.Config.CredentialsFile)
	if err != nil {
		lp.Errorf("Failed to create Cloud Run client: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}
	defer client.Close()

	// Create revision manager
	rm := cloudrun.NewRevisionManager(client)

	// List all revisions before cleanup
	revisions, err := rm.ListRevisions(ctx, project, region, serviceName)
	if err != nil {
		lp.Errorf("Failed to list revisions: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	lp.Infof("Found %d revisions", len(revisions))
	for _, rev := range revisions {
		lp.Infof("  - %s: %d%% traffic, created at %s", rev.Name, rev.TrafficPercent, rev.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	// Perform cleanup
	if err := rm.CleanupOldRevisions(ctx, project, region, serviceName, stageCfg.KeepCount, stageCfg.KeepLatest); err != nil {
		lp.Errorf("Failed to cleanup revisions: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	// List revisions after cleanup
	revisionsAfter, err := rm.ListRevisions(ctx, project, region, serviceName)
	if err != nil {
		lp.Infof("Warning: Failed to list revisions after cleanup: %v", err)
	} else {
		deletedCount := len(revisions) - len(revisionsAfter)
		lp.Infof("Cleanup complete. Deleted %d revisions, %d remaining", deletedCount, len(revisionsAfter))
	}

	lp.Successf("Successfully cleaned up old revisions")

	return &sdk.ExecuteStageResponse{
		Status: sdk.StageStatusSuccess,
	}, nil
}
