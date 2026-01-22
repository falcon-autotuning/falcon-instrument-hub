# Quick Start Guide

Get started with the Falcon Instrument Hub Go runtime in 5 minutes.

## Prerequisites

1. **Go 1.19+**
   ```bash
   go version
   ```

2. **instrument-script-server** (from https://github.com/falcon-autotuning/instrument-script-server)
   ```bash
   # Check if installed
   which instrument-server
   
   # Or set custom path
   export INSTRUMENT_SERVER_BINARY=/path/to/instrument-server
   ```

## Build

```bash
# Clone the repository (if not already)
git clone https://github.com/falcon-autotuning/falcon-instrument-hub
cd falcon-instrument-hub

# Build
make build

# Output: bin/falcon-instrument-hub
```

## Run

```bash
# Run with defaults (connects to localhost:8555)
./bin/falcon-instrument-hub

# Or with custom configuration
export INSTRUMENT_SCRIPT_SERVER_RPC_PORT=9000
export INSTRUMENT_SERVER_HOST=my-server.local
./bin/falcon-instrument-hub
```

## Test

```bash
# Run all tests
make test

# Run specific tests
cd runtime
go test -v ./internal/config
go test -v ./internal/compiler
```

## Development Workflow

### 1. Make Changes

Edit files in `runtime/internal/`:

```bash
# Edit handlers
vim runtime/internal/handlers/instrument/handler.go
vim runtime/internal/handlers/measure/handler.go

# Edit compiler
vim runtime/internal/compiler/compiler.go
```

### 2. Format and Check

```bash
make fmt    # Format code
make vet    # Run go vet
make test   # Run tests
```

### 3. Build and Test

```bash
make clean  # Clean old build
make build  # Build new binary
make run    # Run the binary
```

## Project Structure

```
falcon-instrument-hub/
├── runtime/              # Go runtime (NEW)
│   ├── cmd/              # Main entry point
│   ├── internal/         # Internal packages
│   │   ├── config/       # Configuration
│   │   ├── client/       # HTTP client
│   │   ├── compiler/     # Measurement compiler
│   │   └── handlers/     # Instrument & measure handlers
│   ├── README.md         # Architecture docs
│   ├── EXAMPLES.md       # Usage examples
│   └── MIGRATION.md      # Migration guide
├── src/                  # Python code (LEGACY)
├── bin/                  # Executables
├── Makefile              # Build automation
├── go.mod                # Go dependencies
└── README.md             # Project overview
```

## Common Tasks

### Start the Runtime

```bash
# The runtime will:
# 1. Start instrument-script-server daemon
# 2. Listen for instrument/measurement requests
# 3. Handle graceful shutdown on Ctrl+C

./bin/falcon-instrument-hub
```

### Use as a Library

```go
import (
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/config"
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/handlers/instrument"
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/handlers/measure"
)

func main() {
    cfg, _ := config.LoadConfig()
    instrumentHandler := instrument.NewHandler(cfg)
    
    // Use handlers...
}
```

See `runtime/EXAMPLES.md` for complete examples.

### Add Tests

```bash
# Create test file
vim runtime/internal/mypackage/myfile_test.go

# Run tests
cd runtime
go test ./internal/mypackage -v
```

### Update Documentation

```bash
# Update architecture docs
vim runtime/README.md

# Update examples
vim runtime/EXAMPLES.md

# Update migration guide
vim runtime/MIGRATION.md
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `INSTRUMENT_SCRIPT_SERVER_RPC_PORT` | `8555` | instrument-script-server RPC port |
| `INSTRUMENT_SERVER_HOST` | `localhost` | instrument-script-server host |
| `INSTRUMENT_SERVER_BINARY` | `instrument-server` | Path to instrument-server binary |

## Troubleshooting

### Build Fails

```bash
# Update dependencies
go mod tidy
go mod download

# Check Go version
go version  # Should be 1.19+
```

### Tests Fail

```bash
# Run with verbose output
make test

# Check specific package
cd runtime
go test -v ./internal/config
```

### Runtime Issues

```bash
# Check if instrument-script-server is installed
which instrument-server

# Check if port is available
netstat -an | grep 8555

# Check logs
./bin/falcon-instrument-hub 2>&1 | tee runtime.log
```

### instrument-script-server Not Found

```bash
# Option 1: Install to PATH
# Follow https://github.com/falcon-autotuning/instrument-script-server

# Option 2: Set binary path
export INSTRUMENT_SERVER_BINARY=/full/path/to/instrument-server
./bin/falcon-instrument-hub

# Option 3: Add to PATH
export PATH=$PATH:/path/to/instrument-server-dir
./bin/falcon-instrument-hub
```

## Next Steps

1. **Read the docs**:
   - `runtime/README.md` - Architecture overview
   - `runtime/EXAMPLES.md` - Usage examples
   - `runtime/MIGRATION.md` - TODO items

2. **Try examples**:
   - See code examples in `runtime/EXAMPLES.md`
   - Try instrument management
   - Try measurement execution

3. **Integrate falcon-core**:
   - See TODO in `runtime/internal/compiler/compiler.go`
   - Add falcon-core-libs to `go.mod`
   - Implement advanced compilation

4. **Build API server**:
   - See design in `runtime/cmd/main.go`
   - Implement HTTP/gRPC endpoints
   - Add authentication

## Getting Help

- **Documentation**: See `runtime/*.md` files
- **Code Comments**: Look for `TODO` and inline docs
- **Examples**: See `runtime/EXAMPLES.md`
- **Issues**: Check GitHub issues

## Contributing

1. Create feature branch
2. Make changes
3. Add tests
4. Update docs
5. Run `make fmt && make vet && make test`
6. Submit PR

## Resources

- [instrument-script-server](https://github.com/falcon-autotuning/instrument-script-server)
- [falcon-core-libs](https://github.com/falcon-autotuning/falcon-core-libs)
- [Go Documentation](https://go.dev/doc/)

---

**Ready to start?** Run `make build` and explore the examples!
