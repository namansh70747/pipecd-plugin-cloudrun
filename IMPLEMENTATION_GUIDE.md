# PipeCD Cloud Run Plugin - Implementation Guide

This document provides a detailed explanation of how the PipeCD Cloud Run Plugin is implemented and how it operates.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Plugin Lifecycle](#plugin-lifecycle)
3. [Code Organization](#code-organization)
4. [Key Components Explained](#key-components-explained)
5. [Deployment Flow](#deployment-flow)
6. [Stage Execution Details](#stage-execution-details)
7. [Error Handling](#error-handling)
8. [Testing Strategy](#testing-strategy)

## Architecture Overview

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Control Plane                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                  │
│  │   Web UI    │  │     API     │  │   Storage   │                  │
│  └─────────────┘  └─────────────┘  └─────────────┘                  │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC (Deployment commands)
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                          Piped (Agent)                              │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                     Plugin Manager                          │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │    │
│  │  │   Cloud Run │  │  Kubernetes │  │  Terraform  │          │    │
│  │  │   Plugin    │  │   Plugin    │  │   Plugin    │          │    │
│  │  │  (gRPC)     │  │  (gRPC)     │  │  (gRPC)     │          │    │
│  │  │  :7001      │  │  :7002      │  │  :7003      │          │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘          │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                     │
│  Core Functions: Git sync, deployment orchestration, logging        │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              │ HTTPS (Cloud Run API)
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                       Google Cloud Run                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                  │
│  │  Service A  │  │  Service B  │  │  Service C  │                  │
│  │  Revision 3 │  │  Revision 2 │  │  Revision 1 │                  │
│  │  (100%)     │  │  (100%)     │  │  (100%)     │                  │
│  └─────────────┘  └─────────────┘  └─────────────┘                  │
└─────────────────────────────────────────────────────────────────────┘
```

### Communication Flow

1. **Control Plane → Piped**: Deployment commands via gRPC
2. **Piped → Plugin**: Stage execution requests via gRPC
3. **Plugin → Cloud Run API**: Service management via HTTPS
4. **Plugin → Piped**: Stage status updates via gRPC
5. **Piped → Control Plane**: Deployment status updates

## Plugin Lifecycle

### 1. Initialization

```go
// main.go
func main() {
    // Create plugin instance
    cloudrunPlugin := plugin.NewCloudRunPlugin()
    
    // Register with SDK
    p, err := sdk.NewPlugin(
        "cloudrun",           // Plugin name
        "0.1.0",              // Plugin version
        sdk.WithDeploymentPlugin(cloudrunPlugin),
    )
    
    // Start gRPC server
    p.Run()
}
```

**What happens:**
- Plugin binary is loaded by piped
- Plugin starts gRPC server on configured port
- Plugin registers itself with piped
- Piped discovers supported stages via `FetchDefinedStages()`

### 2. Deployment Trigger

When a deployment is triggered:

```
1. Control Plane detects git change
2. Control Plane sends deployment command to Piped
3. Piped calls plugin.DetermineVersions()
4. Piped calls plugin.DetermineStrategy()
5. Piped calls plugin.BuildQuickSyncStages() or BuildPipelineSyncStages()
6. Piped executes stages sequentially via plugin.ExecuteStage()
```

### 3. Stage Execution

For each stage in the pipeline:

```
1. Piped sends ExecuteStage request
2. Plugin parses stage configuration
3. Plugin executes stage logic
4. Plugin reports status (SUCCESS/FAILURE)
5. Piped proceeds to next stage or triggers rollback
```

### 4. Shutdown

```
1. Piped sends shutdown signal
2. Plugin closes Cloud Run API connections
3. Plugin stops gRPC server
4. Plugin exits
```

## Code Organization

```
pipecd-plugin-cloudrun/
├── cmd/cloudrun-plugin/
│   └── main.go                    # Entry point
│                                   # - Creates plugin instance
│                                   # - Registers with SDK
│                                   # - Starts gRPC server
│
├── pkg/
│   ├── plugin/
│   │   ├── plugin.go              # Main plugin implementation
│   │   │                           # - Implements DeploymentPlugin interface
│   │   │                           # - Determines deployment strategy
│   │   │                           # - Builds deployment pipeline
│   │   │
│   │   ├── stages.go              # Stage definitions
│   │   │                           # - Stage names and descriptions
│   │   │                           # - Stage configuration structures
│   │   │
│   │   ├── stage_sync.go          # CLOUDRUN_SYNC implementation
│   │   │                           # - Reads service manifest
│   │   │                           # - Creates/updates Cloud Run service
│   │   │                           # - Manages traffic routing
│   │   │
│   │   ├── stage_promote.go       # CLOUDRUN_PROMOTE implementation
│   │   │                           # - Adjusts traffic split
│   │   │                           # - Supports progressive delivery
│   │   │
│   │   ├── stage_rollback.go      # CLOUDRUN_ROLLBACK implementation
│   │   │                           # - Rolls back to previous revision
│   │   │                           # - Routes 100% traffic to old revision
│   │   │
│   │   └── stage_cleanup.go       # CLOUDRUN_CANARY_CLEANUP implementation
│   │                               # - Removes old revisions
│   │                               # - Keeps specified number of revisions
│   │
│   ├── config/
│   │   ├── piped.go               # Piped-level configuration
│   │   │                           # - PluginConfig (global settings)
│   │   │                           # - DeployTargetConfig (per-environment)
│   │   │
│   │   └── application.go         # Application-level configuration
│   │                               # - ApplicationConfig (.pipe.yaml)
│   │                               # - InputConfig (deployment inputs)
│   │                               # - Pipeline configuration
│   │
│   └── cloudrun/
│       ├── client.go              # Cloud Run API client
│       │                           # - Low-level API operations
│       │                           # - Authentication handling
│       │
│       ├── service.go             # Service management
│       │                           # - Load service manifests
│       │                           # - Deploy services
│       │                           # - Service info retrieval
│       │
│       ├── revision.go            # Revision management
│       │                           # - List revisions
│       │                           # - Delete revisions
│       │                           # - Revision comparison
│       │
│       └── traffic.go             # Traffic management
│                                   # - Traffic splitting
│                                   # - Canary promotions
│                                   # - Rollback operations
│
└── examples/                       # Example configurations
    ├── piped-config/
    │   └── piped.yaml             # Piped configuration example
    └── application/
        ├── .pipe.yaml             # Application config examples
        └── service.yaml           # Cloud Run service manifest example
```

## Key Components Explained

### 1. Plugin Interface Implementation

The plugin implements the `DeploymentPlugin` interface from the PipeCD SDK:

```go
type DeploymentPlugin[PC, DTC, AC any] interface {
    // Returns supported stages
    FetchDefinedStages() []string
    
    // Determines application version
    DetermineVersions(ctx context.Context, cfg *PC, input *DetermineVersionsInput[AC]) (*DetermineVersionsResponse, error)
    
    // Determines deployment strategy (quick sync vs pipeline)
    DetermineStrategy(ctx context.Context, cfg *PC, input *DetermineStrategyInput[AC]) (*DetermineStrategyResponse, error)
    
    // Builds quick sync pipeline
    BuildQuickSyncStages(ctx context.Context, cfg *PC, input *BuildQuickSyncStagesInput) (*BuildQuickSyncStagesResponse, error)
    
    // Builds custom pipeline
    BuildPipelineSyncStages(ctx context.Context, cfg *PC, input *BuildPipelineSyncStagesInput) (*BuildPipelineSyncStagesResponse, error)
    
    // Executes a stage
    ExecuteStage(ctx context.Context, cfg *PC, deployTargets []*DeployTarget[DTC], input *ExecuteStageInput[AC]) (*ExecuteStageResponse, error)
}
```

### 2. Cloud Run Client

The Cloud Run client wraps the official Google Cloud Run Go client:

```go
type Client interface {
    GetService(ctx context.Context, project, region, service string) (*runpb.Service, error)
    CreateOrUpdateService(ctx context.Context, service *runpb.Service) (*runpb.Service, error)
    UpdateTraffic(ctx context.Context, project, region, service string, traffic []*runpb.TrafficTarget) error
    ListRevisions(ctx context.Context, project, region, service string) ([]*runpb.Revision, error)
    DeleteRevision(ctx context.Context, project, region, service, revision string) error
}
```

**Authentication Methods:**
1. Service Account Key File (recommended for CI/CD)
2. Application Default Credentials (for local development)
3. Workload Identity (for GKE deployments)

### 3. Stage Executor

The `StageExecutor` handles the execution of individual stages:

```go
type StageExecutor struct {
    // Shared dependencies can be added here
}

func (e *StageExecutor) ExecuteSyncStage(...) (*sdk.ExecuteStageResponse, error)
func (e *StageExecutor) ExecutePromoteStage(...) (*sdk.ExecuteStageResponse, error)
func (e *StageExecutor) ExecuteRollbackStage(...) (*sdk.ExecuteStageResponse, error)
func (e *StageExecutor) ExecuteCanaryCleanupStage(...) (*sdk.ExecuteStageResponse, error)
```

## Deployment Flow

### Quick Sync Deployment

```
User pushes commit to Git
         │
         ▼
┌─────────────────┐
│  Control Plane  │ Detects git change
│  triggers deploy │
└─────────────────┘
         │
         ▼
┌─────────────────┐
│     Piped       │ Calls DetermineVersions()
│                 │ Calls DetermineStrategy() → QuickSync
│                 │ Calls BuildQuickSyncStages()
└─────────────────┘
         │
         ▼
┌─────────────────┐
│     Plugin      │ Receives ExecuteStage(CLOUDRUN_SYNC)
│                 │ 1. Reads service.yaml
│                 │ 2. Creates/updates Cloud Run service
│                 │ 3. Routes 100% traffic to new revision
│                 │ 4. Returns SUCCESS
└─────────────────┘
         │
         ▼
┌─────────────────┐
│  Control Plane  │ Shows deployment as SUCCEEDED
│  (Web UI)       │
└─────────────────┘
```

### Canary Deployment

```
User pushes commit to Git
         │
         ▼
┌─────────────────┐
│  Control Plane  │ Detects git change
│  triggers deploy│
└─────────────────┘
         │
         ▼
┌─────────────────┐
│     Piped       │ Calls DetermineStrategy() → PipelineSync
│                 │ Calls BuildPipelineSyncStages()
└─────────────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│     Plugin      │────▶│     Plugin      │────▶│     Plugin      │
│  CLOUDRUN_SYNC  │     │ CLOUDRUN_PROMOTE│     │     WAIT        │
│  (0% traffic)   │     │  (10% traffic)  │     │   (5 minutes)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                              │
         ┌────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│     Plugin      │────▶│     Plugin      │────▶│     Plugin      │
│ CLOUDRUN_PROMOTE│     │     WAIT        │     │ CLOUDRUN_PROMOTE│
│  (50% traffic)  │     │  (10 minutes)   │     │ (100% traffic)  │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                              │
         ┌────────────────────────────────────────────────────┘
         ▼
┌─────────────────┐
│     Plugin      │
│ CLOUDRUN_CANARY │
│    CLEANUP      │
└─────────────────┘
```

## Stage Execution Details

### CLOUDRUN_SYNC

**Purpose**: Deploy a new Cloud Run revision

**Configuration**:
```yaml
- name: CLOUDRUN_SYNC
  with:
    skipTrafficShift: false  # If true, preserves existing traffic config
    prune: false             # If true, removes old revisions
```

**Execution Flow**:
1. Parse stage configuration
2. Create Cloud Run API client
3. Read service manifest from git
4. Apply image override (if specified)
5. Check if service exists
6. Create or update service
7. Wait for service to be ready
8. Optionally prune old revisions

**Output Metadata**:
- `revision`: New revision name
- `service_url`: Service URL
- `service_name`: Service name

### CLOUDRUN_PROMOTE

**Purpose**: Adjust traffic split between revisions

**Configuration**:
```yaml
- name: CLOUDRUN_PROMOTE
  with:
    percent: 10  # 0-100, percentage to new revision
```

**Execution Flow**:
1. Parse stage configuration
2. Validate percentage (0-100)
3. Get current traffic allocation
4. Calculate new traffic split
5. Update service traffic configuration
6. Verify traffic shift

**Traffic Split Logic**:
- `percent: 0` → 0% to new, 100% to old (smoke test)
- `percent: 10` → 10% to new, 90% to old (canary)
- `percent: 100` → 100% to new, 0% to old (full promotion)

### CLOUDRUN_ROLLBACK

**Purpose**: Rollback to a previous revision

**Configuration**:
```yaml
- name: CLOUDRUN_ROLLBACK
  with:
    revision: ""  # Empty = rollback to previous revision
```

**Execution Flow**:
1. Parse stage configuration
2. If revision specified, use it
3. Otherwise, find previous revision
4. Route 100% traffic to target revision
5. Verify rollback

### CLOUDRUN_CANARY_CLEANUP

**Purpose**: Remove old revisions

**Configuration**:
```yaml
- name: CLOUDRUN_CANARY_CLEANUP
  with:
    keepCount: 5   # Number of recent revisions to keep
    keepLatest: true  # Always keep latest revision
```

**Execution Flow**:
1. Parse stage configuration
2. List all revisions
3. Sort by creation time
4. Delete revisions with 0% traffic
5. Keep specified number of recent revisions

## Error Handling

### Stage Failure Handling

```go
func (e *StageExecutor) ExecuteSyncStage(...) (*sdk.ExecuteStageResponse, error) {
    // Log the error
    lp.Errorf("Failed to deploy service: %v", err)
    
    // Return failure response
    return &sdk.ExecuteStageResponse{
        Status: sdk.StageStatusFailure,
    }, err
}
```

### Retry Logic

The plugin relies on piped for retry handling:
- Piped retries failed stages based on configuration
- Plugin should be idempotent (safe to retry)
- Cloud Run API calls are naturally idempotent

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| Authentication failed | Invalid credentials | Check service account key |
| Service not found | Wrong service name | Verify manifest configuration |
| Revision not found | Revision deleted | Use existing revision name |
| Traffic split invalid | Percentage > 100 | Use valid percentage (0-100) |

## Testing Strategy

### Unit Tests

```go
func TestCloudRunPlugin_FetchDefinedStages(t *testing.T) {
    p := NewCloudRunPlugin()
    stages := p.FetchDefinedStages()
    
    // Verify all expected stages are returned
    expected := []string{
        StageCloudRunSync,
        StageCloudRunPromote,
        StageCloudRunRollback,
        StageCloudRunCanaryCleanup,
    }
    
    // Assert stages match expected
}
```

### Integration Tests

```go
func TestIntegration_DeployService(t *testing.T) {
    // Requires GCP credentials
    ctx := context.Background()
    client, _ := cloudrun.NewClient(ctx, "test-key.json")
    
    // Deploy test service
    // Verify deployment
    // Clean up
}
```

### Test Coverage

| Component | Coverage Target |
|-----------|-----------------|
| Plugin interface | 90% |
| Stage executors | 85% |
| Cloud Run client | 80% |
| Configuration | 95% |

---

This implementation guide should help you understand how the PipeCD Cloud Run Plugin works and how to extend it for your needs.
