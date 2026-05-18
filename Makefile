# Build configuration
GO_BINARY := runtime/bin/instrument-hub
PYTHON_ENV := .venv
NATS_CONTAINER := nats
INSTALL_PREFIX ?= /opt/falcon
SUDO ?= sudo
PRESET ?= linux-clang-release
VCPKG_TRIPLET ?= x64-linux-dynamic
LOCAL_VCPKG_INSTALLED := $(abspath vcpkg_installed/$(VCPKG_TRIPLET))
LOCAL_PKGCONFIG := $(LOCAL_VCPKG_INSTALLED)/lib/pkgconfig

# Default target
.PHONY: all
all: build test

# Build targets
.PHONY: build
build: build-go setup-python

.PHONY: vcpkg-bootstrap
vcpkg-bootstrap:
	@echo "Bootstrapping vcpkg..."
	MAKELEVEL=0 cmake -P cmake/bootstrap/bootstrap-vcpkg.cmake

.PHONY: configure
configure: vcpkg-bootstrap
	@echo "Configuring $(PRESET)..."
	MAKELEVEL=0 cmake --preset $(PRESET)

.PHONY: install-falcon-deps
install-falcon-deps: configure
	@echo "vcpkg dependencies installed at $(LOCAL_VCPKG_INSTALLED)"

.PHONY: build-go
build-go:
ifeq ($(OS),Windows_NT)
	cd runtime && go build -o bin/instrument-hub.exe cmd/main.go
	cd runtime && go build -o bin/dataviewer.exe ./cmd/dataviewer/
else
	cd runtime && PKG_CONFIG_PATH="$(LOCAL_PKGCONFIG)" LD_LIBRARY_PATH="$(LOCAL_VCPKG_INSTALLED)/lib:$$LD_LIBRARY_PATH" go build -o bin/instrument-hub cmd/main.go
	cd runtime && PKG_CONFIG_PATH="$(LOCAL_PKGCONFIG)" LD_LIBRARY_PATH="$(LOCAL_VCPKG_INSTALLED)/lib:$$LD_LIBRARY_PATH" go build -o bin/dataviewer ./cmd/dataviewer/
endif

# Release build (optimised, symbols stripped)
.PHONY: build-release
build-release:
	cd runtime && PKG_CONFIG_PATH="$(LOCAL_PKGCONFIG)" LD_LIBRARY_PATH="$(LOCAL_VCPKG_INSTALLED)/lib:$$LD_LIBRARY_PATH" \
		go build -ldflags="-s -w" -o bin/instrument-hub cmd/main.go

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

.PHONY: setup-python
setup-python:
	uv venv $(PYTHON_ENV)
	uv pip install -e .
	uv pip install -r requirements.txt
	uv pip install -r requirements-test.txt


# Test infrastructure
.PHONY: start-nats
start-nats:
	docker stop $(NATS_CONTAINER) || true
	docker rm $(NATS_CONTAINER) || true
	docker run -d --name $(NATS_CONTAINER) -p 4222:4222 -p 8222:8222 nats --http_port 8222 -js
	sleep 2

.PHONY: stop-nats
stop-nats:
	docker stop $(NATS_CONTAINER) || true
	docker rm $(NATS_CONTAINER) || true

.PHONY: test-unit
test-unit: start-nats setup-python
	$(PYTHON_ENV)/bin/pytest tests/test_daemon_simple.py -v
	$(PYTHON_ENV)/bin/pytest tests/test_signal_handling.py -v
	$(PYTHON_ENV)/bin/pytest tests/test_instrument_daemon_shutdown.py -v
	$(PYTHON_ENV)/bin/pytest tests/test_interpreter_daemon.py -v

.PHONY: test-launch
test-launch: start-nats setup-python
	$(PYTHON_ENV)/bin/pytest tests/test_launch_script_interpreter.py -v
	$(PYTHON_ENV)/bin/pytest tests/test_launch_script_daemon.py -v

.PHONY: test-integration
test-integration: start-nats setup-python build-go
	$(PYTHON_ENV)/bin/pytest tests/integration/ -v -s

.PHONY: test-buffered
test-buffered: start-nats setup-python build-go
	$(PYTHON_ENV)/bin/pytest tests/integration/buffered_two_channel/test_random_data.py -v -s

.PHONY: test-linear-integration
test-linear-integration: start-nats setup-python build-go
	$(PYTHON_ENV)/bin/pytest tests/integration/two_channel_device/test_linear_data.py tests/integration/two_channel_device/test_linear_data_double.py -v -s

.PHONY: test-linear-buffered
test-linear-buffered: start-nats setup-python build-go
	$(PYTHON_ENV)/bin/pytest tests/integration/buffered_two_channel/test_linear_data.py -v -s

.PHONY: test-2D-buffered
test-2D-buffered: start-nats setup-python build-go
	$(PYTHON_ENV)/bin/pytest tests/integration/buffered_two_channel/test_2D_data.py -v -s

.PHONY: test-3D-buffered
test-3D-buffered: start-nats setup-python build-go
	$(PYTHON_ENV)/bin/pytest tests/integration/buffered_two_channel/test_3D_data.py -v -s

.PHONY: test-2D-integration
test-2D-integration: start-nats setup-python build-go
	$(PYTHON_ENV)/bin/pytest tests/integration/two_channel_device/test_2D_data.py tests/integration/two_channel_device/test_2D_data_double.py -v -s

.PHONY: test-3D-integration
test-3D-integration: start-nats setup-python build-go
	$(PYTHON_ENV)/bin/pytest tests/integration/two_channel_device/test_3D_data.py tests/integration/two_channel_device/test_3D_data_double.py -v -s

.PHONY: test
test: test-unit test-launch test-integration

.PHONY: clean
clean: stop-nats
	rm -rf $(PYTHON_ENV)
	rm -rf runtime/bin/
	rm -rf *.egg-info
	rm -rf __pycache__
	rm -rf tests/__pycache__

# Platform-specific targets
.PHONY: test-linux
test-linux: test

.PHONY: test-windows
test-windows: build setup-python
	# Windows-specific testing (run in Windows environment)
	$(PYTHON_ENV)/Scripts/pytest.exe tests/ -v
