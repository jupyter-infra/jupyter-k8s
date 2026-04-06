# Build stage
FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

# Bypass Go proxy due to corporate network issues
ENV GOPROXY=direct

# Set working directory
WORKDIR /workspace

# Copy go.mod and go.sum files first to leverage Docker cache
COPY go.mod go.mod
COPY go.sum go.sum

# Download dependencies
RUN go mod download

# Copy the source code (api, internal, and cmd directories)
COPY api/ api/
COPY internal/ internal/
COPY cmd/ cmd/

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o aws-plugin ./cmd/aws-plugin

# Use distroless as minimal base image to package the binary
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/aws-plugin /aws-plugin
USER 65532:65532

# Expose default plugin port
EXPOSE 8080

# Set environment variables
ENV PLUGIN_PORT=8080

ENTRYPOINT ["/aws-plugin"]
