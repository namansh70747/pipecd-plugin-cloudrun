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
	"time"

	"cloud.google.com/go/run/apiv2/runpb"
)

// RevisionManager provides operations for managing Cloud Run revisions.
type RevisionManager struct {
	client Client
}

// NewRevisionManager creates a new RevisionManager.
func NewRevisionManager(client Client) *RevisionManager {
	return &RevisionManager{client: client}
}

// RevisionInfo contains information about a Cloud Run revision.
type RevisionInfo struct {
	Name           string
	Service        string
	Project        string
	Region         string
	Image          string
	CreatedAt      time.Time
	TrafficPercent int32
	IsLatest       bool
	Conditions     map[string]bool
}

// ListRevisions lists all revisions for a service.
func (rm *RevisionManager) ListRevisions(ctx context.Context, project, region, service string) ([]*RevisionInfo, error) {
	revisions, err := rm.client.ListRevisions(ctx, project, region, service)
	if err != nil {
		return nil, err
	}

	// Get current service to find latest revision
	svc, err := rm.client.GetService(ctx, project, region, service)
	if err != nil {
		return nil, err
	}

	latestRevision := ""
	if svc.Template != nil {
		latestRevision = svc.Template.Revision
	}

	// Build traffic map
	trafficMap := make(map[string]int32)
	for _, t := range svc.Traffic {
		if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST {
			trafficMap[latestRevision] = t.Percent
		} else if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION {
			trafficMap[t.Revision] = t.Percent
		}
	}

	var infos []*RevisionInfo
	for _, rev := range revisions {
		info := rm.buildRevisionInfo(rev, latestRevision, trafficMap)
		infos = append(infos, info)
	}

	// Sort by creation time (newest first)
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].CreatedAt.After(infos[j].CreatedAt)
	})

	return infos, nil
}

// GetRevision gets detailed information about a specific revision.
func (rm *RevisionManager) GetRevision(ctx context.Context, project, region, service, revision string) (*RevisionInfo, error) {
	rev, err := rm.client.GetRevision(ctx, project, region, service, revision)
	if err != nil {
		return nil, err
	}

	// Get service for traffic info
	svc, err := rm.client.GetService(ctx, project, region, service)
	if err != nil {
		return nil, err
	}

	latestRevision := ""
	if svc.Template != nil {
		latestRevision = svc.Template.Revision
	}

	trafficMap := make(map[string]int32)
	for _, t := range svc.Traffic {
		if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST {
			trafficMap[latestRevision] = t.Percent
		} else if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION {
			trafficMap[t.Revision] = t.Percent
		}
	}

	return rm.buildRevisionInfo(rev, latestRevision, trafficMap), nil
}

// DeleteRevision deletes a specific revision.
func (rm *RevisionManager) DeleteRevision(ctx context.Context, project, region, service, revision string) error {
	return rm.client.DeleteRevision(ctx, project, region, service, revision)
}

// CleanupOldRevisions removes old revisions that have no traffic.
// Parameters:
//   - keepCount: Number of recent revisions to keep
//   - keepLatest: Whether to always keep the latest revision
func (rm *RevisionManager) CleanupOldRevisions(ctx context.Context, project, region, service string, keepCount int, keepLatest bool) error {
	revisions, err := rm.ListRevisions(ctx, project, region, service)
	if err != nil {
		return err
	}

	if len(revisions) <= keepCount {
		return nil // Nothing to clean up
	}

	// Get latest revision name
	svc, err := rm.client.GetService(ctx, project, region, service)
	if err != nil {
		return err
	}

	latestRevision := ""
	if svc.Template != nil {
		latestRevision = svc.Template.Revision
	}

	// Delete old revisions with no traffic
	deleted := 0
	for i, rev := range revisions {
		// Keep the specified number of recent revisions
		if i < keepCount {
			continue
		}

		// Skip if this is the latest revision and keepLatest is true
		if keepLatest && rev.Name == latestRevision {
			continue
		}

		// Only delete revisions with 0% traffic
		if rev.TrafficPercent == 0 {
			if err := rm.client.DeleteRevision(ctx, project, region, service, rev.Name); err != nil {
				return fmt.Errorf("failed to delete revision %s: %w", rev.Name, err)
			}
			deleted++
		}
	}

	return nil
}

// GetLatestRevision returns the latest revision of a service.
func (rm *RevisionManager) GetLatestRevision(ctx context.Context, project, region, service string) (*RevisionInfo, error) {
	revisions, err := rm.ListRevisions(ctx, project, region, service)
	if err != nil {
		return nil, err
	}

	if len(revisions) == 0 {
		return nil, fmt.Errorf("no revisions found for service %s", service)
	}

	return revisions[0], nil
}

// GetPreviousRevision returns the previous revision (second most recent).
func (rm *RevisionManager) GetPreviousRevision(ctx context.Context, project, region, service string) (*RevisionInfo, error) {
	revisions, err := rm.ListRevisions(ctx, project, region, service)
	if err != nil {
		return nil, err
	}

	if len(revisions) < 2 {
		return nil, fmt.Errorf("no previous revision found for service %s", service)
	}

	return revisions[1], nil
}

// buildRevisionInfo builds a RevisionInfo from a runpb.Revision.
func (rm *RevisionManager) buildRevisionInfo(rev *runpb.Revision, latestRevision string, trafficMap map[string]int32) *RevisionInfo {
	info := &RevisionInfo{
		Name:           rev.Name,
		TrafficPercent: trafficMap[rev.Name],
		IsLatest:       rev.Name == latestRevision,
		Conditions:     make(map[string]bool),
	}

	if rev.CreateTime != nil {
		info.CreatedAt = rev.CreateTime.AsTime()
	}

	// Extract image from containers
	if len(rev.Containers) > 0 {
		info.Image = rev.Containers[0].Image
	}

	// Extract conditions
	for _, cond := range rev.Conditions {
		info.Conditions[cond.Type] = cond.State == runpb.Condition_CONDITION_SUCCEEDED
	}

	return info
}

// RevisionDiff represents differences between two revisions.
type RevisionDiff struct {
	OldRevision *RevisionInfo
	NewRevision *RevisionInfo
	ImageDiff   string
	ConfigDiff  string
}

// CompareRevisions compares two revisions and returns their differences.
func CompareRevisions(old, new *RevisionInfo) *RevisionDiff {
	diff := &RevisionDiff{
		OldRevision: old,
		NewRevision: new,
	}

	if old.Image != new.Image {
		diff.ImageDiff = fmt.Sprintf("%s -> %s", old.Image, new.Image)
	}

	return diff
}
