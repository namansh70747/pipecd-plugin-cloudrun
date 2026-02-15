# PipeCD Cloud Run Plugin

A [PipeCD](https://pipecd.dev) plugin for deploying applications to [Google Cloud Run](https://cloud.google.com/run) with support for progressive delivery strategies like canary deployments and traffic splitting.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
- [Deployment Strategies](#deployment-strategies)
- [Development](#development)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)

## Overview

This plugin enables PipeCD to deploy containerized applications to Google Cloud Run. It implements the PipeCD Plugin SDK (pipedv1) to provide seamless integration with the PipeCD control plane.

### What is PipeCD?

PipeCD is a GitOps-style continuous delivery platform that supports multiple application platforms including Kubernetes, Cloud Run, Terraform, ECS, and Lambda. With the new plugin architecture (pipedv1), anyone can develop custom plugins to extend PipeCD's capabilities.

### What is Cloud Run?

Google Cloud Run is a fully managed compute platform that automatically scales your stateless containers. It abstracts away all infrastructure management, allowing you to focus on building great applications.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    PipeCD Control Plane                          │
│         (Web UI, API, Metadata Storage)                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Piped (Agent)                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   Plugin    │  │   Plugin    │  │   Plugin    │              │
│  │ Kubernetes  │  │  Cloud Run  │  │  Terraform  │              │
│  │  (gRPC)     │  │  (gRPC)     │  │  (gRPC)     │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
│                                                                  │
│  Piped Core: Controls deployment flows, handles Git operations   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ HTTPS
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Google Cloud Run                              │
│         (Managed container platform)                             │
└─────────────────────────────────────────────────────────────────┘
```

### How It Works

1. **Plugin Registration**: The plugin binary is loaded by piped on startup
2. **Deployment Trigger**: PipeCD control plane triggers a deployment
3. **Stage Execution**: Piped sends stage execution requests to the plugin via gRPC
4. **Cloud Run API**: The plugin interacts with Cloud Run API to deploy services
5. **Progress Reporting**: The plugin reports stage status back to piped

## Features

### Deployment Stages

| Stage | Description |
|-------|-------------|
| `CLOUDRUN_SYNC` | Deploy a new Cloud Run revision |
| `CLOUDRUN_PROMOTE` | Promote revision by adjusting traffic split |
| `CLOUDRUN_ROLLBACK` | Rollback to a previous revision |
| `CLOUDRUN_CANARY_CLEANUP` | Clean up old revisions |

### Deployment Strategies

1. **Quick Sync**: Deploy and route 100% traffic immediately
2. **Canary Deployment**: Gradually shift traffic (e.g., 10% → 50% → 100%)
3. **Blue-Green Deployment**: Deploy, test, then switch all traffic
4. **A/B Testing**: Split traffic between versions

### Key Capabilities

- Progressive delivery with traffic splitting
- Automatic rollback on failure
- Revision cleanup and management
- Multi-environment support (staging, production)
- GCP authentication via service accounts

## Installation

### Prerequisites

- Go 1.24 or later
- Access to a GCP project with Cloud Run API enabled
- PipeCD control plane (local or remote)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/your-org/pipecd-plugin-cloudrun.git
cd pipecd-plugin-cloudrun

# Build for current platform
make build

# Build for all platforms
make build-all
```

### Download Pre-built Binary

```bash
# Download the latest release
curl -L -o plugin_cloudrun \
  https://github.com/your-org/pipecd-plugin-cloudrun/releases/download/v0.1.0/plugin_cloudrun_linux_amd64

# Make executable
chmod +x plugin_cloudrun
```

## Configuration

### 1. Piped Configuration

Create a `piped.yaml` file:

```yaml
apiVersion: pipecd.dev/v1beta1
kind: Piped
spec:
  apiAddress: localhost:8080
  projectID: your-pipecd-project
  pipedID: your-piped-id
  pipedKeyData: <base64-encoded-key>
  insecure: true

  repositories:
    - repoId: my-apps
      remote: https://github.com/your-org/your-apps-repo.git
      branch: main

  plugins:
    - name: cloudrun
      port: 7001
      url: https://github.com/your-org/releases/download/v0.1.0/plugin_cloudrun_linux_amd64
      config:
        projectID: my-default-project
        region: us-central1
      deployTargets:
        - name: staging
          config:
            projectID: staging-project
            region: us-central1
            credentialsFile: /etc/piped/gcp-staging-key.json
        - name: production
          config:
            projectID: production-project
            region: us-east1
            credentialsFile: /etc/piped/gcp-prod-key.json
```

### 2. Application Configuration

Create a `.pipe.yaml` in your application directory:

```yaml
apiVersion: pipecd.dev/v1beta1
kind: CloudRunApp
spec:
  name: my-cloudrun-app
  labels:
    env: production
    team: platform

  input:
    serviceManifestPath: service.yaml
    image: gcr.io/my-project/my-app:v1.0.0

  # Quick sync (default)
  quickSync:
    prune: true
```

### 3. Service Manifest

Create a `service.yaml` file:

```yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: my-service
  labels:
    app: my-service
spec:
  template:
    metadata:
      annotations:
        autoscaling.knative.dev/maxScale: '10'
    spec:
      containerConcurrency: 100
      containers:
        - image: gcr.io/my-project/my-app:latest
          ports:
            - containerPort: 8080
          resources:
            limits:
              cpu: 1000m
              memory: 512Mi
  traffic:
    - latestRevision: true
      percent: 100
```

## Deployment Strategies

### Quick Sync (Default)

Deploys the new revision and routes 100% traffic immediately.

```yaml
spec:
  quickSync:
    prune: true
```

### Canary Deployment

Gradually shifts traffic to the new revision.

```yaml
spec:
  pipeline:
    stages:
      - name: CLOUDRUN_SYNC
        with:
          skipTrafficShift: true
      - name: CLOUDRUN_PROMOTE
        with:
          percent: 10
      - name: WAIT
        with:
          duration: 5m
      - name: CLOUDRUN_PROMOTE
        with:
          percent: 50
      - name: WAIT
        with:
          duration: 10m
      - name: CLOUDRUN_PROMOTE
        with:
          percent: 100
      - name: CLOUDRUN_CANARY_CLEANUP
```

### Blue-Green Deployment

Deploys a new revision, tests it, then switches all traffic.

```yaml
spec:
  pipeline:
    stages:
      - name: CLOUDRUN_SYNC
        with:
          skipTrafficShift: true
      - name: CLOUDRUN_PROMOTE
        with:
          percent: 0  # Creates tagged URL for testing
      - name: WAIT_APPROVAL
      - name: CLOUDRUN_PROMOTE
        with:
          percent: 100
      - name: CLOUDRUN_CANARY_CLEANUP
```

## Development

### Project Structure

```
pipecd-plugin-cloudrun/
├── cmd/
│   └── cloudrun-plugin/     # Plugin entry point
│       └── main.go
├── pkg/
│   ├── plugin/              # Plugin implementation
│   │   ├── plugin.go        # Main plugin logic
│   │   ├── stages.go        # Stage definitions
│   │   ├── stage_sync.go    # SYNC stage
│   │   ├── stage_promote.go # PROMOTE stage
│   │   ├── stage_rollback.go
│   │   └── stage_cleanup.go
│   ├── config/              # Configuration structures
│   │   ├── piped.go         # Piped config
│   │   └── application.go   # App config
│   └── cloudrun/            # Cloud Run API client
│       ├── client.go        # API client
│       ├── service.go       # Service management
│       ├── revision.go      # Revision management
│       └── traffic.go       # Traffic management
├── examples/                # Example configurations
├── test/                    # Tests
├── Makefile
└── README.md
```

### Build Commands

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Run linter
make lint

# Format code
make fmt
```

### Local Development

1. **Set up local PipeCD control plane**:
   ```bash
   git clone https://github.com/pipe-cd/pipecd.git
   cd pipecd
   make run/pipecd
   ```

2. **Build and run the plugin**:
   ```bash
   cd pipecd-plugin-cloudrun
   make build
   ./build/plugin_cloudrun
   ```

3. **Configure piped** to use your local plugin:
   ```yaml
   plugins:
     - name: cloudrun
       port: 7001
       url: file:///path/to/your/plugin_cloudrun
   ```

4. **Run piped**:
   ```bash
   cd pipecd
   make run/piped CONFIG_FILE=/path/to/piped.yaml
   ```

## Troubleshooting

### Plugin Not Starting

**Symptom**: Piped fails to start the plugin

**Solutions**:
- Check plugin binary path in piped config
- Verify plugin binary is executable: `chmod +x plugin_cloudrun`
- Check port availability
- Review piped logs: `kubectl logs -f deployment/piped`

### GCP Authentication Failed

**Symptom**: Cloud Run API calls fail with authentication error

**Solutions**:
- Verify service account has `roles/run.developer` role
- Check credentials file path is correct
- Ensure Cloud Run API is enabled: `gcloud services enable run.googleapis.com`
- Test with Application Default Credentials locally

### Service Manifest Not Found

**Symptom**: Plugin cannot find service.yaml

**Solutions**:
- Verify `serviceManifestPath` in app config
- Check path is relative to application directory
- Ensure file exists in git repository
- Check file is committed to git

### Traffic Not Shifting

**Symptom**: Traffic split not working as expected

**Solutions**:
- Check stage configuration syntax
- Verify service name matches manifest
- Review Cloud Run console for traffic allocation
- Check plugin logs for errors

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Setup

1. Fork the repository
2. Clone your fork: `git clone https://github.com/your-username/pipecd-plugin-cloudrun.git`
3. Create a branch: `git checkout -b feature/my-feature`
4. Make changes and test
5. Submit a pull request

### Code Standards

- Follow Go best practices
- Add tests for new features
- Run `make verify` before submitting
- Update documentation as needed

## Resources

- [PipeCD Documentation](https://pipecd.dev/docs)
- [PipeCD Plugin SDK](https://github.com/pipe-cd/piped-plugin-sdk-go)
- [Cloud Run Documentation](https://cloud.google.com/run/docs)
- [Cloud Run API Reference](https://cloud.google.com/run/docs/reference/rest)

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [PipeCD](https://pipecd.dev) - The continuous delivery platform
- [Google Cloud Run](https://cloud.google.com/run) - The serverless container platform
- [PipeCD Plugin SDK](https://github.com/pipe-cd/piped-plugin-sdk-go) - The plugin development kit
