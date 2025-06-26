# Build configuration
GO_BINARY := runtime/bin/instrument-server
PYTHON_ENV := .venv
NATS_CONTAINER := nats

# Default target
.PHONY: all
all: build test

# Build targets
.PHONY: build
build: build-go setup-python

.PHONY: build-go
build-go:
	cd runtime && go build -o bin/instrument-server cmd/main.go

.PHONY: setup-python
setup-python:
	uv venv $(PYTHON_ENV)
	uv pip install -e .
	uv pip install -r requirements.txt
	uv pip install -r requirements-test.txt


# Test infrastructure
.PHONY: start-nats
start-nats:
	docker run -d --name $(NATS_CONTAINER) -p 4222:4222 -p 8222:8222 nats --http_port 8222 -js || true
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
	$(PYTHON_ENV)/bin/pytest tests/integration/ -v

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
