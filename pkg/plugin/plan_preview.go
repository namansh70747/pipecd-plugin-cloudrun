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
	"strings"

	"cloud.google.com/go/run/apiv2/runpb"
	sdk "github.com/pipe-cd/piped-plugin-sdk-go"

	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/cloudrun"
	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/config"
)

// GetPlanPreview returns the plan preview for a Cloud Run deployment.
// This shows what will change when the deployment is executed, enabling:
//   - Pre-deployment visibility of changes
//   - Drift detection between Git and live state
//   - Better decision making before triggering deployments
//
// The plan preview compares:
//   - Service configuration (container image, resources, environment variables)
//   - Traffic allocation
//   - Scaling configuration
func (p *cloudrunPlugin) GetPlanPreview(
	ctx context.Context,
	cfg *config.PluginConfig,
	deployTargets []*sdk.DeployTarget[config.DeployTargetConfig],
	input *sdk.GetPlanPreviewInput[config.ApplicationConfig],
) (*sdk.GetPlanPreviewResponse, error) {
	results := []sdk.PlanPreviewResult{}

	for _, target := range deployTargets {
		result, err := p.generatePlanPreviewForTarget(ctx, target, input)
		if err != nil {
			return nil, fmt.Errorf("failed to generate plan preview for target %s: %w", target.Name, err)
		}
		results = append(results, result)
	}

	return &sdk.GetPlanPreviewResponse{
		Results: results,
	}, nil
}

// generatePlanPreviewForTarget generates plan preview for a single deploy target.
func (p *cloudrunPlugin) generatePlanPreviewForTarget(
	ctx context.Context,
	target *sdk.DeployTarget[config.DeployTargetConfig],
	input *sdk.GetPlanPreviewInput[config.ApplicationConfig],
) (sdk.PlanPreviewResult, error) {
	appConfig := input.Request.TargetDeploymentSource.ApplicationConfig.Spec
	appDir := input.Request.TargetDeploymentSource.ApplicationDirectory

	// Get project ID and region (prefer app config, fallback to deploy target)
	projectID := target.Config.ProjectID
	if appConfig.Input.ProjectID != "" {
		projectID = appConfig.Input.ProjectID
	}

	region := target.Config.Region
	if appConfig.Input.Region != "" {
		region = appConfig.Input.Region
	}

	// Load desired service manifest from Git
	manifestPath := appConfig.ServiceManifestPath
	if manifestPath == "" {
		manifestPath = "service.yaml"
	}

	desiredService, err := cloudrun.LoadServiceManifestFromDir(appDir, manifestPath)
	if err != nil {
		return sdk.PlanPreviewResult{}, fmt.Errorf("failed to load service manifest: %w", err)
	}

	// Apply image override if specified
	if appConfig.Input.Image != "" {
		cloudrun.ApplyImageOverride(desiredService, appConfig.Input.Image)
	}

	serviceName := desiredService.Name
	if appConfig.Input.ServiceName != "" {
		serviceName = appConfig.Input.ServiceName
	}

	// Create Cloud Run client
	client, err := cloudrun.NewClient(ctx, target.Config.CredentialsFile)
	if err != nil {
		return sdk.PlanPreviewResult{}, fmt.Errorf("failed to create Cloud Run client: %w", err)
	}
	defer client.Close()

	// Get current service state from Cloud Run
	currentService, err := client.GetService(ctx, projectID, region, serviceName)
	if err != nil {
		// Service doesn't exist - will be created
		return generateCreateServicePlan(desiredService, projectID, region, target.Name), nil
	}

	// Service exists - compare and generate diff
	return generateUpdateServicePlan(currentService, desiredService, projectID, region, target.Name), nil
}

// generateCreateServicePlan generates a plan for creating a new service.
func generateCreateServicePlan(
	service *runpb.Service,
	projectID, region, targetName string,
) sdk.PlanPreviewResult {
	var details strings.Builder

	details.WriteString(fmt.Sprintf("Target: %s\n", targetName))
	details.WriteString(fmt.Sprintf("Project: %s\n", projectID))
	details.WriteString(fmt.Sprintf("Region: %s\n\n", region))

	details.WriteString("✨ New Cloud Run service will be created\n\n")
	details.WriteString(fmt.Sprintf("Service Name: %s\n", service.Name))

	// Container image
	if service.Template != nil && len(service.Template.Containers) > 0 {
		details.WriteString(fmt.Sprintf("Container Image: %s\n", service.Template.Containers[0].Image))
	}

	// Initial traffic
	if len(service.Traffic) > 0 {
		details.WriteString("\nInitial Traffic:\n")
		for _, t := range service.Traffic {
			if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST {
				details.WriteString(fmt.Sprintf("  - Latest revision: %d%%\n", t.Percent))
			} else if t.Revision != "" {
				details.WriteString(fmt.Sprintf("  - Revision %s: %d%%\n", t.Revision, t.Percent))
			}
		}
	}

	// Scaling configuration
	if service.Template != nil {
		details.WriteString("\nScaling Configuration:\n")
		annotations := service.Template.Annotations
		if minScale, ok := annotations["autoscaling.knative.dev/minScale"]; ok {
			details.WriteString(fmt.Sprintf("  - Min instances: %s\n", minScale))
		}
		if maxScale, ok := annotations["autoscaling.knative.dev/maxScale"]; ok {
			details.WriteString(fmt.Sprintf("  - Max instances: %s\n", maxScale))
		}
	}

	return sdk.PlanPreviewResult{
		DeployTarget: targetName,
		Summary:      fmt.Sprintf("✨ New service '%s' will be created in %s/%s", service.Name, projectID, region),
		NoChange:     false,
		Details:      []byte(details.String()),
	}
}

// generateUpdateServicePlan generates a plan for updating an existing service.
func generateUpdateServicePlan(
	current, desired *runpb.Service,
	projectID, region, targetName string,
) sdk.PlanPreviewResult {
	var details strings.Builder
	changes := []string{}

	details.WriteString(fmt.Sprintf("Target: %s\n", targetName))
	details.WriteString(fmt.Sprintf("Project: %s\n", projectID))
	details.WriteString(fmt.Sprintf("Region: %s\n", region))
	details.WriteString(fmt.Sprintf("Service: %s\n\n", current.Name))

	// Compare container images
	currentImage := ""
	desiredImage := ""
	if current.Template != nil && len(current.Template.Containers) > 0 {
		currentImage = current.Template.Containers[0].Image
	}
	if desired.Template != nil && len(desired.Template.Containers) > 0 {
		desiredImage = desired.Template.Containers[0].Image
	}

	if currentImage != desiredImage {
		changes = append(changes, "container image")
		details.WriteString("📦 Container Image:\n")
		details.WriteString(fmt.Sprintf("  - Current: %s\n", currentImage))
		details.WriteString(fmt.Sprintf("  + Desired: %s\n\n", desiredImage))
	}

	// Compare traffic allocation
	if hasTrafficChanges(current.Traffic, desired.Traffic) {
		changes = append(changes, "traffic allocation")
		details.WriteString("🚦 Traffic Allocation:\n")
		details.WriteString("  Current:\n")
		for _, t := range current.Traffic {
			details.WriteString(formatTrafficTarget(t, "    - "))
		}
		details.WriteString("  Desired:\n")
		for _, t := range desired.Traffic {
			details.WriteString(formatTrafficTarget(t, "    + "))
		}
		details.WriteString("\n")
	}

	// Compare resource limits
	if hasResourceChanges(current.Template, desired.Template) {
		changes = append(changes, "resource limits")
		details.WriteString("💾 Resource Limits:\n")
		if current.Template != nil && len(current.Template.Containers) > 0 {
			currentRes := current.Template.Containers[0].Resources
			if currentRes != nil && currentRes.Limits != nil {
				details.WriteString(fmt.Sprintf("  - CPU: %s, Memory: %s\n",
					currentRes.Limits["cpu"], currentRes.Limits["memory"]))
			}
		}
		if desired.Template != nil && len(desired.Template.Containers) > 0 {
			desiredRes := desired.Template.Containers[0].Resources
			if desiredRes != nil && desiredRes.Limits != nil {
				details.WriteString(fmt.Sprintf("  + CPU: %s, Memory: %s\n\n",
					desiredRes.Limits["cpu"], desiredRes.Limits["memory"]))
			}
		}
	}

	// Compare scaling configuration
	if hasScalingChanges(current.Template, desired.Template) {
		changes = append(changes, "scaling configuration")
		details.WriteString("📈 Scaling Configuration:\n")
		if current.Template != nil {
			currentMin := current.Template.Annotations["autoscaling.knative.dev/minScale"]
			currentMax := current.Template.Annotations["autoscaling.knative.dev/maxScale"]
			if currentMin != "" || currentMax != "" {
				details.WriteString(fmt.Sprintf("  - Min: %s, Max: %s\n", currentMin, currentMax))
			}
		}
		if desired.Template != nil {
			desiredMin := desired.Template.Annotations["autoscaling.knative.dev/minScale"]
			desiredMax := desired.Template.Annotations["autoscaling.knative.dev/maxScale"]
			if desiredMin != "" || desiredMax != "" {
				details.WriteString(fmt.Sprintf("  + Min: %s, Max: %s\n\n", desiredMin, desiredMax))
			}
		}
	}

	// Generate summary
	var summary string
	noChange := len(changes) == 0
	if noChange {
		summary = fmt.Sprintf("✓ No changes - service '%s' matches desired state", current.Name)
		details.WriteString("✓ No changes detected. Service is in sync with Git.\n")
	} else {
		summary = fmt.Sprintf("📝 Service '%s' will be updated (%s)", current.Name, strings.Join(changes, ", "))
		details.WriteString(fmt.Sprintf("🔄 A new revision will be created with %d change(s)\n", len(changes)))
	}

	return sdk.PlanPreviewResult{
		DeployTarget: targetName,
		Summary:      summary,
		NoChange:     noChange,
		Details:      []byte(details.String()),
	}
}

// hasTrafficChanges checks if traffic allocation has changed.
func hasTrafficChanges(current, desired []*runpb.TrafficTarget) bool {
	if len(current) != len(desired) {
		return true
	}

	// Create maps for comparison
	currentMap := make(map[string]int32)
	desiredMap := make(map[string]int32)

	for _, t := range current {
		key := getTrafficKey(t)
		currentMap[key] = t.Percent
	}

	for _, t := range desired {
		key := getTrafficKey(t)
		desiredMap[key] = t.Percent
	}

	// Compare maps
	if len(currentMap) != len(desiredMap) {
		return true
	}

	for key, percent := range currentMap {
		if desiredMap[key] != percent {
			return true
		}
	}

	return false
}

// getTrafficKey returns a unique key for a traffic target.
func getTrafficKey(t *runpb.TrafficTarget) string {
	if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST {
		return "latest"
	}
	return t.Revision
}

// formatTrafficTarget formats a traffic target for display.
func formatTrafficTarget(t *runpb.TrafficTarget, prefix string) string {
	if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST {
		return fmt.Sprintf("%sLatest revision: %d%%\n", prefix, t.Percent)
	}
	return fmt.Sprintf("%sRevision %s: %d%%\n", prefix, t.Revision, t.Percent)
}

// hasResourceChanges checks if resource limits have changed.
func hasResourceChanges(current, desired *runpb.RevisionTemplate) bool {
	if current == nil || desired == nil {
		return current != desired
	}

	if len(current.Containers) == 0 || len(desired.Containers) == 0 {
		return len(current.Containers) != len(desired.Containers)
	}

	currentRes := current.Containers[0].Resources
	desiredRes := desired.Containers[0].Resources

	if currentRes == nil || desiredRes == nil {
		return currentRes != desiredRes
	}

	if currentRes.Limits == nil && desiredRes.Limits == nil {
		return false
	}

	if currentRes.Limits == nil || desiredRes.Limits == nil {
		return true
	}

	return currentRes.Limits["cpu"] != desiredRes.Limits["cpu"] ||
		currentRes.Limits["memory"] != desiredRes.Limits["memory"]
}

// hasScalingChanges checks if scaling configuration has changed.
func hasScalingChanges(current, desired *runpb.RevisionTemplate) bool {
	if current == nil || desired == nil {
		return current != desired
	}

	currentMin := current.Annotations["autoscaling.knative.dev/minScale"]
	currentMax := current.Annotations["autoscaling.knative.dev/maxScale"]
	desiredMin := desired.Annotations["autoscaling.knative.dev/minScale"]
	desiredMax := desired.Annotations["autoscaling.knative.dev/maxScale"]

	return currentMin != desiredMin || currentMax != desiredMax
}
