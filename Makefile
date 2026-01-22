# Makefile for Falcon Instrument Hub

.PHONY: all build clean test install run help

# Binary name
BINARY_NAME=falcon-instrument-hub
BINARY_PATH=bin/$(BINARY_NAME)

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod

# Build the Go runtime
all: build

build:
	@echo "Building Falcon Instrument Hub Runtime..."
	mkdir -p bin
	cd runtime && $(GOBUILD) -o ../$(BINARY_PATH) ./cmd
	@echo "Build complete: $(BINARY_PATH)"

# Run the runtime (requires instrument-script-server to be installed)
run: build
	@echo "Starting Falcon Instrument Hub Runtime..."
	./$(BINARY_PATH)

# Run tests
test:
	@echo "Running Go tests..."
	cd runtime && $(GOTEST) -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_PATH)

# Update Go dependencies
deps:
	@echo "Updating Go dependencies..."
	$(GOMOD) tidy
	$(GOMOD) download

# Install the binary to $GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME) to $$GOPATH/bin..."
	cp $(BINARY_PATH) $$GOPATH/bin/

# Format Go code
fmt:
	@echo "Formatting Go code..."
	cd runtime && $(GOCMD) fmt ./...

# Run Go vet
vet:
	@echo "Running go vet..."
	cd runtime && $(GOCMD) vet ./...

# Run linters (requires golangci-lint)
lint:
	@echo "Running linters..."
	cd runtime && golangci-lint run

# Help
help:
	@echo "Falcon Instrument Hub - Makefile commands:"
	@echo ""
	@echo "  make build    - Build the Go runtime"
	@echo "  make run      - Build and run the runtime"
	@echo "  make test     - Run tests"
	@echo "  make clean    - Remove build artifacts"
	@echo "  make deps     - Update Go dependencies"
	@echo "  make install  - Install binary to GOPATH/bin"
	@echo "  make fmt      - Format Go code"
	@echo "  make vet      - Run go vet"
	@echo "  make lint     - Run linters (requires golangci-lint)"
	@echo "  make help     - Show this help message"
