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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"cloud.google.com/go/run/apiv2/runpb"
	sdk "github.com/pipe-cd/piped-plugin-sdk-go"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/cloudrun"
	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/config"
)

// ExecuteSyncStage executes the CLOUDRUN_SYNC stage.
//
// This stage:
//   1. Reads the service manifest from the application directory
//   2. Applies any image overrides from the app config
//   3. Creates or updates the Cloud Run service
//   4. Optionally routes traffic to the new revision
//
// For Quick Sync: Routes 100% traffic immediately
// For Pipeline Sync: May skip traffic shift (controlled by skipTrafficShift option)
func (e *StageExecutor) ExecuteSyncStage(
	ctx context.Context,
	cfg *config.PluginConfig,
	deployTargets []*sdk.DeployTarget[config.DeployTargetConfig],
	input *sdk.ExecuteStageInput[config.ApplicationConfig],
	lp sdk.StageLogPersister,
) (*sdk.ExecuteStageResponse, error) {
	// Parse stage configuration
	stageCfg := DefaultSyncStageConfig()
	if err := parseStageConfig(input.Request.StageConfig, stageCfg); err != nil {
		lp.Errorf("Failed to parse stage config: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	// Get deploy target config (use first deploy target)
	if len(deployTargets) == 0 {
		lp.Errorf("No deploy targets configured")
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, fmt.Errorf("no deploy targets configured")
	}
	dt := deployTargets[0]

	// Resolve project and region (deploy target overrides plugin config)
	project := dt.Config.ProjectID
	if project == "" {
		project = cfg.ProjectID
	}
	region := dt.Config.Region
	if region == "" {
		region = cfg.Region
	}

	lp.Infof("Deploying to project: %s, region: %s", project, region)

	// Create Cloud Run client
	client, err := cloudrun.NewClient(ctx, dt.Config.CredentialsFile)
	if err != nil {
		lp.Errorf("Failed to create Cloud Run client: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}
	defer client.Close()

	// Read service manifest
	appDir := input.Request.RunningDeploymentSource.ApplicationDirectory
	manifestPath := input.Request.ApplicationConfig.Spec.ServiceManifestPath
	if manifestPath == "" {
		manifestPath = "service.yaml" // Default manifest path
	}

	fullManifestPath := filepath.Join(appDir, manifestPath)
	lp.Infof("Reading service manifest from: %s", fullManifestPath)

	manifestData, err := os.ReadFile(fullManifestPath)
	if err != nil {
		lp.Errorf("Failed to read service manifest: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	// Parse service manifest (JSON format)
	var service runpb.Service
	if err := protojson.Unmarshal(manifestData, &service); err != nil {
		lp.Errorf("Failed to parse service manifest: %v", err)
		lp.Info("Attempting to parse as YAML...")
		// TODO: Add YAML parsing support
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	// Extract service name from manifest or use configured name
	serviceName := input.Request.ApplicationConfig.Spec.Input.ServiceName
	if serviceName == "" && service.Template != nil && service.Template.Labels != nil {
		serviceName = service.Template.Labels["app"]
	}
	if serviceName == "" {
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, fmt.Errorf("service name not specified in manifest or config")
	}

	// Set full resource name
	cloudrun.SetServiceName(&service, project, region, serviceName)

	// Override image if specified in app config
	image := input.Request.ApplicationConfig.Spec.Input.Image
	if image != "" {
		lp.Infof("Overriding container image: %s", image)
		cloudrun.ApplyImageOverride(&service, image)
	}

	lp.Infof("Deploying service: %s", service.Name)

	// Check if service exists
	existingSvc, err := client.GetService(ctx, project, region, serviceName)
	if err != nil {
		lp.Infof("Service does not exist, creating new service")
	}

	// Preserve or set traffic configuration
	if existingSvc != nil {
		if stageCfg.SkipTrafficShift {
			// Preserve existing traffic configuration
			lp.Info("Preserving existing traffic configuration")
			service.Traffic = existingSvc.Traffic
		} else {
			// Route 100% traffic to new revision (quick sync behavior)
			lp.Info("Routing 100% traffic to new revision")
			service.Traffic = []*runpb.TrafficTarget{
				{
					Type:    &runpb.TrafficTarget_LatestRevision{LatestRevision: true},
					Percent: 100,
				},
			}
		}
	} else {
		// New service - route 100% to latest
		service.Traffic = []*runpb.TrafficTarget{
			{
				Type:    &runpb.TrafficTarget_LatestRevision{LatestRevision: true},
				Percent: 100,
			},
		}
	}

	// Deploy the service
	result, err := client.CreateOrUpdateService(ctx, &service)
	if err != nil {
		lp.Errorf("Failed to deploy service: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	// Wait for service to be ready
	lp.Info("Waiting for service to be ready...")
	if err := client.WaitForServiceReady(ctx, project, region, serviceName); err != nil {
		lp.Errorf("Service failed to become ready: %v", err)
		return &sdk.ExecuteStageResponse{
			Status: sdk.StageStatusFailure,
		}, err
	}

	lp.Successf("Successfully deployed revision: %s", result.Template.Revision)
	lp.Infof("Service URL: %s", result.Uri)

	// Prune old revisions if requested
	if stageCfg.Prune {
		lp.Info("Pruning old revisions...")
		rm := cloudrun.NewRevisionManager(client)
		if err := rm.CleanupOldRevisions(ctx, project, region, serviceName, 5, true); err != nil {
			lp.Warnf("Failed to prune old revisions: %v", err)
			// Don't fail the stage for pruning errors
		}
	}

	return &sdk.ExecuteStageResponse{
		Status: sdk.StageStatusSuccess,
		Metadata: map[string]string{
			"revision":     result.Template.Revision,
			"service_url":  result.Uri,
			"service_name": serviceName,
		},
	}, nil
}
