# Makefile for building Jupyter images

# Build config
CONTAINER_TOOL ?= finch
BUILD_OPTS :=
CLOUD_PROVIDER :=

# Registry configuration
# For local development with Kind, we use direct image references with no registry prefix
# For production, you can set REGISTRY to your container registry
REGISTRY ?=

# Default cluster name
# Only set KIND_EXPERIMENTAL_PROVIDER when using finch
ifeq ($(CONTAINER_TOOL),finch)
  export KIND_EXPERIMENTAL_PROVIDER=finch
  # Set BUILD_OPTS to '--network host' on cloud desktop (if /etc/os-release exists), otherwise empty
  # You might have to comment BUILD_OPTS out for devdesktop
  BUILD_OPTS := $(shell if [ -f /etc/os-release ]; then echo "--network host"; else echo ""; fi)
endif
DEV_KIND_CLUSTER ?= jupyter-k8s-dev

# Remote cluster configuration
ifeq ($(CLOUD_PROVIDER),aws)
	REMOTE_KUBECONFIG := $(shell pwd)/../remote/.kubeconfig
	AWS_REGION ?= us-west-2
	AWS_ACCOUNT_ID := $(shell aws sts get-caller-identity --query "Account" --output text)
	ECR_REGISTRY := $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com
	EKS_CLUSTER_NAME ?= jupyter-k8s-cluster
endif

# Image versions
TAG ?= latest

# List of all images
APPLICATION_IMAGE_PREFIX := jk8s-application
IMAGES := jupyter-uv



.PHONY: build-all $(IMAGES)

build-all: $(IMAGES)

# Build individual images
jupyter-uv:
	@echo "Building jupyter-uv image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) \
		-t $(APPLICATION_IMAGE_PREFIX)-jupyter-uv:$(TAG) \
		-f jupyter-uv/Dockerfile jupyter-uv


# Push all images to registry (when REGISTRY is set)
push-all: build-all
	@if [ -n "$(REGISTRY)" ]; then \
		echo "Pushing images to $(REGISTRY)..."; \
		for img in $(IMAGES); do \
			$(CONTAINER_TOOL) tag $(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG) $(REGISTRY)/$(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG); \
			$(CONTAINER_TOOL) push $(REGISTRY)/$(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG); \
		done; \
	else \
		echo "REGISTRY not set, skipping push"; \
	fi

# Push all images to AWS ECR
.PHONY: push-all-aws
push-all-aws: build-all
	@if [ "$(CLOUD_PROVIDER)" != "aws" ]; then \
		echo "CLOUD_PROVIDER is not set to aws. Please run with CLOUD_PROVIDER=aws"; \
		exit 1; \
	fi
	@echo "Logging in to ECR..."
	aws ecr get-login-password --region $(AWS_REGION) | $(CONTAINER_TOOL) login --username AWS --password-stdin $(ECR_REGISTRY)
	@echo "Creating ECR repositories if they don't exist..."
	for img in $(IMAGES); do \
		aws ecr describe-repositories --repository-names $(APPLICATION_IMAGE_PREFIX)-$$img --region $(AWS_REGION) > /dev/null || \
		aws ecr create-repository --repository-name $(APPLICATION_IMAGE_PREFIX)-$$img --region $(AWS_REGION); \
	done
	@echo "Building and pushing images to AWS ECR..."
	for img in $(IMAGES); do \
		echo "Pushing $(APPLICATION_IMAGE_PREFIX)-$$img to ECR..."; \
		$(CONTAINER_TOOL) tag $(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG) $(ECR_REGISTRY)/$(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG); \
		$(CONTAINER_TOOL) push $(ECR_REGISTRY)/$(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG); \
	done

# Load all images into kind cluster
# CLUSTER_NAME can be overridden from command line
CLUSTER_NAME ?= $(DEV_KIND_CLUSTER)

push-all-kind: build-all
	@echo "Loading images into kind cluster $(CLUSTER_NAME)..."
	@mkdir -p /tmp/kind-images
	for img in $(IMAGES); do \
		echo "Loading $(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG) image..."; \
		$(CONTAINER_TOOL) save $(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG) > /tmp/kind-images/$$img.tar; \
		kind load image-archive /tmp/kind-images/$$img.tar --name $(CLUSTER_NAME) || echo "Warning: Failed to load $(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG)"; \
		rm -f /tmp/kind-images/$$img.tar; \
	done


# Clean up local images
clean:
	@echo "Cleaning up local images..."
	for img in $(IMAGES); do \
		$(CONTAINER_TOOL) rmi $(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG) || true; \
	done

# Show configured registry
info:
	@echo "Registry: $(REGISTRY)"
	@echo "Tag: $(TAG)"
	@echo "Images: $(IMAGES)"
	@if [ "$(CLOUD_PROVIDER)" = "aws" ]; then \
		echo "\nAWS Configuration:"; \
		echo "  ECR Registry: $(ECR_REGISTRY)"; \
		echo "  AWS Region: $(AWS_REGION)"; \
		echo "  EKS Cluster: $(EKS_CLUSTER_NAME)"; \
	fi

# AWS-specific targets
.PHONY: aws-info
aws-info:
	@if [ "$(CLOUD_PROVIDER)" != "aws" ]; then \
		echo "CLOUD_PROVIDER is not set to aws. Please run with CLOUD_PROVIDER=aws"; \
		exit 1; \
	fi
	@echo "AWS ECR Images:"
	for img in $(IMAGES); do \
		echo "  $(ECR_REGISTRY)/$(APPLICATION_IMAGE_PREFIX)-$$img:$(TAG)"; \
	done