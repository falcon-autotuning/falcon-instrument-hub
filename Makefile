.PHONY: help configure build test clean install vcpkg-bootstrap 

PRESET ?= linux-clang-release
CMAKE_BUILD_DIR := build/$(PRESET)

vcpkg-bootstrap:
	@echo "Bootstrapping vcpkg..."
	cmake -P cmake/bootstrap/bootstrap-vcpkg.cmake


CACHE_FILE := build/$(PRESET)/CMakeCache.txt
ifneq ($(wildcard $(CACHE_FILE)),)
    # Extracts VCPKG_TARGET_TRIPLET from the cache line matching "VCPKG_TARGET_TRIPLET:STRING=..."
    VCPKG_TRIPLET := $(shell grep "^VCPKG_TARGET_TRIPLET:" $(CACHE_FILE) | cut -d'=' -f2)
    # Extracts VCPKG_INSTALLED_DIR from the cache line matching "VCPKG_INSTALLED_DIR:PATH=..."
    VCPKG_INSTALLED_DIR := $(shell grep "^VCPKG_INSTALLED_DIR:" $(CACHE_FILE) | cut -d'=' -f2)
    # Extracts CMAKE_BUILD_TYPE from the cache line matching "CMAKE_BUILD_TYPE:STRING=..."
    CMAKE_BUILD_TYPE:= $(shell grep "^CMAKE_BUILD_TYPE:" $(CACHE_FILE) | cut -d'=' -f2)
else
    CMAKE_BUILD_TYPE := Debug
    VCPKG_TRIPLET := x64-linux-dynamic
    VCPKG_INSTALLED_DIR := $(abspath vcpkg_installed)
endif

LOCAL_VCPKG_INSTALLED := $(VCPKG_INSTALLED_DIR)/$(VCPKG_TRIPLET)
LOCAL_PKGCONFIG := $(LOCAL_VCPKG_INSTALLED)/lib/pkgconfig

# All required environment variables are derived from LOCAL_VCPKG_INSTALLED.
GO_ENV = CGO_ENABLED=1 \
	PKG_CONFIG_PATH="$(LOCAL_PKGCONFIG)" \
	CGO_LDFLAGS="-L$(LOCAL_VCPKG_INSTALLED)/lib -Wl,-rpath,$(LOCAL_VCPKG_INSTALLED)/lib" \
	PATH="$(LOCAL_VCPKG_INSTALLED)/bin:$(PATH)" \
	LD_LIBRARY_PATH="$(LOCAL_VCPKG_INSTALLED)/lib:$(LD_LIBRARY_PATH)"

ISS_PROTO_DIR := \
$(LOCAL_VCPKG_INSTALLED)/share/instrument-script-server/proto

.PHONY: proto
proto:
	rm -rf runtime/proto
	mkdir -p runtime/proto
	cp -R $(ISS_PROTO_DIR)/* runtime/proto/
	cd runtime && buf generate

configure: vcpkg-bootstrap proto
	@echo "Configuring $(PRESET)..."
	cmake --preset $(PRESET)

build: configure
	cd runtime && go mod tidy
ifeq ($(CMAKE_BUILD_TYPE),Debug)
	cd runtime && $(GO_ENV) go build -tags cgo,falcon_core -o bin/instrument-hub ./cmd
else
	cd runtime && $(GO_ENV) go build -tags cgo,falcon_core -ldflags="-s -w" -o bin/instrument-hub ./cmd
endif
	cmake --build --preset $(PRESET) --target validate-wiremap-config

test: build
	@echo "Running tests for $(PRESET)..."
	cd runtime && $(GO_ENV) go test -tags cgo,falcon_core ./...
	ctest --preset $(PRESET) -V

test-cmd: build
	cd runtime && $(GO_ENV) go test \
		-v \
		-coverprofile=coverage.out \
		-tags cgo,falcon_core \
		./cmd

test-instrumentserver: build
	cd runtime && $(GO_ENV) go test \
		-v \
		-coverprofile=coverage.out \
		-tags cgo,falcon_core \
		./internal/instrumentserver

test-databuffer: build
	cd runtime && $(GO_ENV) go test \
		-v \
		-coverprofile=coverage.out \
		-tags cgo,falcon_core \
		./internal/databuffer

test-config: build
	cd runtime && $(GO_ENV) go test \
		-v \
		-coverprofile=coverage.out \
		-tags cgo,falcon_core \
		./internal/config

test-config-handler: build
	cd runtime && $(GO_ENV) go test \
		-v \
		-coverprofile=coverage.out \
		-tags cgo,falcon_core \
		./internal/handlers/device_config

test-interpreter: build
	cd runtime && $(GO_ENV) go test \
		-v \
		-coverprofile=coverage.out \
		-tags cgo,falcon_core \
		./internal/interpreter

test-measure: build
	cd runtime && $(GO_ENV) go test \
		-v \
		-coverprofile=coverage.out \
		-tags cgo,falcon_core \
		./internal/handlers/measure

install: build
	install -m 0755 runtime/bin/instrument-hub $(CMAKE_BUILD_DIR)/instrument-hub
	cmake --install $(CMAKE_BUILD_DIR)

.PHONY: clean
clean:
	rm -rf runtime/bin/
	@echo "Cleaning all build artifacts..."
	rm -rf build vcpkg_installed
	@echo "✓ Clean complete"

# DEPRECATED
# Data viewer — plots raw & averaged measurement data in the browser.
# Usage: make dataviewer DATA_DIR=path/to/measurement/data
DATA_DIR ?= test_data/demo_measurements
.PHONY: dataviewer
dataviewer: build-go
	runtime/bin/dataviewer --data-dir $(DATA_DIR)


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

clean:
	rm -rf .venv
	rm -rf runtime/bin/
	rm -rf *.egg-info
	rm -rf __pycache__
	rm -rf tests/__pycache__
	@echo "Cleaning all build artifacts..."
	rm -rf build vcpkg_installed
	@echo "✓ Clean complete"

test-go-short: go-mod-prepare build
	cd runtime && $(GO_ENV) go test -tags cgo,falcon_core -short ./...
