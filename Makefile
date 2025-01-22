SHELL := /bin/bash

# The name of the executable (default is current directory name)
TARGET := kube-vip-cloud-provider
.DEFAULT_GOAL: $(TARGET)

# These will be provided to the target
VERSION ?= v0.0.11
BUILD := `git rev-parse HEAD`

# Operating System Default (LINUX)
TARGETOS=linux

# Use linker flags to provide version/build settings to the target
LDFLAGS=-ldflags "-X=main.Version=$(VERSION) -X=main.Build=$(BUILD) -X k8s.io/component-base/version.gitVersion=$(VERSION) -s"

# go source files, ignore vendor directory
SRC = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

DOCKERTAG ?= $(VERSION)
REPOSITORY ?= kubevip

KUBE_VIP_CLOUD_PROVIDER_E2E_IMAGE ?= ghcr.io/kube-vip/kube-vip-cloud-provider:main
# Optional variables
# Run specific test specs (matched by regex)
KUBE_VIP_CLOUD_PROVIDER_E2E_TEST_FOCUS ?=
KUBE_VIP_CLOUD_PROVIDER_E2E_PACKAGE_FOCUS ?= ./test/e2e

.PHONY: all build clean install uninstall fmt simplify check run test

all: check install

$(TARGET): $(SRC)
	@echo go build $(LDFLAGS) -o $(TARGET)
	@go build $(LDFLAGS) -o $(TARGET)

build: $(TARGET)
	@true

clean:
	@rm -f $(TARGET)

install:
	@echo Building and Installing project
	@go install $(LDFLAGS)

uninstall: clean
	@rm -f $$(which ${TARGET})

fmt:
	@gofmt -l -w $(SRC)

image-amd64-build-only:
	@docker buildx build  --platform linux/amd64 --build-arg VERSION=$(VERSION) -t $(REPOSITORY)/$(TARGET):$(DOCKERTAG) .

# For faster local builds
image-amd64:
	@docker buildx build  --platform linux/amd64  --build-arg VERSION=$(VERSION) --push -t $(REPOSITORY)/$(TARGET):$(DOCKERTAG) .
	@echo New amd64 Docker image created

image:
	@docker buildx build  --platform linux/amd64,linux/arm64,linux/arm/v7 --build-arg VERSION=$(VERSION) --push -t $(REPOSITORY)/$(TARGET):$(DOCKERTAG) .
	@echo New Multi Architecture Docker image created

simplify:
	@gofmt -s -l -w $(SRC)

check: test
	@test -z $(shell gofmt -l main.go | tee /dev/stderr) || echo "[WARN] Fix formatting issues with 'make fmt'"
	@for d in $$(go list ./... | grep -v /vendor/); do golint $${d}; done
	@go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.56.2 run
	@go vet ./...

run: install
	@$(TARGET)

test:
	go test ./...

.PHONY: setup-kind-cluster
setup-kind-cluster: ## Make a kind cluster for testing
	./test/scripts/make-kind-cluster.sh

## Loads kube-vip-cloud-provider image into kind cluster specified by CLUSTERNAME (default
## kvcp-e2e). By default for local development will build the current
## working kube-vip-cloud-provider source and load into the cluster. If LOAD_PREBUILT_IMAGE
## is specified and set to true, it will load a pre-build image. This requires
## the multiarch-build target to have been run which puts the Kube-vip-cloud-provider docker
## image at <repo>/image/kube-vip-cloud-provider-version.tar.gz. This second option is chosen
## in CI to speed up builds.
.PHONY: load-kvcp-image-kind
load-kvcp-image-kind: ## Load Kube-vip-cloud-provider image from building working source or pre-built image into Kind.
	./test/scripts/kind-load-kvcp-image.sh

.PHONY: cleanup-kind
cleanup-kind:
	./test/scripts/cleanup.sh

.PHONY: e2e
e2e: | setup-kind-cluster load-kvcp-image-kind run-e2e cleanup-kind ## Run E2E tests against a real k8s cluster

.PHONY: run-e2e
run-e2e:
	KUBE_VIP_CLOUD_PROVIDER_E2E_IMAGE=$(KUBE_VIP_CLOUD_PROVIDER_E2E_IMAGE) \
	go run github.com/onsi/ginkgo/v2/ginkgo -tags=e2e -mod=readonly -keep-going -randomize-suites -randomize-all -poll-progress-after=120s --focus '$(KUBE_VIP_CLOUD_PROVIDER_E2E_TEST_FOCUS)' -r $(KUBE_VIP_CLOUD_PROVIDER_E2E_PACKAGE_FOCUS)
