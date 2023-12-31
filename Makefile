REG ?= quay.io
ORG ?= kiagnose
CHECKUP_IMAGE_NAME ?= kubevirt-dpdk-checkup
CHECKUP_IMAGE_TAG ?= latest
CHECKUP_GIT_TAG ?= $(shell git describe --always --abbrev=8 --tags)
CHECKUP_BASE_IMAGE_TAG ?= 9.3-1475
VM_IMAGE_BUILDER_IMAGE_NAME := kubevirt-dpdk-checkup-vm-image-builder
VM_IMAGE_BUILDER_IMAGE_TAG ?= latest
VIRT_BUILDER_CACHE_DIR := $(CURDIR)/_virt_builder/cache
VIRT_BUILDER_OUTPUT_DIR := $(CURDIR)/_virt_builder/output
VM_CONTAINER_DISK_IMAGE_NAME := kubevirt-dpdk-checkup-vm
VM_CONTAINER_DISK_IMAGE_TAG ?= latest
TRAFFIC_GEN_CONTAINER_DISK_IMAGE_NAME := kubevirt-dpdk-checkup-traffic-gen
TRAFFIC_GEN_CONTAINER_DISK_IMAGE_TAG ?= latest
GO_IMAGE_NAME := docker.io/library/golang
GO_IMAGE_TAG := 1.20.6
BIN_DIR = $(CURDIR)/_output/bin
CRI_BIN ?= $(shell hack/detect_cri.sh)
LINTER_IMAGE_NAME := docker.io/golangci/golangci-lint
LINTER_IMAGE_TAG := v1.50.1
GO_MOD_VERSION=$(shell hack/go-mod-version.sh)
KUBECONFIG ?= $(HOME)/.kube/config

E2E_TEST_TIMEOUT ?= 1h
E2E_TEST_ARGS ?= $(strip -test.v -test.timeout=$(E2E_TEST_TIMEOUT) -ginkgo.v -ginkgo.timeout=$(E2E_TEST_TIMEOUT) $(E2E_TEST_EXTRA_ARGS))

all: check build

check: lint test/unit

build:
	mkdir -p $(CURDIR)/_go-cache
	$(CRI_BIN) run --rm \
	           --volume $(CURDIR):$(CURDIR):Z \
	           --volume $(CURDIR)/_go-cache:/root/.cache/go-build:Z \
	           --workdir $(CURDIR) \
	           -e GOOS=linux \
	           -e GOARCH=amd64 \
	           $(GO_IMAGE_NAME):$(GO_IMAGE_TAG) go build -v -o $(BIN_DIR)/$(CHECKUP_IMAGE_NAME) ./cmd/
	$(CRI_BIN) build --build-arg BASE_IMAGE_TAG=$(CHECKUP_BASE_IMAGE_TAG) . -t $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_IMAGE_TAG)
.PHONY: build

push:
	$(CRI_BIN) push $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_IMAGE_TAG)
	$(CRI_BIN) tag $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_IMAGE_TAG) $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_GIT_TAG)
	$(CRI_BIN) push $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_GIT_TAG)
.PHONY: push

test/unit:
	mkdir -p $(CURDIR)/_go-cache
	$(CRI_BIN) run --rm \
	           --volume $(CURDIR):$(CURDIR):Z \
	           --volume $(CURDIR)/_go-cache:/root/.cache/go-build:Z \
	           --workdir $(CURDIR) \
	           $(GO_IMAGE_NAME):$(GO_IMAGE_TAG) go test -v ./cmd/... ./pkg/...
.PHONY: test/unit

test/e2e:
	$(CRI_BIN) run --rm \
	           --volume $(CURDIR):$(CURDIR):Z \
	           --volume $(shell dirname $(KUBECONFIG)):/root/.kube:Z,ro \
	           --workdir $(CURDIR) \
	           -e KUBECONFIG=/root/.kube/$(shell basename $(KUBECONFIG)) \
	           -e TEST_CHECKUP_IMAGE=$(TEST_CHECKUP_IMAGE) \
	           -e TEST_NAMESPACE=$(TEST_NAMESPACE) \
	           -e NETWORK_ATTACHMENT_DEFINITION_NAME=$(NETWORK_ATTACHMENT_DEFINITION_NAME) \
	           -e TRAFFIC_GEN_CONTAINER_DISK_IMAGE=$(TRAFFIC_GEN_CONTAINER_DISK_IMAGE) \
	           -e VM_UNDER_TEST_CONTAINER_DISK_IMAGE=$(VM_UNDER_TEST_CONTAINER_DISK_IMAGE) \
	           $(GO_IMAGE_NAME):$(GO_IMAGE_TAG) go test ./tests/... $(E2E_TEST_ARGS)
.PHONY: test/e2e

lint:
	mkdir -p $(CURDIR)/_linter-cache
	$(CRI_BIN) run --rm \
	           --volume $(CURDIR):$(CURDIR):Z \
	           --volume $(CURDIR)/_linter-cache:/root/.cache:Z \
	           --workdir $(CURDIR) \
	            $(LINTER_IMAGE_NAME):$(LINTER_IMAGE_TAG) golangci-lint run --timeout 3m ./cmd/... ./pkg/... ./tests/...
.PHONY: lint

fmt:
	$(CRI_BIN) run --rm \
	           --volume $(CURDIR):$(CURDIR):Z \
	           --workdir $(CURDIR) \
	           $(GO_IMAGE_NAME):$(GO_IMAGE_TAG) gofmt -w ./cmd ./tests
.PHONY: fmt

check-uncommitted:
	./hack/check-uncommitted.sh
.PHONY: check-uncommitted

vendor:
	$(CRI_BIN) run --rm \
	           --volume $(CURDIR):$(CURDIR):Z \
	           --workdir $(CURDIR) \
	           $(GO_IMAGE_NAME):$(GO_IMAGE_TAG) go mod tidy -compat=$(GO_MOD_VERSION) && go mod vendor
.PHONY: vendor

build-vm-image-builder:
	$(CRI_BIN) build $(CURDIR)/vms/image-builder -f $(CURDIR)/vms/image-builder/Dockerfile -t $(REG)/$(ORG)/$(VM_IMAGE_BUILDER_IMAGE_NAME):$(VM_IMAGE_BUILDER_IMAGE_TAG)
.PHONY: build-vm-image-builder

build-vm-image: build-vm-image-builder
	mkdir -vp $(VIRT_BUILDER_CACHE_DIR)
	mkdir -vp $(VIRT_BUILDER_OUTPUT_DIR)

	$(CRI_BIN) container run --rm \
      --volume=$(VIRT_BUILDER_CACHE_DIR):/root/.cache/virt-builder:Z \
      --volume=$(VIRT_BUILDER_OUTPUT_DIR):/output:Z \
      --volume=$(CURDIR)/vms/vm-under-test/scripts:/root/scripts:Z \
      $(REG)/$(ORG)/$(VM_IMAGE_BUILDER_IMAGE_NAME):$(VM_IMAGE_BUILDER_IMAGE_TAG) \
      /root/scripts/build-vm-image
.PHONY: build-vm-image

build-vm-container-disk: build-vm-image
	$(CRI_BIN) build $(CURDIR) -f $(CURDIR)/vms/vm-under-test/Dockerfile -t $(REG)/$(ORG)/$(VM_CONTAINER_DISK_IMAGE_NAME):$(VM_CONTAINER_DISK_IMAGE_TAG)
.PHONY: build-vm-container-disk

push-vm-container-disk:
	$(CRI_BIN) push $(REG)/$(ORG)/$(VM_CONTAINER_DISK_IMAGE_NAME):$(VM_CONTAINER_DISK_IMAGE_TAG)
.PHONY: push-vm-container-disk

build-traffic-gen-vm-image: build-vm-image-builder
	mkdir -vp $(VIRT_BUILDER_CACHE_DIR)
	mkdir -vp $(VIRT_BUILDER_OUTPUT_DIR)

	$(CRI_BIN) container run --rm \
      --volume=$(VIRT_BUILDER_CACHE_DIR):/root/.cache/virt-builder:Z \
      --volume=$(VIRT_BUILDER_OUTPUT_DIR):/output:Z \
      --volume=$(CURDIR)/vms/traffic-gen/scripts:/root/scripts:Z \
      $(REG)/$(ORG)/$(VM_IMAGE_BUILDER_IMAGE_NAME):$(VM_IMAGE_BUILDER_IMAGE_TAG) \
      /root/scripts/build-vm-image
.PHONY: build-traffic-gen-vm-image

build-traffic-gen-container-disk: build-traffic-gen-vm-image
	$(CRI_BIN) build $(CURDIR) -f $(CURDIR)/vms/traffic-gen/Dockerfile -t $(REG)/$(ORG)/$(TRAFFIC_GEN_CONTAINER_DISK_IMAGE_NAME):$(TRAFFIC_GEN_CONTAINER_DISK_IMAGE_TAG)
.PHONY: build-traffic-gen-container-disk

push-traffic-gen-container-disk:
	$(CRI_BIN) push $(REG)/$(ORG)/$(TRAFFIC_GEN_CONTAINER_DISK_IMAGE_NAME):$(TRAFFIC_GEN_CONTAINER_DISK_IMAGE_TAG)
.PHONY: push-traffic-gen-container-disk
