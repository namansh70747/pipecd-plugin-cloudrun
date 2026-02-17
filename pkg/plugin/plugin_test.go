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
	"strings"
	"testing"

	"cloud.google.com/go/run/apiv2/runpb"
	sdk "github.com/pipe-cd/piped-plugin-sdk-go"

	"github.com/pipe-cd/pipecd-plugin-cloudrun/pkg/config"
)

func TestCloudRunPlugin_FetchDefinedStages(t *testing.T) {
	p := NewCloudRunPlugin()
	stages := p.FetchDefinedStages()

	expected := []string{
		StageCloudRunSync,
		StageCloudRunPromote,
		StageCloudRunRollback,
		StageCloudRunCanaryCleanup,
	}

	if len(stages) != len(expected) {
		t.Errorf("expected %d stages, got %d", len(expected), len(stages))
	}

	for i, stage := range stages {
		if stage != expected[i] {
			t.Errorf("expected stage %d to be %s, got %s", i, expected[i], stage)
		}
	}
}

func TestCloudRunPlugin_DetermineStrategy_QuickSync(t *testing.T) {
	p := NewCloudRunPlugin()

	input := &sdk.DetermineStrategyInput[config.ApplicationConfig]{
		Request: sdk.DetermineStrategyRequest[config.ApplicationConfig]{
			TargetDeploymentSource: sdk.DeploymentSource[config.ApplicationConfig]{
				ApplicationConfig: &sdk.ApplicationConfig[config.ApplicationConfig]{
					Spec: &config.ApplicationConfig{
						// No pipeline sync configured
						PipelineSync: nil,
					},
				},
			},
		},
	}

	resp, err := p.DetermineStrategy(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Strategy != sdk.SyncStrategyQuickSync {
		t.Errorf("expected quick sync, got %v", resp.Strategy)
	}
}

func TestCloudRunPlugin_DetermineStrategy_PipelineSync(t *testing.T) {
	p := NewCloudRunPlugin()

	input := &sdk.DetermineStrategyInput[config.ApplicationConfig]{
		Request: sdk.DetermineStrategyRequest[config.ApplicationConfig]{
			TargetDeploymentSource: sdk.DeploymentSource[config.ApplicationConfig]{
				ApplicationConfig: &sdk.ApplicationConfig[config.ApplicationConfig]{
					Spec: &config.ApplicationConfig{
						PipelineSync: &config.PipelineSyncConfig{
							Stages: []config.PipelineStage{
								{Name: StageCloudRunSync},
							},
						},
					},
				},
			},
		},
	}

	resp, err := p.DetermineStrategy(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Strategy != sdk.SyncStrategyPipelineSync {
		t.Errorf("expected pipeline sync, got %v", resp.Strategy)
	}
}

func TestCloudRunPlugin_BuildQuickSyncStages(t *testing.T) {
	p := NewCloudRunPlugin()

	input := &sdk.BuildQuickSyncStagesInput{}

	resp, err := p.BuildQuickSyncStages(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Stages) != 1 {
		t.Errorf("expected 1 stage, got %d", len(resp.Stages))
	}

	if resp.Stages[0].Name != StageCloudRunSync {
		t.Errorf("expected stage %s, got %s", StageCloudRunSync, resp.Stages[0].Name)
	}
}

func TestCloudRunPlugin_BuildPipelineSyncStages(t *testing.T) {
	p := NewCloudRunPlugin()

	input := &sdk.BuildPipelineSyncStagesInput{
		Request: sdk.BuildPipelineSyncStagesRequest{
			Stages: []sdk.StageConfig{
				{Index: 0, Name: StageCloudRunSync},
				{Index: 1, Name: StageCloudRunPromote},
				{Index: 2, Name: StageCloudRunCanaryCleanup},
			},
		},
	}

	resp, err := p.BuildPipelineSyncStages(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Stages) != 3 {
		t.Errorf("expected 3 stages, got %d", len(resp.Stages))
	}

	expectedStages := []string{StageCloudRunSync, StageCloudRunPromote, StageCloudRunCanaryCleanup}
	for i, stage := range resp.Stages {
		if stage.Name != expectedStages[i] {
			t.Errorf("expected stage %d to be %s, got %s", i, expectedStages[i], stage.Name)
		}
	}
}

func TestExtractVersionFromImage(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{
			image:    "gcr.io/project/app:v1.0.0",
			expected: "v1.0.0",
		},
		{
			image:    "nginx:latest",
			expected: "latest",
		},
		{
			image:    "my-registry.com/team/service:abc123",
			expected: "abc123",
		},
		{
			image:    "no-tag-image",
			expected: "no-tag-image",
		},
		{
			image:    "",
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			result := extractVersionFromImage(tt.image)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetStageDescription(t *testing.T) {
	tests := []struct {
		stageName string
		expected  string
	}{
		{StageCloudRunSync, StageDescriptionCloudRunSync},
		{StageCloudRunPromote, StageDescriptionCloudRunPromote},
		{StageCloudRunRollback, StageDescriptionCloudRunRollback},
		{StageCloudRunCanaryCleanup, StageDescriptionCloudRunCanaryCleanup},
		{"UNKNOWN_STAGE", "Unknown stage"},
	}

	for _, tt := range tests {
		t.Run(tt.stageName, func(t *testing.T) {
			result := getStageDescription(tt.stageName)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestDefaultStageConfigs(t *testing.T) {
	t.Run("SyncStageConfig", func(t *testing.T) {
		cfg := DefaultSyncStageConfig()
		if cfg.SkipTrafficShift != false {
			t.Errorf("expected SkipTrafficShift to be false")
		}
		if cfg.Prune != false {
			t.Errorf("expected Prune to be false")
		}
	})

	t.Run("PromoteStageConfig", func(t *testing.T) {
		cfg := DefaultPromoteStageConfig()
		if cfg.Percent != 100 {
			t.Errorf("expected Percent to be 100, got %d", cfg.Percent)
		}
	})

	t.Run("RollbackStageConfig", func(t *testing.T) {
		cfg := DefaultRollbackStageConfig()
		if cfg.Revision != "" {
			t.Errorf("expected Revision to be empty")
		}
	})

	t.Run("CanaryCleanupStageConfig", func(t *testing.T) {
		cfg := DefaultCanaryCleanupStageConfig()
		if cfg.KeepCount != 5 {
			t.Errorf("expected KeepCount to be 5, got %d", cfg.KeepCount)
		}
		if cfg.KeepLatest != true {
			t.Errorf("expected KeepLatest to be true")
		}
	})
}

func TestPlanPreview_CreateService(t *testing.T) {
	// Test plan preview for creating a new service
	result := generateCreateServicePlan(
		&runpb.Service{
			Name: "test-service",
			Template: &runpb.RevisionTemplate{
				Containers: []*runpb.Container{
					{Image: "gcr.io/project/app:v1.0.0"},
				},
				Annotations: map[string]string{
					"autoscaling.knative.dev/minScale": "0",
					"autoscaling.knative.dev/maxScale": "10",
				},
			},
			Traffic: []*runpb.TrafficTarget{
				{
					Type:    runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST,
					Percent: 100,
				},
			},
		},
		"test-project",
		"us-central1",
		"staging",
	)

	if !strings.Contains(result.Summary, "New service") {
		t.Errorf("expected summary to mention new service, got: %s", result.Summary)
	}

	details := string(result.Details)
	if !strings.Contains(details, "test-service") {
		t.Errorf("expected details to contain service name, got: %s", details)
	}

	if !strings.Contains(details, "gcr.io/project/app:v1.0.0") {
		t.Errorf("expected details to contain image, got: %s", details)
	}
}

func TestPlanPreview_UpdateService_NoChanges(t *testing.T) {
	// Test plan preview when service hasn't changed
	service := &runpb.Service{
		Name: "test-service",
		Template: &runpb.RevisionTemplate{
			Containers: []*runpb.Container{
				{
					Image: "gcr.io/project/app:v1.0.0",
					Resources: &runpb.ResourceRequirements{
						Limits: map[string]string{
							"cpu":    "1000m",
							"memory": "512Mi",
						},
					},
				},
			},
			Annotations: map[string]string{
				"autoscaling.knative.dev/minScale": "0",
				"autoscaling.knative.dev/maxScale": "10",
			},
		},
		Traffic: []*runpb.TrafficTarget{
			{
				Type:    runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST,
				Percent: 100,
			},
		},
	}

	result := generateUpdateServicePlan(service, service, "test-project", "us-central1", "production")

	if !strings.Contains(result.Summary, "No changes") {
		t.Errorf("expected summary to indicate no changes, got: %s", result.Summary)
	}

	details := string(result.Details)
	if !strings.Contains(details, "No changes detected") {
		t.Errorf("expected details to indicate no changes, got: %s", details)
	}
}

func TestPlanPreview_UpdateService_ImageChange(t *testing.T) {
	// Test plan preview when only image changes
	current := &runpb.Service{
		Name: "test-service",
		Template: &runpb.RevisionTemplate{
			Containers: []*runpb.Container{
				{Image: "gcr.io/project/app:v1.0.0"},
			},
		},
	}

	desired := &runpb.Service{
		Name: "test-service",
		Template: &runpb.RevisionTemplate{
			Containers: []*runpb.Container{
				{Image: "gcr.io/project/app:v2.0.0"},
			},
		},
	}

	result := generateUpdateServicePlan(current, desired, "test-project", "us-central1", "production")

	if !strings.Contains(result.Summary, "container image") {
		t.Errorf("expected summary to mention container image change, got: %s", result.Summary)
	}

	details := string(result.Details)
	if !strings.Contains(details, "v1.0.0") {
		t.Errorf("expected details to contain current image, got: %s", details)
	}

	if !strings.Contains(details, "v2.0.0") {
		t.Errorf("expected details to contain desired image, got: %s", details)
	}
}

func TestPlanPreview_TrafficChanges(t *testing.T) {
	tests := []struct {
		name     string
		current  []*runpb.TrafficTarget
		desired  []*runpb.TrafficTarget
		expected bool
	}{
		{
			name: "Same traffic",
			current: []*runpb.TrafficTarget{
				{Type: runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST, Percent: 100},
			},
			desired: []*runpb.TrafficTarget{
				{Type: runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST, Percent: 100},
			},
			expected: false,
		},
		{
			name: "Different percentages",
			current: []*runpb.TrafficTarget{
				{Type: runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST, Percent: 100},
			},
			desired: []*runpb.TrafficTarget{
				{Type: runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST, Percent: 50},
			},
			expected: true,
		},
		{
			name: "Different number of targets",
			current: []*runpb.TrafficTarget{
				{Type: runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST, Percent: 100},
			},
			desired: []*runpb.TrafficTarget{
				{Type: runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST, Percent: 50},
				{Type: runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION, Revision: "rev-001", Percent: 50},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasTrafficChanges(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPlanPreview_ResourceChanges(t *testing.T) {
	tests := []struct {
		name     string
		current  *runpb.RevisionTemplate
		desired  *runpb.RevisionTemplate
		expected bool
	}{
		{
			name: "Same resources",
			current: &runpb.RevisionTemplate{
				Containers: []*runpb.Container{
					{
						Resources: &runpb.ResourceRequirements{
							Limits: map[string]string{"cpu": "1000m", "memory": "512Mi"},
						},
					},
				},
			},
			desired: &runpb.RevisionTemplate{
				Containers: []*runpb.Container{
					{
						Resources: &runpb.ResourceRequirements{
							Limits: map[string]string{"cpu": "1000m", "memory": "512Mi"},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Different CPU",
			current: &runpb.RevisionTemplate{
				Containers: []*runpb.Container{
					{
						Resources: &runpb.ResourceRequirements{
							Limits: map[string]string{"cpu": "1000m", "memory": "512Mi"},
						},
					},
				},
			},
			desired: &runpb.RevisionTemplate{
				Containers: []*runpb.Container{
					{
						Resources: &runpb.ResourceRequirements{
							Limits: map[string]string{"cpu": "2000m", "memory": "512Mi"},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasResourceChanges(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPlanPreview_ScalingChanges(t *testing.T) {
	tests := []struct {
		name     string
		current  *runpb.RevisionTemplate
		desired  *runpb.RevisionTemplate
		expected bool
	}{
		{
			name: "Same scaling",
			current: &runpb.RevisionTemplate{
				Annotations: map[string]string{
					"autoscaling.knative.dev/minScale": "0",
					"autoscaling.knative.dev/maxScale": "10",
				},
			},
			desired: &runpb.RevisionTemplate{
				Annotations: map[string]string{
					"autoscaling.knative.dev/minScale": "0",
					"autoscaling.knative.dev/maxScale": "10",
				},
			},
			expected: false,
		},
		{
			name: "Different max scale",
			current: &runpb.RevisionTemplate{
				Annotations: map[string]string{
					"autoscaling.knative.dev/minScale": "0",
					"autoscaling.knative.dev/maxScale": "10",
				},
			},
			desired: &runpb.RevisionTemplate{
				Annotations: map[string]string{
					"autoscaling.knative.dev/minScale": "0",
					"autoscaling.knative.dev/maxScale": "20",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasScalingChanges(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
