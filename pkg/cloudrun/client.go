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

// Package cloudrun provides a client for interacting with the Google Cloud Run API.
//
// This package wraps the official Google Cloud Run Go client library to provide
// a simplified interface for the PipeCD plugin to manage Cloud Run services.
package cloudrun

import (
	"context"
	"fmt"
	"time"

	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// Client defines the interface for interacting with Cloud Run API.
// This interface abstracts the Cloud Run operations needed by the plugin.
type Client interface {
	// GetService retrieves a Cloud Run service by name.
	// Parameters:
	//   - project: GCP project ID
	//   - region: GCP region
	//   - service: Service name
	GetService(ctx context.Context, project, region, service string) (*runpb.Service, error)

	// CreateOrUpdateService creates a new service or updates an existing one.
	// Updating a service creates a new revision automatically.
	CreateOrUpdateService(ctx context.Context, service *runpb.Service) (*runpb.Service, error)

	// UpdateTraffic updates traffic allocation for a service.
	// Parameters:
	//   - project: GCP project ID
	//   - region: GCP region
	//   - service: Service name
	//   - traffic: List of traffic targets
	UpdateTraffic(ctx context.Context, project, region, service string, traffic []*runpb.TrafficTarget) error

	// ListRevisions lists all revisions of a service.
	ListRevisions(ctx context.Context, project, region, service string) ([]*runpb.Revision, error)

	// GetRevision gets a specific revision.
	GetRevision(ctx context.Context, project, region, service, revision string) (*runpb.Revision, error)

	// DeleteRevision deletes a specific revision.
	DeleteRevision(ctx context.Context, project, region, service, revision string) error

	// WaitForServiceReady waits for a service to be ready.
	WaitForServiceReady(ctx context.Context, project, region, service string) error

	// Close closes the client connection.
	Close() error
}

// client implements the Client interface using the official Cloud Run Go client.
type client struct {
	servicesClient  *run.ServicesClient
	revisionsClient *run.RevisionsClient
}

// NewClient creates a new Cloud Run API client.
//
// Parameters:
//   - ctx: Context for the client creation
//   - credentialsFile: Path to GCP service account key file (optional)
//
// If credentialsFile is empty, Application Default Credentials will be used.
// This is useful for local development with `gcloud auth application-default login`.
//
// Example:
//
//	// With service account key
//	client, err := cloudrun.NewClient(ctx, "/path/to/key.json")
//
//	// With Application Default Credentials
//	client, err := cloudrun.NewClient(ctx, "")
func NewClient(ctx context.Context, credentialsFile string) (Client, error) {
	var opts []option.ClientOption
	if credentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsFile))
	}

	// Create the services client for service operations
	servicesClient, err := run.NewServicesClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create services client: %w", err)
	}

	// Create the revisions client for revision operations
	revisionsClient, err := run.NewRevisionsClient(ctx, opts...)
	if err != nil {
		servicesClient.Close()
		return nil, fmt.Errorf("failed to create revisions client: %w", err)
	}

	return &client{
		servicesClient:  servicesClient,
		revisionsClient: revisionsClient,
	}, nil
}

// GetService retrieves a Cloud Run service by name.
func (c *client) GetService(ctx context.Context, project, region, service string) (*runpb.Service, error) {
	name := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, region, service)
	return c.servicesClient.GetService(ctx, &runpb.GetServiceRequest{
		Name: name,
	})
}

// CreateOrUpdateService creates a new service or updates an existing one.
// When updating, a new revision is automatically created.
func (c *client) CreateOrUpdateService(ctx context.Context, service *runpb.Service) (*runpb.Service, error) {
	// Check if service exists
	_, err := c.servicesClient.GetService(ctx, &runpb.GetServiceRequest{
		Name: service.Name,
	})

	if err != nil {
		// Service doesn't exist, create it
		return c.servicesClient.CreateService(ctx, &runpb.CreateServiceRequest{
			Parent:    getParentFromServiceName(service.Name),
			ServiceId: getServiceIDFromServiceName(service.Name),
			Service:   service,
		})
	}

	// Service exists, update it
	// Use update mask to only update specific fields
	updateMask := &fieldmaskpb.FieldMask{
		Paths: []string{
			"template",
			"traffic",
		},
	}

	return c.servicesClient.UpdateService(ctx, &runpb.UpdateServiceRequest{
		Service:    service,
		UpdateMask: updateMask,
	})
}

// UpdateTraffic updates traffic allocation for a service.
func (c *client) UpdateTraffic(ctx context.Context, project, region, service string, traffic []*runpb.TrafficTarget) error {
	name := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, region, service)

	// Get current service
	svc, err := c.servicesClient.GetService(ctx, &runpb.GetServiceRequest{
		Name: name,
	})
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	// Update traffic configuration
	svc.Traffic = traffic

	// Apply update
	_, err = c.servicesClient.UpdateService(ctx, &runpb.UpdateServiceRequest{
		Service: svc,
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{"traffic"},
		},
	})

	return err
}

// ListRevisions lists all revisions of a service.
func (c *client) ListRevisions(ctx context.Context, project, region, service string) ([]*runpb.Revision, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, region, service)

	iter := c.revisionsClient.ListRevisions(ctx, &runpb.ListRevisionsRequest{
		Parent: parent,
	})

	var revisions []*runpb.Revision
	for {
		rev, err := iter.Next()
		if err != nil {
			// Check if we've reached the end
			if err.Error() == "iterator done" {
				break
			}
			return nil, fmt.Errorf("failed to list revisions: %w", err)
		}
		revisions = append(revisions, rev)
	}

	return revisions, nil
}

// GetRevision gets a specific revision.
func (c *client) GetRevision(ctx context.Context, project, region, service, revision string) (*runpb.Revision, error) {
	name := fmt.Sprintf("projects/%s/locations/%s/services/%s/revisions/%s",
		project, region, service, revision)
	return c.revisionsClient.GetRevision(ctx, &runpb.GetRevisionRequest{
		Name: name,
	})
}

// DeleteRevision deletes a specific revision.
func (c *client) DeleteRevision(ctx context.Context, project, region, service, revision string) error {
	name := fmt.Sprintf("projects/%s/locations/%s/services/%s/revisions/%s",
		project, region, service, revision)
	return c.revisionsClient.DeleteRevision(ctx, &runpb.DeleteRevisionRequest{
		Name: name,
	})
}

// WaitForServiceReady waits for a service to be ready.
func (c *client) WaitForServiceReady(ctx context.Context, project, region, service string) error {
	name := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, region, service)

	// Poll until service is ready
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			svc, err := c.servicesClient.GetService(ctx, &runpb.GetServiceRequest{
				Name: name,
			})
			if err != nil {
				return err
			}

			// Check if service is ready
			if svc.Conditions != nil {
				for _, cond := range svc.Conditions {
					if cond.Type == runpb.Condition_READY {
						if cond.State == runpb.Condition_CONDITION_SUCCEEDED {
							return nil
						}
						if cond.State == runpb.Condition_CONDITION_FAILED {
							return fmt.Errorf("service failed to become ready: %s", cond.Message)
						}
					}
				}
			}
		}
	}
}

// Close closes the client connections.
func (c *client) Close() error {
	if c.servicesClient != nil {
		c.servicesClient.Close()
	}
	if c.revisionsClient != nil {
		c.revisionsClient.Close()
	}
	return nil
}

// Helper function to extract parent from service name
func getParentFromServiceName(name string) string {
	// name format: projects/{project}/locations/{location}/services/{service}
	// parent format: projects/{project}/locations/{location}
	// Find the last slash and truncate
	lastSlash := 0
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			lastSlash = i
			break
		}
	}
	if lastSlash > 0 {
		return name[:lastSlash]
	}
	return name
}

// Helper function to extract service ID from service name
func getServiceIDFromServiceName(name string) string {
	// name format: projects/{project}/locations/{location}/services/{service}
	// Extract the service ID (after last slash)
	lastSlash := 0
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			lastSlash = i
			break
		}
	}
	if lastSlash > 0 && lastSlash < len(name)-1 {
		return name[lastSlash+1:]
	}
	return name
}
