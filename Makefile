# Copyright 2025 The PipeCD Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# PipeCD Cloud Run Plugin Makefile
# This Makefile provides commands for building, testing, and releasing the plugin.

# Plugin metadata
PLUGIN_NAME := cloudrun
PLUGIN_VERSION := 0.1.0
PLUGIN_DESCRIPTION := "PipeCD Plugin for Google Cloud Run deployments"

# Build configuration
BUILD_DIR := ./build
CMD_DIR := ./cmd/cloudrun-plugin
GO := go
GOFLAGS := -v

# LDFLAGS for embedding version info
LDFLAGS := -ldflags "-X main.version=$(PLUGIN_VERSION) -X main.name=$(PLUGIN_NAME)"

# Platform targets for cross-compilation
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all
all: build

# =============================================================================
# Build Commands
# =============================================================================

.PHONY: build
build: ## Build the plugin binary for the current platform
	@echo "Building $(PLUGIN_NAME) plugin version $(PLUGIN_VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/plugin_$(PLUGIN_NAME) $(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/plugin_$(PLUGIN_NAME)"

.PHONY: build-all
build-all: $(PLATFORMS) ## Build for all supported platforms

.PHONY: $(PLATFORMS)
$(PLATFORMS):
	@mkdir -p $(BUILD_DIR)
	@GOOS=$(word 1,$(subst /, ,$@)) GOARCH=$(word 2,$(subst /, ,$@)) \
		$(GO) build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_$(subst /,_,$@)$(if $(findstring windows,$(word 1,$(subst /, ,$@))),.exe,) \
		$(CMD_DIR)
	@echo "Built for $@"

.PHONY: build-linux-amd64
build-linux-amd64: ## Build for Linux AMD64
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_linux_amd64 $(CMD_DIR)
	@echo "Built for linux/amd64: $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_linux_amd64"

.PHONY: build-linux-arm64
build-linux-arm64: ## Build for Linux ARM64
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_linux_arm64 $(CMD_DIR)
	@echo "Built for linux/arm64: $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_linux_arm64"

.PHONY: build-darwin-amd64
build-darwin-amd64: ## Build for macOS AMD64
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_darwin_amd64 $(CMD_DIR)
	@echo "Built for darwin/amd64: $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_darwin_amd64"

.PHONY: build-darwin-arm64
build-darwin-arm64: ## Build for macOS ARM64 (Apple Silicon)
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_darwin_arm64 $(CMD_DIR)
	@echo "Built for darwin/arm64: $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_darwin_arm64"

# =============================================================================
# Development Commands
# =============================================================================

.PHONY: run
run: build ## Build and run the plugin locally
	$(BUILD_DIR)/plugin_$(PLUGIN_NAME)

.PHONY: run-local
run-local: build ## Run the plugin with local piped (requires piped config)
	@echo "Running plugin locally..."
	@echo "Make sure you have configured piped to use this plugin"
	$(BUILD_DIR)/plugin_$(PLUGIN_NAME)

.PHONY: dev
dev: ## Run in development mode with hot reload (requires air)
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	air -c .air.toml

# =============================================================================
# Testing Commands
# =============================================================================

.PHONY: test
test: ## Run all tests
	$(GO) test -v ./...

.PHONY: test-unit
test-unit: ## Run unit tests only
	$(GO) test -v -short ./...

.PHONY: test-integration
test-integration: ## Run integration tests (requires GCP credentials)
	$(GO) test -v -run Integration ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: test-race
test-race: ## Run tests with race detector
	$(GO) test -v -race ./...

# =============================================================================
# Linting and Code Quality
# =============================================================================

.PHONY: lint
lint: ## Run golangci-lint
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run --fix ./...

.PHONY: fmt
fmt: ## Format Go code
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: tidy
tidy: ## Tidy go modules
	$(GO) mod tidy

.PHONY: verify
verify: fmt vet lint test ## Run all verification steps

# =============================================================================
# Dependency Management
# =============================================================================

.PHONY: deps
deps: ## Download dependencies
	$(GO) mod download

.PHONY: deps-update
deps-update: ## Update dependencies
	$(GO) get -u ./...
	$(GO) mod tidy

.PHONY: deps-verify
deps-verify: ## Verify dependencies
	$(GO) mod verify

# =============================================================================
# Release Commands
# =============================================================================

.PHONY: release
release: clean build-all ## Build release artifacts for all platforms
	@echo "Creating release artifacts..."
	@mkdir -p $(BUILD_DIR)/release
	@for f in $(BUILD_DIR)/plugin_$(PLUGIN_NAME)_*; do \
		if [ -f "$$f" ]; then \
			cp "$$f" $(BUILD_DIR)/release/; \
		fi; \
	done
	@echo "Release artifacts created in $(BUILD_DIR)/release/"

.PHONY: release-checksum
release-checksum: release ## Generate checksums for release artifacts
	@cd $(BUILD_DIR)/release && sha256sum * > checksums.txt
	@echo "Checksums generated: $(BUILD_DIR)/release/checksums.txt"

.PHONY: release-compress
release-compress: release ## Compress release artifacts
	@cd $(BUILD_DIR)/release && for f in plugin_*; do \
		if [ -f "$$f" ] && [ "$$f" != "checksums.txt" ]; then \
			gzip -k "$$f"; \
		fi; \
	done
	@echo "Compressed artifacts created"

# =============================================================================
# Docker Commands
# =============================================================================

.PHONY: docker-build
docker-build: ## Build Docker image
	docker build -t pipecd-plugin-$(PLUGIN_NAME):$(PLUGIN_VERSION) .

.PHONY: docker-run
docker-run: ## Run Docker container
	docker run --rm -it pipecd-plugin-$(PLUGIN_NAME):$(PLUGIN_VERSION)

# =============================================================================
# Cleanup Commands
# =============================================================================

.PHONY: clean
clean: ## Clean build artifacts
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Cleaned build artifacts"

.PHONY: clean-all
clean-all: clean ## Clean all artifacts including downloaded tools
	@rm -rf bin/
	@$(GO) clean -cache
	@echo "Cleaned all artifacts"

# =============================================================================
# Documentation
# =============================================================================

.PHONY: docs
docs: ## Generate documentation
	@echo "Generating documentation..."
	@which godoc > /dev/null || (echo "Installing godoc..." && go install golang.org/x/tools/cmd/godoc@latest)
	@echo "Documentation server starting at http://localhost:6060"
	godoc -http=:6060

# =============================================================================
# Help
# =============================================================================

.PHONY: help
help: ## Display this help message
	@echo "PipeCD Cloud Run Plugin - Makefile Commands"
	@echo "============================================"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build              - Build for current platform"
	@echo "  make build-all          - Build for all platforms"
	@echo "  make build-linux-amd64  - Build for Linux AMD64"
	@echo "  make build-linux-arm64  - Build for Linux ARM64"
	@echo "  make build-darwin-amd64 - Build for macOS AMD64"
	@echo "  make build-darwin-arm64 - Build for macOS ARM64"
	@echo ""
	@echo "Development Commands:"
	@echo "  make run                - Build and run locally"
	@echo "  make dev                - Run with hot reload"
	@echo ""
	@echo "Testing Commands:"
	@echo "  make test               - Run all tests"
	@echo "  make test-unit          - Run unit tests"
	@echo "  make test-integration   - Run integration tests"
	@echo "  make test-coverage      - Run tests with coverage"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint               - Run linter"
	@echo "  make lint-fix           - Run linter with auto-fix"
	@echo "  make fmt                - Format code"
	@echo "  make vet                - Run go vet"
	@echo "  make verify             - Run all checks"
	@echo ""
	@echo "Release Commands:"
	@echo "  make release            - Build release artifacts"
	@echo "  make release-checksum   - Generate checksums"
	@echo ""
	@echo "Other Commands:"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make deps               - Download dependencies"
	@echo "  make help               - Show this help"

# Default target
.DEFAULT_GOAL := help
