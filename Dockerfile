# Build stage
FROM golang:1.21-alpine AS builder

# Install git for Go modules
RUN apk add --no-cache git

WORKDIR /workspace

# Bypass Go proxy due to corporate network issues
ENV GOPROXY=direct

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal/ internal/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o manager cmd/manager/main.go

# Final stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the binary from builder stage
COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]