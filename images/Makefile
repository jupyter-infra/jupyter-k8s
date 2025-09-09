# Makefile for building Jupyter images

# Build config
DOCKER := finch
# Set BUILD_OPTS to '--network host' on cloud desktop (if /etc/os-release exists), otherwise empty
# You might have to comment BUILD_OPTS out for devdesktop
BUILD_OPTS := $(shell if [ -f /etc/os-release ]; then echo "--network host"; else echo ""; fi)

# Registry configuration
# For local development with Kind, we use direct image references with no registry prefix
# For production, you can set REGISTRY to your container registry
REGISTRY ?=

# AWS configuration for remote registry (if needed in production)
AWS_REGION := us-west-2
AWS_ACCOUNT_ID=$(shell aws sts get-caller-identity --query "Account" --output text 2>/dev/null || echo "unknown")
ECR_REGISTRY := $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com

# Default cluster name
export KIND_EXPERIMENTAL_PROVIDER=finch
DEV_KIND_CLUSTER ?= jupyter-k8s-dev

# Image versions
TAG ?= latest

# List of all images
IMAGES := jupyter-uv



.PHONY: build-all $(IMAGES)

build-all: $(IMAGES)

# Build individual images
jupyter-uv:
	@echo "Building jupyter-uv image..."
	$(DOCKER) build $(BUILD_OPTS) \
		-t jupyter-uv:$(TAG) \
		-f jupyter-uv/Dockerfile jupyter-uv


# Push all images to registry (when REGISTRY is set)
push-all: build-all
	@if [ -n "$(REGISTRY)" ]; then \
		echo "Pushing images to $(REGISTRY)..."; \
		for img in $(IMAGES); do \
			$(DOCKER) tag $$img:$(TAG) $(REGISTRY)/$$img:$(TAG); \
			$(DOCKER) push $(REGISTRY)/$$img:$(TAG); \
		done; \
	else \
		echo "REGISTRY not set, skipping push"; \
	fi

# Load all images into kind cluster
# CLUSTER_NAME can be overridden from command line
CLUSTER_NAME ?= $(DEV_KIND_CLUSTER)

push-all-kind: build-all
	@echo "Loading images into kind cluster $(CLUSTER_NAME)..."
	@mkdir -p /tmp/kind-images
	for img in $(IMAGES); do \
		echo "Loading $$img:$(TAG) image..."; \
		$(DOCKER) save $$img:$(TAG) -o /tmp/kind-images/$$img.tar; \
		kind load image-archive /tmp/kind-images/$$img.tar --name $(CLUSTER_NAME) || echo "Warning: Failed to load $$img:$(TAG)"; \
		rm -f /tmp/kind-images/$$img.tar; \
	done


# Clean up local images
clean:
	@echo "Cleaning up local images..."
	for img in $(IMAGES); do \
		$(DOCKER) rmi $$img:$(TAG) || true; \
	done

# Show configured registry
info:
	@echo "Registry: $(REGISTRY)"
	@echo "Tag: $(TAG)"
	@echo "Images: $(IMAGES)"