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

// ExecuteRollbackStage executes the CLOUDRUN_ROLLBACK stage.
//
// This stage rolls back to a previous revision by routing 100% traffic to it.
// It can either roll back to a specific revision or to the previous revision.
//
// Rollback Scenarios:
//
//  1. Rollback to previous revision (default):
//     - Finds the second most recent revision
//     - Routes 100% traffic to it
//
//  2. Rollback to specific revision:
//     - Uses the revision specified in config
//     - Routes 100% traffic to it
//
// Example Pipeline with Rollback:
//
//	┌─────────────┐     ┌─────────────┐     ┌─────────────┐
//	│ SYNC        │────▶│ PROMOTE     │────▶│ ROLLBACK    │
//	│ (0%)        │     │ (10%)       │     │ (if failed) │
//	└─────────────┘     └─────────────┘     └─────────────┘
//	                                               │
//	                                               ▼
//	                                        ┌─────────────┐
//	                                        │ 100% to     │
//	                                        │ previous    │
//	                                        └─────────────┘
func (e *StageExecutor) ExecuteRollbackStage(
	ctx context.Context,
	cfg *config.PluginConfig,
	deployTargets []*sdk.DeployTarget[config.DeployTargetConfig],
	input *sdk.ExecuteStageInput[config.ApplicationConfig],
	lp sdk.StageLogPersister,
) (*sdk.ExecuteStageResponse, error) {
	// Parse stage configuration
	stageCfg := DefaultRollbackStageConfig()
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

	lp.Infof("Rolling back service: %s", serviceName)

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
	tm := cloudrun.NewTrafficManager(client)

	var targetRevision string

	if stageCfg.Revision != "" {
		// Rollback to specific revision
		targetRevision = stageCfg.Revision
		lp.Infof("Rolling back to specified revision: %s", targetRevision)
	} else {
		// Rollback to previous revision
		lp.Info("Finding previous revision...")
		prevRev, err := rm.GetPreviousRevision(ctx, project, region, serviceName)
		if err != nil {
			lp.Errorf("Failed to find previous revision: %v", err)
			return &sdk.ExecuteStageResponse{
				Status: sdk.StageStatusFailure,
			}, err
		}
		targetRevision = prevRev.Name
		lp.Infof("Rolling back to previous revision: %s", targetRevision)
	}

	// Get revision info for logging
	revInfo, err := rm.GetRevision(ctx, project, region, serviceName, targetRevision)
	if err != nil {
		lp.Infof("Warning: Failed to get revision info: %v", err)
	} else {
		lp.Infof("Target revision image: %s", revInfo.Image)
	}

	// Perform rollback
	if err := tm.Rollback(ctx, project, region, serviceName, targetRevision); err != nil {
		lp.Errorf("Failed to rollback service: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	lp.Successf("Successfully rolled back to revision: %s", targetRevision)

	return &sdk.ExecuteStageResponse{
		Status: sdk.StageStatusSuccess,
	}, nil
}
