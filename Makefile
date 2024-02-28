SHELL := /bin/bash

# The name of the executable (default is current directory name)
TARGET := kube-vip-cloud-provider
.DEFAULT_GOAL: $(TARGET)

# These will be provided to the target
VERSION ?= v0.0.9
BUILD := `git rev-parse HEAD`

# Operating System Default (LINUX)
TARGETOS=linux

# Use linker flags to provide version/build settings to the target
LDFLAGS=-ldflags "-X=main.Version=$(VERSION) -X=main.Build=$(BUILD) -X k8s.io/component-base/version.gitVersion=$(VERSION) -s"

# go source files, ignore vendor directory
SRC = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

DOCKERTAG ?= $(VERSION)
REPOSITORY ?= kubevip

.PHONY: all build clean install uninstall fmt simplify check run

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
	@echo New Multi Architecture Docker image created

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
