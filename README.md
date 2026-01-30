# Falcon Instrument Hub

A hybrid Python/Go system for managing and controlling scientific instruments in laboratory automation.

## Architecture

This repository contains two implementations:

### 1. Go Runtime (New) - `runtime/`

The modern Go-based runtime that integrates with [instrument-script-server](https://github.com/falcon-autotuning/instrument-script-server) for instrument control and measurement execution. This replaces the previous Python daemon architecture.

**Key Features:**
- HTTP/RPC integration with instrument-script-server
- Native Go measurement compilation (using [falcon-core Go bindings](https://github.com/falcon-autotuning/falcon-core-libs))
- Process-isolated instrument management
- Lua-based measurement scripting

See [runtime/README.md](runtime/README.md) for detailed documentation.

### 2. Python Implementation (Legacy) - `src/`

The original Python-based instrument server using NATS messaging. Retained for backward compatibility during migration.

**Note:** Please use Python 3.13 for the Python implementation.

## Quick Start

### Building the Go Runtime

```bash
make build
```

### Running the Go Runtime

```bash
# Requires instrument-script-server to be installed and in PATH
make run
```

### Configuration Validation

The project includes automated configuration validation to ensure the `instrument_hub_config.yaml` file conforms to the JSON schema defined in `config.schema.json`.

```bash
# Validate the default config file
python3 bin/validate_config.py

# Validate a specific config file
python3 bin/validate_config.py --config path/to/config.yaml --schema path/to/schema.json

# Quiet mode (only returns exit code)
python3 bin/validate_config.py -q
```

The validation script is cross-platform compatible with both Linux and Windows. See [docs/CONFIG_VALIDATION.md](docs/CONFIG_VALIDATION.md) for detailed documentation.

**Exit Codes:**
- `0`: Configuration is valid
- `1`: Configuration is invalid or error occurred

## Prerequisites

### For Go Runtime:
- Go 1.19+
- [instrument-script-server](https://github.com/falcon-autotuning/instrument-script-server) installed and accessible
- (Coming soon) falcon-core Go bindings

### For Python Implementation:
- Python 3.13
- Dependencies from `requirements.txt`

## Project Structure

```
.
├── bin/                      # Executables and scripts
├── runtime/                  # Go runtime implementation
│   ├── cmd/                  # Main entry point
│   └── internal/             # Internal packages
│       ├── config/           # Configuration
│       ├── client/           # HTTP client for instrument-script-server
│       ├── compiler/         # Measurement request compiler
│       └── handlers/         # Instrument and measurement handlers
├── src/                      # Python implementation (legacy)
│   └── instrument_server/    # Python instrument server
├── Makefile                  # Build automation
└── go.mod                    # Go module definition
```

## Migration Status

This project is undergoing a major refactoring:

- ✅ Go project structure created
- ✅ HTTP/RPC client for instrument-script-server
- ✅ Instrument lifecycle management handlers
- ✅ Basic measurement compilation to Lua
- ⏳ Falcon-core Go bindings integration (TODO)
- ⏳ Measurement script selection logic (TODO)
- ⏳ HTTP/gRPC API server (TODO)

## Development

### Building

```bash
make build      # Build the Go runtime
make test       # Run tests
make clean      # Clean build artifacts
```

### Code Formatting

```bash
make fmt        # Format Go code
make vet        # Run go vet
```

## Documentation

- [Go Runtime Documentation](runtime/README.md)
- [instrument-script-server](https://github.com/falcon-autotuning/instrument-script-server)
- [falcon-core Go bindings](https://github.com/falcon-autotuning/falcon-core-libs/tree/main/go/falcon-core)

## License

MIT
