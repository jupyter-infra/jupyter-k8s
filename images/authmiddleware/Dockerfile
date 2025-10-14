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

# Copy the source code (both internal and cmd directories)
COPY internal/ internal/
COPY cmd/ cmd/

# Build
# the GOARCH has no default value to allow the binary to be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o authmiddleware ./cmd/authmiddleware

# Use distroless as minimal base image to package the binary
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/authmiddleware /authmiddleware
USER 65532:65532

# Expose port
EXPOSE 8080

# Set environment variables
ENV PORT=8080 \
    READ_TIMEOUT=10s \
    WRITE_TIMEOUT=10s \
    SHUTDOWN_TIMEOUT=30s

ENTRYPOINT ["/authmiddleware"]