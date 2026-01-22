# Migration Guide & TODO Items

This document outlines the migration from the Python-based instrument server to the Go runtime with instrument-script-server integration, and lists items that still need to be completed.

## Migration Overview

### What Was Changed

1. **Instrument Management**
   - **Old**: Python daemon (`bin/launch_instrument_daemon.py`) using NATS messaging
   - **New**: Go runtime with HTTP/RPC client for instrument-script-server
   - **Location**: `runtime/internal/handlers/instrument/`

2. **Measurement Execution**
   - **Old**: Python-based measurement compilation in `/src/server_daemons`
   - **New**: Go-based compiler generating Lua scripts
   - **Location**: `runtime/internal/compiler/` and `runtime/internal/handlers/measure/`

3. **Communication Protocol**
   - **Old**: NATS messaging between Python daemons
   - **New**: HTTP/RPC to instrument-script-server (port 8555)

4. **Instrument Drivers**
   - **Old**: Python drivers loaded as plugins
   - **New**: Instrument-script-server manages drivers via YAML configs

### What Was Retained

- Python codebase in `src/instrument_server/` for backward compatibility
- Existing driver logic (will be migrated to instrument-script-server configs)
- Python dependencies in `requirements.txt` and `pyproject.toml`

## Completed Work

### ✅ Core Infrastructure

1. Go module initialization (`go.mod`)
2. Project structure under `runtime/`
3. Configuration management with environment variable support
4. HTTP/RPC client for instrument-script-server
5. Basic tests for config and compiler
6. Build system (Makefile)
7. Documentation (README, EXAMPLES)

### ✅ Instrument Handler

1. Daemon lifecycle management (start/stop)
2. Instrument start/stop operations
3. Instrument listing
4. Command sending to instruments
5. Proper error handling and logging

### ✅ Measurement Handler

1. Basic measurement request compilation
2. Lua script generation
3. Script execution via instrument-script-server
4. Support for pre-written Lua scripts
5. Temporary script file management

### ✅ Compiler

1. Basic Lua script generation
2. Parameter handling
3. Global variable support
4. Teal-compatible format (main function with ctx parameter)

## TODO Items

### 🔴 High Priority - Blocking Production Use

#### 1. Falcon-Core Go Bindings Integration

**Status**: Not Started  
**Blocker**: External dependency required

**Tasks**:
- [ ] Add falcon-core-libs Go bindings to `go.mod`
- [ ] Update compiler to use falcon-core for measurement pattern generation
- [ ] Implement proper parameter validation using falcon-core
- [ ] Support sweep patterns and quantum measurements
- [ ] Add falcon-core configuration management

**Code Locations**:
- `runtime/internal/compiler/compiler.go` - Replace placeholder with falcon-core calls
- Tests in `runtime/internal/compiler/compiler_test.go`

**References**:
- https://github.com/falcon-autotuning/falcon-core-libs/tree/main/go/falcon-core
- See TODOs in `compiler.go` lines 26-48

#### 2. Measurement Script Selection

**Status**: Not Implemented  
**Blocker**: Requires falcon-core integration

**Problem Statement** (from issue):
> "A part of this compiler that is currently missing is selecting the current measurement_script to then pass to the measure command on the instrument-script-server."

**Tasks**:
- [ ] Implement `SelectMeasurementScript()` function in compiler
- [ ] Create script template repository/registry
- [ ] Add matching logic to select appropriate script for measurement type
- [ ] Support script versioning and caching
- [ ] Integration tests for script selection

**Code Locations**:
- `runtime/internal/compiler/compiler.go:109-126` - Placeholder implementation
- Need to add script template management system

**Implementation Strategy**:
```go
// Pseudocode for future implementation
func (c *Compiler) SelectMeasurementScript(req *MeasurementRequest) (string, error) {
    // 1. Query falcon-core for available measurement patterns
    patterns := falconcore.GetMeasurementPatterns()
    
    // 2. Match request to pattern
    pattern := findBestMatch(req, patterns)
    
    // 3. Load or generate script template
    script := loadScriptTemplate(pattern)
    
    // 4. Customize for instrument capabilities
    customized := customizeForInstrument(script, req.InstrumentName)
    
    // 5. Cache for reuse
    c.cacheScript(req, customized)
    
    return customized, nil
}
```

### 🟡 Medium Priority - Required for Full Functionality

#### 3. API Server Implementation

**Status**: Designed but not implemented

**Tasks**:
- [ ] Add HTTP router (e.g., gorilla/mux or chi)
- [ ] Implement REST API endpoints
- [ ] Add request validation and error handling
- [ ] Implement authentication/authorization
- [ ] Add API documentation (OpenAPI/Swagger)
- [ ] Add rate limiting and request logging

**Endpoints to Implement** (commented in `runtime/cmd/main.go:51-57`):
- `POST /api/instruments/start`
- `POST /api/instruments/:name/stop`
- `GET /api/instruments/list`
- `POST /api/measurements/execute`
- `POST /api/measurements/from-script`
- `POST /api/instruments/:name/command`

#### 4. Integration Tests

**Status**: Unit tests exist, integration tests needed

**Tasks**:
- [ ] Set up test environment with instrument-script-server
- [ ] Add integration tests for instrument lifecycle
- [ ] Add integration tests for measurement execution
- [ ] Add end-to-end workflow tests
- [ ] Mock instrument-script-server for CI/CD
- [ ] Add performance benchmarks

#### 5. Configuration Migration

**Status**: Not started

**Tasks**:
- [ ] Convert Python driver configs to instrument-script-server YAML format
- [ ] Migration script for existing instrument configurations
- [ ] Document configuration format changes
- [ ] Create config validation tool

### 🟢 Low Priority - Nice to Have

#### 6. Advanced Measurement Features

**Tasks**:
- [ ] Batch measurement execution
- [ ] Measurement result caching
- [ ] Real-time measurement streaming
- [ ] Measurement scheduling and queuing
- [ ] Measurement templates library
- [ ] Historical result storage

#### 7. Enhanced Error Handling

**Tasks**:
- [ ] Structured error types
- [ ] Error recovery strategies
- [ ] Detailed error messages with troubleshooting hints
- [ ] Error metrics and monitoring

#### 8. Documentation

**Tasks**:
- [ ] API reference documentation
- [ ] Architecture diagrams
- [ ] Deployment guide
- [ ] Troubleshooting guide
- [ ] Performance tuning guide

#### 9. Observability

**Tasks**:
- [ ] Structured logging (e.g., zap, logrus)
- [ ] Metrics collection (Prometheus)
- [ ] Distributed tracing (OpenTelemetry)
- [ ] Health check endpoints
- [ ] Performance profiling

#### 10. Security

**Tasks**:
- [ ] TLS for API server
- [ ] Authentication (JWT, OAuth)
- [ ] Authorization (RBAC)
- [ ] Secrets management
- [ ] Security audit

## Migration Steps for Users

### For Developers

1. **Install Dependencies**:
   ```bash
   # Install instrument-script-server
   # See: https://github.com/falcon-autotuning/instrument-script-server
   
   # Install Go 1.19+
   go version
   
   # Clone and build
   git clone https://github.com/falcon-autotuning/falcon-instrument-hub
   cd falcon-instrument-hub
   make build
   ```

2. **Update Instrument Configs**:
   - Convert Python driver definitions to instrument-script-server YAML
   - See `runtime/EXAMPLES.md` for config format

3. **Update Measurement Code**:
   - Replace Python measurement calls with Go API calls
   - Or use the HTTP API once implemented

### For Operations

1. **Deploy instrument-script-server**:
   - Install and configure instrument-script-server
   - Ensure it's accessible on the configured port (default 8555)

2. **Deploy falcon-instrument-hub**:
   ```bash
   # Set environment variables
   export INSTRUMENT_SCRIPT_SERVER_RPC_PORT=8555
   export INSTRUMENT_SERVER_HOST=localhost
   
   # Run the runtime
   ./bin/falcon-instrument-hub
   ```

3. **Monitor**:
   - Check logs for any errors
   - Verify instruments are starting correctly
   - Test measurement execution

## Testing Strategy

### Current Tests

- ✅ Configuration loading and validation
- ✅ Compiler basic functionality
- ✅ Lua script generation
- ✅ Parameter formatting

### Needed Tests

- ⏳ Instrument handler with mock server
- ⏳ Measurement handler with mock server
- ⏳ End-to-end workflows
- ⏳ Error scenarios
- ⏳ Performance benchmarks

## Timeline Estimate

### Immediate (1-2 weeks)
- Falcon-core Go bindings integration
- Measurement script selection implementation
- Basic integration tests

### Short-term (1 month)
- API server implementation
- Configuration migration tools
- Comprehensive documentation

### Medium-term (2-3 months)
- Advanced measurement features
- Enhanced observability
- Security hardening

### Long-term (3-6 months)
- Performance optimization
- Production deployment
- Full Python deprecation (if desired)

## Breaking Changes

None yet - Python implementation is still functional.

Future breaking changes will be documented here before implementation.

## Questions & Decisions Needed

1. **Falcon-Core Integration**: 
   - What's the status of falcon-core Go bindings?
   - Are they ready for production use?
   - Do we need to wait for specific features?

2. **API Design**:
   - Should we use REST, gRPC, or both?
   - What authentication mechanism is preferred?
   - Do we need GraphQL support?

3. **Deployment**:
   - Should the runtime be a standalone service or embedded?
   - Container-based deployment preferred?
   - Cloud deployment required?

4. **Python Deprecation**:
   - Timeline for deprecating Python implementation?
   - Migration support needed for existing users?
   - Backward compatibility requirements?

## Getting Help

- Review code comments marked with `TODO`
- Check `runtime/README.md` for architecture details
- See `runtime/EXAMPLES.md` for usage examples
- Open issues for questions or blockers

## Contributing

When working on TODO items:

1. Create a feature branch
2. Update this document to mark items as "In Progress"
3. Add tests for new functionality
4. Update documentation
5. Submit PR with description of changes
6. Update this document to mark items as "Complete"
