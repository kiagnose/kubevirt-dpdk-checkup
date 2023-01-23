REG ?= quay.io
ORG ?= kiagnose
CHECKUP_IMAGE_NAME ?= kubevirt-dpdk-checkup
CHECKUP_IMAGE_TAG ?= latest
CHECKUP_GIT_TAG ?= $(shell git describe --always --abbrev=8 --tags)
GO_IMAGE_NAME := docker.io/library/golang
GO_IMAGE_TAG := 1.19
BIN_DIR = $(CURDIR)/_output/bin
CRI_BIN ?= $(shell hack/detect_cri.sh)
CRI_BUILD_BASE_IMAGE_TAG ?= latest
LINTER_IMAGE_NAME := docker.io/golangci/golangci-lint
LINTER_IMAGE_TAG := v1.50.1
GO_MOD_VERSION=$(shell hack/go-mod-version.sh)

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
	$(CRI_BIN) build --build-arg BASE_IMAGE_TAG=$(CRI_BUILD_BASE_IMAGE_TAG) . -t $(REG)/$(ORG)/$(CHECKUP_IMAGE_NAME):$(CHECKUP_IMAGE_TAG)
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
	           --volume $(HOME)/.kube:/root/.kube:Z \
	           --workdir $(CURDIR) \
	           -e KUBECONFIG=/root/.kube/config \
	           -e TEST_IMAGE=$(TEST_IMAGE) \
	           -e TEST_NAMESPACE=$(TEST_NAMESPACE) \
	           -e NETWORK_ATTACHMENT_DEFINITION_NAME=$(NETWORK_ATTACHMENT_DEFINITION_NAME) \
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
