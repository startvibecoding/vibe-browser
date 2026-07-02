.PHONY: build build-compressed build-all install test lint fmt clean run help
.PHONY: build-linux build-darwin build-windows
.PHONY: npm-version npm-packages npm-pack npm-publish-all npm-publish-pre
.PHONY: pypi-version pypi-packages pypi-pack pypi-publish pypi-publish-pre

# Variables
BINARY_NAME=vibe-browser
VERSION=$(shell git describe --tags --abbrev=0 2>/dev/null || echo "0.1.0")
PRE_VERSION=$(if $(filter %-pre,$(VERSION)),$(VERSION),$(VERSION)-pre)
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"
GOBUILD_FLAGS=-trimpath
PYTHON ?= python3

# Python venv for PyPI builds (isolated from system Python)
PYPI_VENV := $(CURDIR)/pypi/.venv-build
PYPI_PYTHON := $(PYPI_VENV)/bin/python

# Create venv and install build deps (idempotent; only runs if venv is missing)
$(PYPI_VENV)/bin/python:
	@echo "Creating PyPI build venv..."
	python3 -m venv $(PYPI_VENV)
	$(PYPI_VENV)/bin/pip install -q --upgrade "setuptools>=77.0.0" build twine
	@echo "PyPI build deps ready: $(PYPI_VENV)"

# UPX compression
USE_UPX ?= true
ifeq ($(shell which upx 2>/dev/null),)
USE_UPX = false
endif
ifeq ($(USE_UPX),true)
UPX_CMD = upx --best
else
UPX_CMD = @true
endif

# Default target
help:
	@echo "vibe-browser Build System"
	@echo ""
	@echo "Build targets:"
	@echo "  build            Build for current platform"
	@echo "  build-compressed Build with UPX compression"
	@echo "  build-linux      Build for Linux (amd64, arm64)"
	@echo "  build-darwin     Build for macOS (amd64, arm64)"
	@echo "  build-windows    Build for Windows (amd64, arm64)"
	@echo "  build-all        Build for all platforms"
	@echo ""
	@echo "NPM targets:"
	@echo "  npm-version       Sync version to npm packages"
	@echo "  npm-packages      Build platform-specific npm packages"
	@echo "  npm-pack          Pack all npm packages"
	@echo "  npm-publish-all   Publish all npm packages"
	@echo "  npm-publish-pre   Publish pre-release packages"
	@echo ""
	@echo "PyPI targets:"
	@echo "  pypi-version      Sync version to PyPI package"
	@echo "  pypi-packages     Build platform-specific PyPI wheels"
	@echo "  pypi-pack         Pack all PyPI wheels"
	@echo "  pypi-publish      Publish PyPI wheels"
	@echo "  pypi-publish-pre  Publish pre-release PyPI wheels"
	@echo ""
	@echo "Other targets:"
	@echo "  install        Install via go install"
	@echo "  test           Run tests"
	@echo "  lint           Run linter"
	@echo "  fmt            Format code"
	@echo "  clean          Remove build artifacts"
	@echo "  run            Build and run"
	@echo "  help           Show this help"

# Build for current platform
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/vibe-browser

# Build with UPX compression
build-compressed: build
	@echo "Compressing binary..."
	$(UPX_CMD) bin/$(BINARY_NAME)

# Platform builds
build-linux:
	@echo "Building for Linux..."
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/vibe-browser
	GOOS=linux GOARCH=arm64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/vibe-browser
	@echo "Compressing Linux amd64 binary..."
	$(UPX_CMD) bin/$(BINARY_NAME)-linux-amd64

build-linux-musl:
	@echo "Building for Linux musl..."
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-musl-amd64 ./cmd/vibe-browser
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-musl-arm64 ./cmd/vibe-browser
	@echo "Compressing Linux musl amd64 binary..."
	$(UPX_CMD) bin/$(BINARY_NAME)-linux-musl-amd64

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p bin
	GOOS=darwin GOARCH=amd64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/vibe-browser
	GOOS=darwin GOARCH=arm64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/vibe-browser

build-windows:
	@echo "Building for Windows..."
	@mkdir -p bin
	GOOS=windows GOARCH=amd64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/vibe-browser
	GOOS=windows GOARCH=arm64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-arm64.exe ./cmd/vibe-browser
	@echo "Compressing Windows amd64 binary..."
	$(UPX_CMD) bin/$(BINARY_NAME)-windows-amd64.exe

# Build all platforms
build-all: build-linux build-linux-musl build-darwin build-windows
	@echo ""
	@echo "Build complete! Binaries in bin/"
	@ls -lh bin/

# Install
install:
	go install $(GOBUILD_FLAGS) $(LDFLAGS) ./cmd/vibe-browser

# Test
test:
	go test ./...

# Test verbose
test-verbose:
	go test -v ./...

# Lint
lint:
	golangci-lint run ./...

# Format
fmt:
	gofmt -w .

# Vet
vet:
	go vet ./...

# Clean
clean:
	rm -rf bin/
	rm -rf pypi/dist pypi/build pypi/*.egg-info $(PYPI_VENV)

# Run
run: build
	./bin/$(BINARY_NAME)

# NPM targets
npm-version:
	./scripts/sync-npm-version.sh $(VERSION)

npm-packages: build-all
	./scripts/build-npm-packages.sh

npm-pack: npm-version npm-packages
	@echo "Packing platform packages..."
	@for d in npm/packages/*/; do \
		if [ -f "$$d/package.json" ]; then \
			echo "  Packing $$(basename $$d)..."; \
			cd "$$d" && npm pack && cd - > /dev/null; \
			mv "$$d"/*.tgz npm/ 2>/dev/null || true; \
		fi; \
	done
	@echo "Packing main package..."
	cd npm && npm pack
	@echo "Done. Tarballs in npm/"

npm-publish-all: npm-version npm-packages
	@echo "Publishing platform packages..."
	@for d in npm/packages/*/; do \
		if [ -f "$$d/package.json" ]; then \
			echo "  Publishing $$(basename $$d)..."; \
			cd "$$d" && npm publish --tag latest --access public && cd - > /dev/null; \
		fi; \
	done
	@echo "Publishing main package..."
	cd npm && npm publish --tag latest --access public
	@echo "Published all packages!"

npm-publish-pre:
	./scripts/sync-npm-version.sh $(PRE_VERSION)
	$(MAKE) npm-packages VERSION=$(PRE_VERSION)
	@echo "Publishing platform packages (pre-release)..."
	@for d in npm/packages/*/; do \
		if [ -f "$$d/package.json" ]; then \
			echo "  Publishing $$(basename $$d)..."; \
			cd "$$d" && npm publish --tag next --access public && cd - > /dev/null; \
		fi; \
	done
	@echo "Publishing main package (pre-release)..."
	cd npm && npm publish --tag next --access public
	@echo "Published all packages (pre-release)!"

# PyPI targets

# Target-specific variable: use the isolated venv for all PyPI operations
pypi-version: PYTHON := $(PYPI_PYTHON)
pypi-packages: PYTHON := $(PYPI_PYTHON)
pypi-pack: PYTHON := $(PYPI_PYTHON)
pypi-publish: PYTHON := $(PYPI_PYTHON)
pypi-publish-pre: PYTHON := $(PYPI_PYTHON)

pypi-version: $(PYPI_VENV)/bin/python
	PYTHON="$(PYTHON)" ./scripts/sync-pypi-version.sh $(VERSION)

pypi-packages: build-all $(PYPI_VENV)/bin/python
	PYTHON="$(PYTHON)" ./scripts/build-pypi-packages.sh

pypi-pack: pypi-version pypi-packages
	@echo "Done. Wheels in pypi/dist/"

pypi-publish: pypi-pack $(PYPI_VENV)/bin/python
	$(PYTHON) -m twine upload pypi/dist/*.whl

pypi-publish-pre:
	PYTHON="$(PYPI_PYTHON)" ./scripts/sync-pypi-version.sh $(PRE_VERSION)
	$(MAKE) pypi-packages VERSION=$(PRE_VERSION) PYTHON=$(PYPI_PYTHON)
	$(PYPI_PYTHON) -m twine upload pypi/dist/*.whl
