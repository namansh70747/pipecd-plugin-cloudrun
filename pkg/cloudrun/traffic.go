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

package cloudrun

import (
	"context"
	"fmt"
	"sort"

	"cloud.google.com/go/run/apiv2/runpb"
)

// TrafficManager provides operations for managing traffic splitting.
type TrafficManager struct {
	client Client
}

// NewTrafficManager creates a new TrafficManager.
func NewTrafficManager(client Client) *TrafficManager {
	return &TrafficManager{client: client}
}

// TrafficSplit defines a traffic split configuration.
type TrafficSplit struct {
	// RevisionName is the name of the revision to route traffic to.
	// If empty and IsLatest is true, traffic goes to the latest revision.
	RevisionName string

	// IsLatest indicates if traffic should go to the latest revision.
	IsLatest bool

	// Percent is the percentage of traffic (0-100).
	Percent int32

	// Tag is an optional tag for the revision (creates a unique URL).
	Tag string
}

// BuildTrafficTargets builds TrafficTarget protobufs from TrafficSplit configs.
func BuildTrafficTargets(splits []TrafficSplit) []*runpb.TrafficTarget {
	targets := make([]*runpb.TrafficTarget, 0, len(splits))

	for _, split := range splits {
		target := &runpb.TrafficTarget{
			Percent: split.Percent,
			Tag:     split.Tag,
		}

		if split.IsLatest {
			target.Type = runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST
		} else {
			target.Type = runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION
			target.Revision = split.RevisionName
		}

		targets = append(targets, target)
	}

	return targets
}

// Promote promotes a revision by adjusting traffic split.
// Parameters:
//   - percent: Percentage of traffic to route to the latest revision (0-100)
//
// When percent is 100, all traffic goes to the latest revision.
// When percent is less than 100, the remaining traffic goes to the previous revision.
func (tm *TrafficManager) Promote(ctx context.Context, project, region, service string, percent int32) error {
	if percent < 0 || percent > 100 {
		return fmt.Errorf("invalid traffic percentage: %d (must be 0-100)", percent)
	}

	// Get current service to find revisions
	_, err := tm.client.GetService(ctx, project, region, service)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	var traffic []*runpb.TrafficTarget

	if percent == 100 {
		// Route all traffic to latest revision
		traffic = []*runpb.TrafficTarget{
			{
				Type:    runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST,
				Percent: 100,
			},
		}
	} else {
		// Get revisions to find the previous one
		revisions, err := tm.client.ListRevisions(ctx, project, region, service)
		if err != nil {
			return fmt.Errorf("failed to list revisions: %w", err)
		}

		if len(revisions) < 2 {
			// Not enough revisions for splitting, route all to latest
			traffic = []*runpb.TrafficTarget{
				{
					Type:    runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST,
					Percent: 100,
				},
			}
		} else {
			// Sort revisions by creation time (newest first)
			sortRevisionsByCreationTime(revisions)

			// Get the previous revision (second in the sorted list)
			previousRev := revisions[1].Name

			// Split traffic between latest and previous
			traffic = []*runpb.TrafficTarget{
				{
					Type:    runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST,
					Percent: percent,
				},
				{
					Type:     runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION,
					Revision: previousRev,
					Percent:  100 - percent,
				},
			}
		}
	}

	// Update traffic
	return tm.client.UpdateTraffic(ctx, project, region, service, traffic)
}

// Rollback rolls back to a specific revision.
func (tm *TrafficManager) Rollback(ctx context.Context, project, region, service, revision string) error {
	traffic := []*runpb.TrafficTarget{
		{
			Type:     runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION,
			Revision: revision,
			Percent:  100,
		},
	}

	return tm.client.UpdateTraffic(ctx, project, region, service, traffic)
}

// GetCurrentTraffic returns the current traffic allocation.
func (tm *TrafficManager) GetCurrentTraffic(ctx context.Context, project, region, service string) ([]TrafficSplit, error) {
	svc, err := tm.client.GetService(ctx, project, region, service)
	if err != nil {
		return nil, err
	}

	var splits []TrafficSplit
	for _, t := range svc.Traffic {
		split := TrafficSplit{
			Percent: t.Percent,
			Tag:     t.Tag,
		}

		if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST {
			split.IsLatest = true
			split.RevisionName = "LATEST"
		} else if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION {
			split.RevisionName = t.Revision
		}

		splits = append(splits, split)
	}

	return splits, nil
}

// sortRevisionsByCreationTime sorts revisions by creation time (newest first).
func sortRevisionsByCreationTime(revisions []*runpb.Revision) {
	sort.Slice(revisions, func(i, j int) bool {
		// Compare creation times (newest first)
		if revisions[i].CreateTime == nil || revisions[j].CreateTime == nil {
			return false
		}
		return revisions[i].CreateTime.AsTime().After(revisions[j].CreateTime.AsTime())
	})
}

// CanaryConfig defines configuration for canary deployments.
type CanaryConfig struct {
	// Steps defines the traffic percentages for each canary step.
	// Example: [10, 25, 50, 100]
	Steps []int32

	// AnalysisEnabled indicates if automated analysis should be performed.
	AnalysisEnabled bool

	// AnalysisThreshold is the failure threshold for automated analysis.
	AnalysisThreshold float64
}

// DefaultCanaryConfig returns a default canary configuration.
func DefaultCanaryConfig() *CanaryConfig {
	return &CanaryConfig{
		Steps:             []int32{10, 25, 50, 100},
		AnalysisEnabled:   false,
		AnalysisThreshold: 0.5,
	}
}
