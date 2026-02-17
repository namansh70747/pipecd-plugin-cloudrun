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

// Stage names for Cloud Run deployments.
// These are the stages that the plugin can execute.
const (
	// StageCloudRunSync deploys a new revision to Cloud Run.
	// This stage creates or updates a Cloud Run service.
	StageCloudRunSync = "CLOUDRUN_SYNC"

	// StageCloudRunPromote promotes a revision by adjusting traffic split.
	// This stage is used for progressive delivery (canary deployments).
	StageCloudRunPromote = "CLOUDRUN_PROMOTE"

	// StageCloudRunRollback rolls back to a previous revision.
	// This stage routes all traffic to a specified previous revision.
	StageCloudRunRollback = "CLOUDRUN_ROLLBACK"

	// StageCloudRunCanaryCleanup removes canary revisions that have no traffic.
	// This stage cleans up old revisions after a successful deployment.
	StageCloudRunCanaryCleanup = "CLOUDRUN_CANARY_CLEANUP"
)

// Stage descriptions for UI display.
const (
	StageDescriptionCloudRunSync          = "Deploy a new Cloud Run revision"
	StageDescriptionCloudRunPromote       = "Promote the new revision by adjusting traffic split"
	StageDescriptionCloudRunRollback      = "Rollback to the previous revision"
	StageDescriptionCloudRunCanaryCleanup = "Clean up canary revisions"
)

// SyncStageConfig defines configuration for CLOUDRUN_SYNC stage.
type SyncStageConfig struct {
	// SkipTrafficShift indicates whether to skip traffic shift on initial deploy.
	// If true, the existing traffic configuration is preserved.
	// If false (default), 100% traffic is routed to the new revision.
	SkipTrafficShift bool `json:"skipTrafficShift,omitempty"`

	// Prune indicates whether to remove unused revisions after deployment.
	Prune bool `json:"prune,omitempty"`
}

// PromoteStageConfig defines configuration for CLOUDRUN_PROMOTE stage.
type PromoteStageConfig struct {
	// Percent is the percentage of traffic to route to the new revision (0-100).
	// Example: 10 means 10% to new revision, 90% to previous revision.
	// Example: 100 means 100% to new revision (full promotion).
	Percent int `json:"percent"`
}

// RollbackStageConfig defines configuration for CLOUDRUN_ROLLBACK stage.
type RollbackStageConfig struct {
	// Revision is the revision name to rollback to.
	// If empty, rolls back to the previous revision.
	Revision string `json:"revision,omitempty"`
}

// CanaryCleanupStageConfig defines configuration for CLOUDRUN_CANARY_CLEANUP stage.
type CanaryCleanupStageConfig struct {
	// KeepCount is the number of recent revisions to keep.
	// Default: 5
	KeepCount int `json:"keepCount,omitempty"`

	// KeepLatest indicates whether to always keep the latest revision.
	// Default: true
	KeepLatest bool `json:"keepLatest,omitempty"`
}

// DefaultSyncStageConfig returns default sync stage configuration.
func DefaultSyncStageConfig() *SyncStageConfig {
	return &SyncStageConfig{
		SkipTrafficShift: false,
		Prune:            false,
	}
}

// DefaultPromoteStageConfig returns default promote stage configuration.
func DefaultPromoteStageConfig() *PromoteStageConfig {
	return &PromoteStageConfig{
		Percent: 100,
	}
}

// DefaultRollbackStageConfig returns default rollback stage configuration.
func DefaultRollbackStageConfig() *RollbackStageConfig {
	return &RollbackStageConfig{
		Revision: "",
	}
}

// DefaultCanaryCleanupStageConfig returns default canary cleanup stage configuration.
func DefaultCanaryCleanupStageConfig() *CanaryCleanupStageConfig {
	return &CanaryCleanupStageConfig{
		KeepCount:  5,
		KeepLatest: true,
	}
}

// StageResult represents the result of executing a stage.
type StageResult struct {
	// Status indicates whether the stage succeeded or failed.
	Status StageStatus

	// Message provides additional information about the result.
	Message string

	// Metadata contains stage-specific output data.
	Metadata map[string]string
}

// StageStatus represents the status of a stage execution.
type StageStatus string

const (
	// StageStatusSuccess indicates the stage completed successfully.
	StageStatusSuccess StageStatus = "SUCCESS"

	// StageStatusFailure indicates the stage failed.
	StageStatusFailure StageStatus = "FAILURE"

	// StageStatusCancelled indicates the stage was cancelled.
	StageStatusCancelled StageStatus = "CANCELLED"
)
