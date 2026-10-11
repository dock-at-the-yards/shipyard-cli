BINARY_NAME := shipyard
CMD_DIR := ./cmd/shipyard
BIN_DIR := bin
PKG := github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline

# Platform / Architecture configuration (defaults to host arch/os)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

GO ?= go

.PHONY: all init build install test lint fmt vet clean help \
        version channel install-script dev-build docker-build docker-run \
        docker-shell release-snapshot

all: build

## init: Download dependencies and verify toolchain
init:
	$(GO) mod download
	$(GO) mod verify

# ---------------------------------------------------------------- versioning --

# The version is the single source of truth in the VERSION file. It is the
# version a release from main would publish, and promote.yml reads it to name
# the release PR.
VERSION_FILE := VERSION
VERSION_BASE := $(shell [ -f $(VERSION_FILE) ] && tr -d '[:space:]' < $(VERSION_FILE) || echo 0.0.0)

# Channel is derived from the build version, matching the rule the release
# pipeline uses, so a local build always reports the channel it would be
# published on:
#   1.2.3          -> stable
#   1.2.3-rc.1     -> test      (also -alpha, -beta)
#   0.0.0-dev.42   -> dev
# Anything untagged is dev, so a local binary is never mistaken for a release.
CURRENT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
EXACT_TAG := $(shell git describe --tags --exact-match 2>/dev/null)

ifeq ($(EXACT_TAG),)
  GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
  BUILD_VERSION := $(VERSION_BASE)-dev.$(GIT_SHA)
else
  BUILD_VERSION := $(shell echo $(EXACT_TAG) | sed 's/^v//')
endif

CHANNEL ?= $(shell v='$(BUILD_VERSION)'; if printf '%s' "$$v" | grep -qE -- '-dev\.'; then echo dev; elif printf '%s' "$$v" | grep -qE -- '-(alpha|beta|rc)\.'; then echo test; else echo stable; fi)

COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)

LDFLAGS := -s -w -buildid= \
	-X '$(PKG).Version=$(BUILD_VERSION)' \
	-X '$(PKG).Commit=$(COMMIT)' \
	-X '$(PKG).Channel=$(CHANNEL)'

## version: Print the version, commit, and channel this tree would build
version:
	@echo "version:  $(BUILD_VERSION)"
	@echo "channel:  $(CHANNEL)"
	@echo "commit:   $(COMMIT)"
	@echo "branch:   $(CURRENT_BRANCH)"
	@echo "next rel: $(VERSION_BASE)"

# ------------------------------------------------------------------- build --

## build: Build hardened, stripped binary (removes symbols, DWARF, local paths)
build:
	mkdir -p $(BIN_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)/main.go

# Installation directory: default to GOPATH/bin or /opt/homebrew/bin if in PATH
GOPATH_BIN := $(shell $(GO) env GOBIN)
ifeq ($(GOPATH_BIN),)
GOPATH_BIN := $(shell $(GO) env GOPATH)/bin
endif
INSTALL_DIR ?= $(if $(shell [ -d /opt/homebrew/bin ] && [ -w /opt/homebrew/bin ] && echo 1),/opt/homebrew/bin,$(GOPATH_BIN))

## install: Install shipyard binary to system path (INSTALL_DIR or GOPATH/bin)
install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BIN_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"

## release-snapshot: Reproduce a release tarball locally, exactly as CI would
release-snapshot:
	@mkdir -p dist
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -trimpath -mod=readonly \
		-ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME) $(CMD_DIR)/main.go
	@tar -czf "dist/$(BINARY_NAME)-v$(BUILD_VERSION)-$(GOOS)-$(GOARCH).tar.gz" -C dist $(BINARY_NAME)
	@rm -f dist/$(BINARY_NAME)
	@cd dist && shasum -a 256 *.tar.gz > checksums.txt 2>/dev/null || true
	@echo "Artifacts in dist/ (this is a local preview, not a published release)"

## install-script: Install from the published release, exercising install.sh
install-script:
	@curl -fsSL https://raw.githubusercontent.com/dock-at-the-yards/shipyard-cli/main/install.sh \
		| sh -s -- --channel $(CHANNEL)

# --------------------------------------------------------------------- test --

## test: Run unit and integration tests
test:
	$(GO) test -v ./...

## lint: Run go vet
lint vet:
	$(GO) vet ./...

## fmt: Format Go sources
fmt:
	$(GO) fmt ./...

# ------------------------------------------------------------------- docker --

DOCKER_IMAGE ?= ghcr.io/dock-at-the-yards/shipyard-cli
DOCKER_TAG   ?= $(CHANNEL)

## docker-build: Build the runtime image locally from a locally built binary
docker-build:
	@mkdir -p dist
	GOOS=linux GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags="$(LDFLAGS)" \
		-o dist/shipyard-linux-$(GOARCH) $(CMD_DIR)/main.go
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

## docker-run: Run the image against the current directory
docker-run:
	docker run --rm -it -v "$(PWD):/src" -v /var/run/docker.sock:/var/run/docker.sock \
		$(DOCKER_IMAGE):$(DOCKER_TAG) $(ARGS)

## dev-build: Build everything CI would for a dev build
dev-build: build release-snapshot docker-build
	@echo "dev build complete: $(BIN_DIR)/$(BINARY_NAME), dist/, $(DOCKER_IMAGE):$(DOCKER_TAG)"

# -------------------------------------------------------------------- clean --

## clean: Remove built binaries and artifacts
clean:
	rm -rf $(BIN_DIR) dist

## help: Show available make targets
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  init              Download and verify dependencies"
	@echo "  build             Build bin/shipyard for the host platform"
	@echo "  install           Install to \$$GOPATH/bin or /opt/homebrew/bin"
	@echo "  version           Print version, commit, and channel for this tree"
	@echo "  test              Run the test suite"
	@echo "  lint / vet        Run go vet"
	@echo "  fmt               Format Go sources"
	@echo "  release-snapshot  Reproduce a release tarball locally"
	@echo "  install-script    Install from the published release via install.sh"
	@echo "  docker-build      Build the runtime container image"
	@echo "  docker-run        Run the image against the current directory"
	@echo "  dev-build         build + release-snapshot + docker-build"
	@echo "  clean             Remove build artifacts"
	@echo ""
	@echo "Current: $(BUILD_VERSION) ($(CHANNEL), branch $(CURRENT_BRANCH))"
