# Build configuration
GO_BINARY := runtime/bin/instrument-hub
INSTALL_PREFIX ?= /opt/falcon
SUDO ?= sudo
PRESET ?= linux-clang-release
VCPKG_TRIPLET ?= x64-linux-dynamic
LOCAL_VCPKG_INSTALLED := $(abspath vcpkg_installed/$(VCPKG_TRIPLET))
LOCAL_PKGCONFIG := $(LOCAL_VCPKG_INSTALLED)/lib/pkgconfig
SCHEMA_BUILD_DIR ?= build/wiremap-validator

# Default target
.PHONY: all
all: build test

# Build targets
.PHONY: build
build: build-go

.PHONY: vcpkg-bootstrap
vcpkg-bootstrap:
	@echo "Bootstrapping vcpkg..."
	MAKELEVEL=0 cmake -P cmake/bootstrap/bootstrap-vcpkg.cmake

.PHONY: configure
configure: vcpkg-bootstrap
	@echo "Configuring $(PRESET)..."
	MAKELEVEL=0 cmake --preset $(PRESET)

.PHONY: install-falcon-deps
install-falcon-deps: vcpkg-bootstrap
	@echo "vcpkg dependencies installed at $(LOCAL_VCPKG_INSTALLED)"

.PHONY: go-mod-prepare
go-mod-prepare:
	cd runtime && go mod tidy

.PHONY: build-go
build-go: install-falcon-deps go-mod-prepare
ifeq ($(OS),Windows_NT)
	cd runtime && go build -o bin/instrument-hub.exe cmd/main.go
	cd runtime && go build -o bin/dataviewer.exe ./cmd/dataviewer/
else
	cd runtime && $(GO_CGO_ENV) LD_LIBRARY_PATH="$(LOCAL_VCPKG_INSTALLED)/lib:$$LD_LIBRARY_PATH" go build -tags cgo,falcon_core -o bin/instrument-hub cmd/main.go
	cd runtime && PKG_CONFIG_PATH="$(LOCAL_PKGCONFIG)" LD_LIBRARY_PATH="$(LOCAL_VCPKG_INSTALLED)/lib:$$LD_LIBRARY_PATH" go build -o bin/dataviewer ./cmd/dataviewer/
endif

# Release build (optimised, symbols stripped)
.PHONY: build-release
build-release: install-falcon-deps go-mod-prepare
	cd runtime && $(GO_CGO_ENV) LD_LIBRARY_PATH="$(LOCAL_VCPKG_INSTALLED)/lib:$$LD_LIBRARY_PATH" \
		go build -tags cgo,falcon_core -ldflags="-s -w" -o bin/instrument-hub cmd/main.go

# Install the instrument-hub binary to INSTALL_PREFIX/bin
.PHONY: install
install: build-go
	$(SUDO) install -d $(INSTALL_PREFIX)/bin
	$(SUDO) install -m 0755 $(GO_BINARY) $(INSTALL_PREFIX)/bin/instrument-hub

# Data viewer — plots raw & averaged measurement data in the browser.
# Usage: make dataviewer DATA_DIR=path/to/measurement/data
DATA_DIR ?= test_data/demo_measurements
.PHONY: dataviewer
dataviewer: build-go
	runtime/bin/dataviewer --data-dir $(DATA_DIR)

# Go unit/integration tests (CGO + falcon_core build tags).
# All required environment variables are derived from LOCAL_VCPKG_INSTALLED.
GO_CGO_ENV = CGO_ENABLED=1 \
	PKG_CONFIG_PATH="$(LOCAL_PKGCONFIG)" \
	CGO_LDFLAGS="-L$(LOCAL_VCPKG_INSTALLED)/lib -Wl,-rpath,$(LOCAL_VCPKG_INSTALLED)/lib"

.PHONY: test-go
test-go: go-mod-prepare build-go
	cd runtime && $(GO_CGO_ENV) go test -tags cgo,falcon_core ./...

.PHONY: test-go-short
test-go-short: go-mod-prepare build-go
	cd runtime && $(GO_CGO_ENV) go test -tags cgo,falcon_core -short ./...

.PHONY: configure-schema
configure-schema: vcpkg-bootstrap
	cmake -S . -B $(SCHEMA_BUILD_DIR) -G Ninja \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_TESTING=ON \
		-DCMAKE_TOOLCHAIN_FILE="$(abspath vcpkg/scripts/buildsystems/vcpkg.cmake)" \
		-DVCPKG_INSTALLED_DIR="$(abspath vcpkg_installed)" \
		-DVCPKG_OVERLAY_TRIPLETS="$(abspath my-vcpkg-triplets)" \
		-DVCPKG_OVERLAY_PORTS="$(abspath ports)" \
		-DVCPKG_TARGET_TRIPLET="$(VCPKG_TRIPLET)" \
		-DCMAKE_PREFIX_PATH="$(LOCAL_VCPKG_INSTALLED)/share;$(abspath vcpkg_installed)"

.PHONY: test-schema
test-schema: configure-schema
	cmake --build $(SCHEMA_BUILD_DIR) --target validate-wiremap-config
	ctest --test-dir $(SCHEMA_BUILD_DIR) --output-on-failure

.PHONY: test
test: test-go test-schema

.PHONY: test-unit test-launch test-integration test-buffered test-linear-integration test-linear-buffered test-2D-buffered test-3D-buffered test-2D-integration test-3D-integration
# Retired names are migration errors, not aliases claiming measurement coverage.
test-unit test-launch test-integration test-buffered test-linear-integration test-linear-buffered test-2D-buffered test-3D-buffered test-2D-integration test-3D-integration:
	@echo "Retired target '$@': use test-go-short or test-go. No measurement-specific suite is selected." >&2
	@exit 2

.PHONY: clean
clean:
	rm -rf .venv
	rm -rf runtime/bin/
	rm -rf *.egg-info
	rm -rf __pycache__
	rm -rf tests/__pycache__
	@echo "Cleaning all build artifacts..."
	rm -rf build vcpkg_installed
	@echo "✓ Clean complete"

# Platform-specific targets
.PHONY: test-linux
test-linux: test

.PHONY: test-windows
test-windows: test-go
