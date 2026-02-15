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

# PipeCD Cloud Run Plugin - Dockerfile
# This Dockerfile builds a minimal container image for the plugin.

# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the plugin
RUN make build-linux-amd64

# Final stage
FROM alpine:3.19

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN adduser -D -u 1000 pipecd

# Set working directory
WORKDIR /app

# Copy plugin binary from builder
COPY --from=builder /build/build/plugin_cloudrun_linux_amd64 /app/plugin_cloudrun

# Change ownership to non-root user
RUN chown -R pipecd:pipecd /app

# Switch to non-root user
USER pipecd

# Expose default plugin port
EXPOSE 7001

# Run the plugin
ENTRYPOINT ["/app/plugin_cloudrun"]
