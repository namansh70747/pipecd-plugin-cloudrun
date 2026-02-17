# PipeCD Cloud Run Plugin

A [PipeCD](https://pipecd.dev) plugin for deploying containerized applications to [Google Cloud Run](https://cloud.google.com/run) with progressive delivery support.

## Quick Start

### Build

```bash
make build
# Output: build/cloudrun-plugin
```

### Prerequisites

- Go 1.24+
- GCP project with Cloud Run API enabled
- PipeCD Control Plane & Piped

## GCP Setup

```bash
# Enable Cloud Run API
gcloud services enable run.googleapis.com

# Create service account
gcloud iam service-accounts create pipecd-cloudrun \
  --display-name="PipeCD Cloud Run Plugin"

# Grant permissions
PROJECT_ID=$(gcloud config get-value project)
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:pipecd-cloudrun@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/run.admin"

# Create key
gcloud iam service-accounts keys create gcp-key.json \
  --iam-account=pipecd-cloudrun@${PROJECT_ID}.iam.gserviceaccount.com
```

## Configuration

### Piped (`piped.yaml`)

```yaml
apiVersion: pipecd.dev/v1beta1
kind: Piped
spec:
  apiAddress: pipecd-api:443
  projectID: my-pipecd-project
  pipedID: my-piped-id
  pipedKeyData: <base64-key>
  
  plugins:
    - name: cloudrun
      port: 7001
      url: file:///path/to/cloudrun-plugin
      config:
        projectID: my-gcp-project
        region: us-central1
        credentialsFile: /path/to/gcp-key.json
```

### Application (`.pipe.yaml`)

**Quick Sync:**

```yaml
apiVersion: pipecd.dev/v1beta1
kind: CloudRunApp
spec:
  input:
    serviceManifestPath: service.yaml
    image: gcr.io/project/app:v1.0.0
```

**Canary:**

```yaml
apiVersion: pipecd.dev/v1beta1
kind: CloudRunApp
spec:
  input:
    serviceManifestPath: service.yaml
    image: gcr.io/project/app:v1.0.0
  pipeline:
    stages:
      - name: CLOUDRUN_SYNC
        with: {skipTrafficShift: true}
      - name: CLOUDRUN_PROMOTE
        with: {percent: 10}
      - name: WAIT
        with: {duration: 5m}
      - name: CLOUDRUN_PROMOTE
        with: {percent: 100}
      - name: CLOUDRUN_CANARY_CLEANUP
```

### Service Manifest (`service.yaml`)

```yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: my-service
spec:
  template:
    spec:
      containers:
        - image: gcr.io/project/app:v1.0.0
          ports:
            - containerPort: 8080
          resources:
            limits:
              cpu: "1000m"
              memory: "512Mi"
```

## Deployment Stages

| Stage | Purpose |
|-------|---------|
| `CLOUDRUN_SYNC` | Deploy new revision |
| `CLOUDRUN_PROMOTE` | Shift traffic % |
| `CLOUDRUN_ROLLBACK` | Revert to previous |
| `CLOUDRUN_CANARY_CLEANUP` | Remove old revisions |

## Plan Preview & Drift Detection

The plugin supports **Plan Preview** to show what will change before deployment and **Drift Detection** to identify when live state differs from Git.

### Plan Preview Features

- **Container Image Changes**: Shows current vs desired image versions
- **Traffic Allocation**: Displays traffic split differences
- **Resource Limits**: Compares CPU/memory configurations
- **Scaling Settings**: Identifies min/max instance changes
- **New Service Creation**: Highlights services that will be created

### Example Output

```
Target: production
Project: my-gcp-project
Region: us-central1
Service: my-service

📦 Container Image:
  - Current: gcr.io/project/app:v1.0.0
  + Desired: gcr.io/project/app:v2.0.0

🚦 Traffic Allocation:
  Current:
    - Latest revision: 100%
  Desired:
    + Latest revision: 90%
    + Revision my-service-00042: 10%

🔄 A new revision will be created with 2 change(s)
```

### Using Plan Preview

Plan preview is automatically triggered by PipeCD when:

- Opening a Pull Request with changes to `.pipe.yaml` or `service.yaml`
- Manual trigger from PipeCD UI

No additional configuration required - the plugin automatically compares Git state with live Cloud Run services.

## Development

```bash
make build              # Build binary
make test               # Run tests
make clean              # Clean build artifacts
```

### Project Structure

```
├── cmd/cloudrun-plugin/    # Entry point
├── pkg/
│   ├── plugin/            # Plugin implementation
│   ├── cloudrun/          # Cloud Run API client
│   └── config/            # Config structures
└── examples/              # Configuration examples
```

## Troubleshooting

**Authentication errors:**

```bash
gcloud auth activate-service-account --key-file=gcp-key.json
gcloud run services list
```

**Deployment failures:**

```bash
gcloud logging read "resource.type=cloud_run_revision" --limit=50
```

**Plugin not starting:**

```bash
chmod +x build/cloudrun-plugin
lsof -i :7001
```

## Resources

- [PipeCD Docs](https://pipecd.dev/docs)
- [Cloud Run Docs](https://cloud.google.com/run/docs)
- [Plugin SDK](https://github.com/pipe-cd/piped-plugin-sdk-go)

## License

Apache 2.0 - See [LICENSE](LICENSE)
