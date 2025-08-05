# E2E_K8S_VERSION refers to the kubernetes e2e test binary version downloaded 
# from https://dl.k8s.io/release/
E2E_K8S_VERSION ?= v1.33.1

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
ARTIFACTS ?= ""
GO_VERSION := $(shell awk '/^go /{print $$2}' go.mod|head -n1)
E2E_KIND_NODE_VERSION ?= kindest/node:$(E2E_K8S_VERSION)
E2E_KIND_VERSION ?= v0.27.0
USE_EXISTING_KUBE_CLUSTER ?= false
USE_EXISTING_CUBEFS_CLUSTER ?= false

##@ Build

BASE_IMAGE ?= cubefs/cfs-csi-base:v0.0.1
DOCKER_BUILDX_CMD ?= docker buildx
IMAGE_BUILD_CMD ?= $(DOCKER_BUILDX_CMD) build
IMAGE_BUILD_EXTRA_OPTS ?=
IMAGE_REGISTRY ?= cubefs
IMAGE_NAME ?= cfs-csi-driver
IMAGE_TAG ?= 3.5.1
IMAGE_REPO := $(IMAGE_REGISTRY)/$(IMAGE_NAME)
GOPROXY=${GOPROXY:-""}
IMG ?= $(IMAGE_REPO):$(IMAGE_TAG)
BUILDER_IMAGE ?= golang:$(GO_VERSION)
KIND_CLUSTER_NAME ?= kind
CUBEFS_RELEASE_BRANCH ?= release-$(IMAGE_TAG)

CGO_ENABLED ?= 0
GOOS ?= linux
GOARCH ?= amd64
BUILD_DIR = $(PROJECT_DIR)/build
COMMIT_ID = $(shell git rev-parse --short=8 HEAD)
BRANCH = $(shell git symbolic-ref --short -q HEAD)
BUILD_TIME = $(shell date +%Y-%m-%dT%H:%M)
LDFLAGS = "-s -w -X main.CommitID=${COMMIT_ID} -X main.BuildTime=${BUILD_TIME} -X main.Branch=${BRANCH}"

# TODO: add image-build target to support build binary in dockerfile.
# .PHONY: image-build
# image-build:  ## Build image
# 	$(IMAGE_BUILD_CMD) -t $(IMG) \
# 		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
# 		--build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) \
# 		--build-arg CGO_ENABLED=$(CGO_ENABLED) \
# 		$(IMAGE_BUILD_EXTRA_OPTS) ./build
#
# .PHONY: kind-image-build
# kind-image-build: PLATFORMS=linux/amd64
# kind-image-build: IMAGE_BUILD_EXTRA_OPTS=--load
# kind-image-build: kind image-build

.PHONY: build
build:  ## Build csi driver binary
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-trimpath \
		-gcflags=-trimpath=$(PROJECT_DIR) -asmflags=-trimpath=$(PROJECT_DIR) \
		-ldflags=$(LDFLAGS) \
		-o ${BUILD_DIR}/bin/cfs-csi-driver ./cmd

.PHONY: image
image: cfs-client build ## Build image
	docker build --platform $(GOOS)/$(GOARCH) -t $(IMG) ./build

.PHONY: push
push: image ## Push image
	docker push $(IMG)

.PHONY: clean
clean:  ## Clean build artifacts
	rm -rf build/bin
	rm -rf bin

##@ Development

.PHONY: test-e2e
test-e2e: build image yq kube-e2e-binaries kind ## Run e2e test
	JQ=$(JQ) ARTIFACTS=$(ARTIFACTS) GINKGO=$(GINKGO) CUBEFS_RELEASE_BRANCH=$(CUBEFS_RELEASE_BRANCH) KUBE_E2E_BINARY=$(KUBE_E2E_BINARY) E2E_KIND_NODE_VERSION=$(E2E_KIND_NODE_VERSION) KIND_CLUSTER_NAME=$(KIND_CLUSTER_NAME) KIND=$(KIND) KUBECTL=$(KUBECTL) YQ=$(YQ) USE_EXISTING_KUBE_CLUSTER=$(USE_EXISTING_KUBE_CLUSTER) USE_EXISTING_CUBEFS_CLUSTER=$(USE_EXISTING_CUBEFS_CLUSTER) IMAGE_TAG=$(IMG) ./hack/e2e-test.sh

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= $(LOCALBIN)/kind
KUBE_E2E_BINARY ?= $(LOCALBIN)/e2e.test
GINKGO ?= $(LOCALBIN)/ginkgo
YQ ?= $(LOCALBIN)/yq
JQ ?= $(LOCALBIN)/jq
CFS_CLIENT ?= $(PROJECT_DIR)/build/bin/cfs-client

KIND = $(shell pwd)/bin/kind
.PHONY: kind
kind: $(LOCALBIN) ## Download kind locally if necessary. If wrong version is installed, it will be removed before downloading.
	@if test -x $(LOCALBIN)/kind && ! $(LOCALBIN)/kind version | grep -q $(E2E_KIND_VERSION); then \
		echo "$(LOCALBIN)/kind version is not expected $(E2E_KIND_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kind; \
	fi
	test -s $(LOCALBIN)/kind || GOBIN=$(LOCALBIN) GO111MODULE=on go install sigs.k8s.io/kind@${E2E_KIND_VERSION}

YQ = $(shell pwd)/bin/yq
.PHONY: yq
yq: $(LOCALBIN) ## Download yq locally if necessary.
	test -s $(LOCALBIN)/yq || \
	GOBIN=$(LOCALBIN) go install github.com/mikefarah/yq/v4@v4.26.1

JQ = $(shell pwd)/bin/jq
.PHONY: jq
jq: $(LOCALBIN) ## Download jq locally if necessary.
	test -s $(LOCALBIN)/jq || \
	(curl -LO https://github.com/jqlang/jq/releases/download/jq-1.8.0/jq-$(GOOS)-$(GOARCH) && \
		chmod +x jq-$(GOOS)-$(GOARCH) && \
		mv jq-$(GOOS)-$(GOARCH) $(LOCALBIN)/jq)

KUBE_E2E_BINARY = $(shell pwd)/bin/e2e.test
GINKGO = $(shell pwd)/bin/ginkgo
kube-e2e-binaries: $(LOCALBIN) ## Download kubenetes e2e binary locally if necessary.
	test -s $(LOCALBIN)/e2e.test || \
	(curl -LO https://dl.k8s.io/release/$(E2E_K8S_VERSION)/kubernetes-test-$(GOOS)-$(GOARCH).tar.gz && \
        tar -zxvf kubernetes-test-$(GOOS)-$(GOARCH).tar.gz && \
        chmod +x kubernetes/test/bin/* && \
		mv kubernetes/test/bin/e2e.test $(LOCALBIN)/e2e.test && \
		mv kubernetes/test/bin/ginkgo $(LOCALBIN)/ginkgo && \
		rm -rf kubernetes-test-linux-amd64.tar.gz && \
		rm -rf kubernetes)

CFS_CLIENT = $(shell pwd)/build/bin/cfs-client
cfs-client:  ## Download cubefs client binary locally if necessary.
	mkdir -p $(BUILD_DIR)/bin
	test -s $(BUILD_DIR)/bin/cfs-client || \
	(curl -LO https://github.com/cubefs/cubefs/releases/download/v$(IMAGE_TAG)/cubefs-$(IMAGE_TAG)-$(GOOS)-$(GOARCH).tar.gz && \
        tar -zxvf cubefs-$(IMAGE_TAG)-$(GOOS)-$(GOARCH).tar.gz && \
        chmod +x cubefs/build/bin/cfs-client && \
		mv cubefs/build/bin/cfs-client $(BUILD_DIR)/bin/cfs-client && \
		rm -rf cubefs-$(IMAGE_TAG)-$(GOOS)-$(GOARCH).tar.gz && \
		rm -rf cubefs)
