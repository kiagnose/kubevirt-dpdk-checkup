REG ?= quay.io
ORG ?= kiagnose
CHECKUP_IMAGE_NAME ?= kubevirt-dpdk-checkup
CHECKUP_IMAGE_TAG ?= latest
CHECKUP_GIT_TAG ?= $(shell git describe --abbrev=8 --tags)
GO_IMAGE_NAME := docker.io/library/golang
GO_IMAGE_TAG := 1.19
BIN_DIR = $(CURDIR)/_output/bin
CRI_BIN ?= $(shell hack/detect_cri.sh)
CRI_BUILD_BASE_IMAGE_TAG ?= latest
LINTER_IMAGE_NAME := docker.io/golangci/golangci-lint
LINTER_IMAGE_TAG := v1.50.1

build:
	$(CRI_BIN) run --rm \
	           --volume `pwd`:$(CURDIR):Z \
	           --workdir $(CURDIR) \
	           --user $(shell id -u):$(shell id -g) \
	           -e XDG_CACHE_HOME=/tmp/.cache \
	           -e GOOS=linux \
	           -e GOARCH=amd64 \
	           $(GO_IMAGE_NAME):$(GO_IMAGE_TAG) go build -v -o $(BIN_DIR)/$(CHECKUP_IMAGE_NAME) ./cmd/
	$(CRI_BIN) build --build-arg BASE_IMAGE_TAG=$(CRI_BUILD_BASE_IMAGE_TAG) . -t $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_IMAGE_TAG)
.PHONY: build

push:
	$(CRI_BIN) push $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_IMAGE_TAG)
	$(CRI_BIN) tag $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_IMAGE_TAG) $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_GIT_TAG)
	$(CRI_BIN) push $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_GIT_TAG)
.PHONY: push

test/unit:
	$(CRI_BIN) run --rm \
	           --volume `pwd`:$(CURDIR):Z \
	           --workdir $(CURDIR) \
	           $(GO_IMAGE_NAME):$(GO_IMAGE_TAG) go test -v ./cmd/...
.PHONY: test/unit

lint:
	$(CRI_BIN) run --rm \
	           --volume `pwd`:$(CURDIR):Z \
	           --workdir $(CURDIR) \
	            $(LINTER_IMAGE_NAME):$(LINTER_IMAGE_TAG) golangci-lint run --timeout 3m ./cmd/...
.PHONY: lint

fmt:
	$(CRI_BIN) run --rm \
	           --volume `pwd`:$(CURDIR):Z \
	           --workdir $(CURDIR) \
	           $(GO_IMAGE_NAME):$(GO_IMAGE_TAG) gofmt -w .
.PHONY: fmt

