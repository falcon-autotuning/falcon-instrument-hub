# Falcon Instrument Hub

Welcome to the Falcon Instrument Hub documentation! This hub bridges falcon-core measurement requests to hardware instruments through user-provided Lua measurement scripts.

## What is the Falcon Instrument Hub?

The falcon-instrument-hub is a critical orchestration layer in the FALCon measurement framework. It:

- **Parses** incoming falcon measurement requests (JSON schemas from falcon-measurement-lib)
- **Orchestrates** complex measurements by calling simpler Lua scripts multiple times
- **Buffers** trace data from the instrument-script-server
- **Averages** results when N-averaged measurements are requested
- **Stores** raw and averaged data to HDF5/JSON database
- **Notifies** falcon via NATS/JetStream when data is available

## Key Concepts

### Measurement Orchestration

The hub does NOT auto-generate Lua measurement scripts. Instead, experimenters create custom Lua scripts that run on the instrument-script-server. The hub's role is to coordinate these scripts for complex measurement patterns.

**Example: 2D Voltage Sweep**

A 2D voltage sweep from falcon is orchestrated as:

```
For each Y voltage:
  1. hub calls set_voltage.lua(Y_gate, Y_value)
  2. hub calls sweep_1d.lua(X sweep parameters)  
  3. hub calls ramp_voltage.lua(X_gate, X_start)  # Return to start
  4. hub buffers the 1D trace
Aggregate all traces into 2D result
```

### Required Lua Scripts

The following scripts must be provided in `runtime/scripts/`:

| Script | Purpose |
|--------|---------|
| `set_voltage.lua` | Set a single gate voltage |
| `get_voltage.lua` | Read a single voltage |
| `sweep_1d.lua` | 1D voltage sweep with current measurement |
| `ramp_voltage.lua` | Smooth voltage ramping |
| `dc_get_set.lua` | Parallel set/get operations |
| `measure_current.lua` | Current measurement with averaging |

See the [Lua Script Authoring Guide](LUA_SCRIPT_AUTHORING.md) for detailed script requirements.

## Getting Started

### Quick Start

```bash
# Build the hub (requires Go)
make build

# Configure your device and instruments
# Edit instrument_hub_config.yaml with your setup

# Start the hub
./bin/falcon-instrument-hub --config instrument_hub_config.yaml
```

### Documentation Guide

- **[Device Configuration](CONFIG_VALIDATION.md)** - Configure quantum dot devices and gate mappings
- **[Lua Script Authoring](LUA_SCRIPT_AUTHORING.md)** - Write custom measurement scripts
- **[Server & Interpreter](server-interpreter.md)** - Understand the hub's server architecture
- **[NATS Protocol](nats-protocol.md)** - Communication protocol with falcon-core
- **[Data Viewer](data-viewer.md)** - Visualise raw and averaged measurement data in the browser

## Architecture Overview

The hub operates as a daemon process that:

1. **Receives** measurement requests from falcon-core via NATS/JetStream
2. **Translates** high-level measurement specifications into instrument commands
3. **Executes** Lua scripts on the instrument-script-server
4. **Collects** and processes measurement data
5. **Stores** results in structured formats (HDF5/JSON)
6. **Reports** completion back to falcon-core

### Device Configuration

The hub supports 1D array-style quantum dot devices with parallel charge sensors. Configuration includes:

- **Gate Types**: Screening, Plunger, Barrier, Reservoir, and Ohmic gates
- **Channel Groups**: Organize gates into readout channels
- **DC Wiring**: Specify parasitic resistance and capacitance
- **Wire Maps**: Physical connections between instruments and device

See [Device Configuration](CONFIG_VALIDATION.md) for complete details.

## Contributing

Contributions are welcome! Please follow the project's coding standards and testing requirements.

## License

See [LICENSE](../LICENSE.txt) for details.
