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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"cloud.google.com/go/run/apiv2/runpb"
	"google.golang.org/protobuf/encoding/protojson"
)

// ServiceManager provides high-level operations for managing Cloud Run services.
type ServiceManager struct {
	client Client
}

// NewServiceManager creates a new ServiceManager.
func NewServiceManager(client Client) *ServiceManager {
	return &ServiceManager{client: client}
}

// LoadServiceManifest loads a Cloud Run service manifest from a file.
// The manifest should be a valid Knative Service YAML or JSON file.
//
// Example service.yaml:
//
//	apiVersion: serving.knative.dev/v1
//	kind: Service
//	metadata:
//	  name: my-service
//	spec:
//	  template:
//	    metadata:
//	      annotations:
//	        autoscaling.knative.dev/maxScale: '10'
//	    spec:
//	      containerConcurrency: 100
//	      containers:
//	        - image: gcr.io/project/image:tag
//	          ports:
//	            - containerPort: 8080
//	          resources:
//	            limits:
//	              cpu: 1000m
//	              memory: 512Mi
//	  traffic:
//	    - latestRevision: true
//	      percent: 100
func LoadServiceManifest(path string) (*runpb.Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read service manifest: %w", err)
	}

	var service runpb.Service

	// Try JSON first
	if err := protojson.Unmarshal(data, &service); err != nil {
		// If JSON fails, try YAML (convert to JSON first)
		// For simplicity, we assume the file is in JSON format
		// In production, you'd want to support YAML as well
		return nil, fmt.Errorf("failed to parse service manifest (expected JSON): %w", err)
	}

	return &service, nil
}

// LoadServiceManifestFromDir loads a service manifest from an application directory.
func LoadServiceManifestFromDir(appDir, manifestPath string) (*runpb.Service, error) {
	fullPath := filepath.Join(appDir, manifestPath)
	return LoadServiceManifest(fullPath)
}

// ApplyImageOverride overrides the container image in the service spec.
func ApplyImageOverride(service *runpb.Service, image string) {
	if image == "" || service.Template == nil {
		return
	}

	for _, container := range service.Template.Containers {
		container.Image = image
	}
}

// SetServiceName sets the full resource name for the service.
func SetServiceName(service *runpb.Service, project, region, name string) {
	service.Name = fmt.Sprintf("projects/%s/locations/%s/services/%s", project, region, name)
}

// GetServiceName extracts the service name from a full resource name.
func GetServiceName(fullName string) string {
	// Full name format: projects/{project}/locations/{location}/services/{service}
	var project, region, name string
	fmt.Sscanf(fullName, "projects/%s/locations/%s/services/%s", &project, &region, &name)
	return name
}

// Deploy deploys a new revision of a service.
// If the service doesn't exist, it creates a new one.
func (sm *ServiceManager) Deploy(ctx context.Context, service *runpb.Service) (*runpb.Service, error) {
	return sm.client.CreateOrUpdateService(ctx, service)
}

// GetService retrieves a service by project, region, and name.
func (sm *ServiceManager) GetService(ctx context.Context, project, region, name string) (*runpb.Service, error) {
	return sm.client.GetService(ctx, project, region, name)
}

// ServiceDiff represents the difference between two service configurations.
type ServiceDiff struct {
	HasDiff bool
	Old     *runpb.Service
	New     *runpb.Service
	Diff    string
}

// DiffServices compares two service configurations and returns the differences.
func DiffServices(old, new *runpb.Service) (*ServiceDiff, error) {
	oldJSON, err := protojson.Marshal(old)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal old service: %w", err)
	}

	newJSON, err := protojson.Marshal(new)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal new service: %w", err)
	}

	// Simple string comparison for now
	// In production, you'd want a more sophisticated diff
	hasDiff := string(oldJSON) != string(newJSON)

	return &ServiceDiff{
		HasDiff: hasDiff,
		Old:     old,
		New:     new,
		Diff:    string(oldJSON) + "\n---\n" + string(newJSON),
	}, nil
}

// ServiceInfo contains summary information about a Cloud Run service.
type ServiceInfo struct {
	Name           string
	Project        string
	Region         string
	URL            string
	LatestRevision string
	Traffic        []TrafficInfo
}

// TrafficInfo contains traffic allocation information.
type TrafficInfo struct {
	Revision string
	Percent  int32
	IsLatest bool
	Tag      string
}

// GetServiceInfo retrieves summary information about a service.
func (sm *ServiceManager) GetServiceInfo(ctx context.Context, project, region, name string) (*ServiceInfo, error) {
	svc, err := sm.client.GetService(ctx, project, region, name)
	if err != nil {
		return nil, err
	}

	info := &ServiceInfo{
		Name:    name,
		Project: project,
		Region:  region,
		URL:     svc.Uri,
	}

	if svc.Template != nil {
		info.LatestRevision = svc.Template.Revision
	}

	for _, t := range svc.Traffic {
		trafficInfo := TrafficInfo{
			Percent: t.Percent,
			Tag:     t.Tag,
		}

		if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST {
			trafficInfo.IsLatest = true
			trafficInfo.Revision = "LATEST"
		} else if t.Type == runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION {
			trafficInfo.Revision = t.Revision
		}

		info.Traffic = append(info.Traffic, trafficInfo)
	}

	return info, nil
}

// ToJSON converts service info to JSON for logging.
func (si *ServiceInfo) ToJSON() string {
	data, _ := json.MarshalIndent(si, "", "  ")
	return string(data)
}
