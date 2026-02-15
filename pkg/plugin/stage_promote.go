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

// ExecutePromoteStage executes the CLOUDRUN_PROMOTE stage.
//
// This stage promotes a revision by adjusting the traffic split.
// It's used for progressive delivery (canary deployments).
//
// Traffic Split Examples:
//   - percent: 0   -> 0% to new, 100% to old (smoke test)
//   - percent: 10  -> 10% to new, 90% to old (canary)
//   - percent: 50  -> 50% to new, 50% to old (A/B test)
//   - percent: 100 -> 100% to new, 0% to old (full promotion)
//
// Pipeline Example (Canary Deployment):
//
//	┌─────────────┐     ┌─────────────┐     ┌─────────────┐
//	│ SYNC        │────▶│ PROMOTE     │────▶│ PROMOTE     │
//	│ (0%)        │     │ (10%)       │     │ (100%)      │
//	└─────────────┘     └─────────────┘     └─────────────┘
//	                           │
//	                           ▼
//	                    ┌─────────────┐
//	                    │ WAIT 5m     │
//	                    └─────────────┘
func (e *StageExecutor) ExecutePromoteStage(
	ctx context.Context,
	cfg *config.PluginConfig,
	deployTargets []*sdk.DeployTarget[config.DeployTargetConfig],
	input *sdk.ExecuteStageInput[config.ApplicationConfig],
	lp sdk.StageLogPersister,
) (*sdk.ExecuteStageResponse, error) {
	// Parse stage configuration
	stageCfg := DefaultPromoteStageConfig()
	if err := parseStageConfig(input.Request.StageConfig, stageCfg); err != nil {
		lp.Errorf("Failed to parse stage config: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	// Validate percentage
	if stageCfg.Percent < 0 || stageCfg.Percent > 100 {
		lp.Errorf("Invalid traffic percentage: %d (must be 0-100)", stageCfg.Percent)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, fmt.Errorf("invalid traffic percentage: %d", stageCfg.Percent)
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
	serviceName := input.Request.ApplicationConfig.Spec.Input.ServiceName
	if serviceName == "" {
		// Try to get from metadata
		serviceName = input.Request.Deployment.DeploymentReference.ApplicationID
	}

	lp.Infof("Promoting service %s to %d%% traffic", serviceName, stageCfg.Percent)

	// Create Cloud Run client
	client, err := cloudrun.NewClient(ctx, dt.Config.CredentialsFile)
	if err != nil {
		lp.Errorf("Failed to create Cloud Run client: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}
	defer client.Close()

	// Create traffic manager
	tm := cloudrun.NewTrafficManager(client)

	// Get current traffic for logging
	currentTraffic, err := tm.GetCurrentTraffic(ctx, project, region, serviceName)
	if err != nil {
		lp.Warnf("Failed to get current traffic: %v", err)
	} else {
		lp.Info("Current traffic allocation:")
		for _, t := range currentTraffic {
			lp.Infof("  - %s: %d%%", t.RevisionName, t.Percent)
		}
	}

	// Perform promotion
	if err := tm.Promote(ctx, project, region, serviceName, int32(stageCfg.Percent)); err != nil {
		lp.Errorf("Failed to promote service: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	// Get new traffic allocation
	newTraffic, err := tm.GetCurrentTraffic(ctx, project, region, serviceName)
	if err != nil {
		lp.Warnf("Failed to get new traffic: %v", err)
	} else {
		lp.Info("New traffic allocation:")
		for _, t := range newTraffic {
			lp.Infof("  - %s: %d%%", t.RevisionName, t.Percent)
		}
	}

	lp.Successf("Successfully promoted service to %d%% traffic", stageCfg.Percent)

	return &sdk.ExecuteStageResponse{
		Status: sdk.StageStatusSuccess,
		Metadata: map[string]string{
			"traffic_percent": fmt.Sprintf("%d", stageCfg.Percent),
			"service_name":    serviceName,
		},
	}, nil
}
