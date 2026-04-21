# Falcon Instrument Hub

The falcon-instrument-hub bridges falcon-core measurement requests to hardware instruments through user-provided Lua measurement scripts.
Our [documentation](https://falcon-autotuning.github.io/falcon-instrument-hub/) can bring you up to speed.

## Architecture

**Important**: The hub does NOT auto-generate Lua measurement scripts. Experimenters create their own custom Lua scripts that run on the instrument-script-server. The hub's role is to:

1. **Parse** incoming falcon measurement requests (JSON schemas from falcon-measurement-lib)
2. **Orchestrate** complex measurements by calling simpler Lua scripts multiple times
3. **Buffer** trace data from the instrument-script-server
4. **Average** results when N-averaged measurements are requested
5. **Store** raw and averaged data to HDF5/JSON database
6. **Notify** falcon via NATS/JetStream when data is available

### Example: 2D Voltage Sweep

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
| `sweep_2d.lua` | 2D voltage sweep orchestrating multiple 1D sweeps |
| `ramp_voltage.lua` | Smooth voltage ramping |
| `dc_get_set.lua` | Parallel set/get operations |
| `measure_current.lua` | Current measurement with averaging |

See [docs/LUA_SCRIPT_AUTHORING.md](docs/LUA_SCRIPT_AUTHORING.md) for script requirements.

## Documentation Guide

### Configuration
- **[Device Configuration](docs/CONFIG_VALIDATION.md)** - Configure quantum dot devices and gate mappings
- **[Lua Script Authoring](docs/LUA_SCRIPT_AUTHORING.md)** - Write custom measurement scripts

### Architecture
- **[Server & Interpreter](docs/server-interpreter.md)** - Understand the hub's server architecture
- **[NATS Protocol](docs/nats-protocol.md)** - Communication protocol with falcon-core

<!--toc:start-->

- [Falcon Instrument Hub](#falcon-instrument-hub)
  - [Architecture](#architecture)
  - [Build](#build)
  - [Running](#running)
  - [Configuration](#configuration)
  - [Contributing](#contributing)
  - [License](#license)
<!--toc:end-->

## Build

Given that this application needs to interface directly with real hardware, we
support compiling both into Linux and Windows executables to support most hardware.

### Linux

```bash
# Development build
make build-go

# Release build (optimised, symbols stripped)
make build-release

# Build and install to /opt/falcon/bin (requires sudo)
make install

# Override install prefix
make install INSTALL_PREFIX=$HOME/.local
```

### Windows

```bash
cd runtime
GOOS=windows GOARCH=amd64 go build -o bin/instrument-hub.exe ./cmd/
```

## Running

`instrument-hub` exposes a `start` subcommand that:
1. Starts an **embedded NATS server** (or connects to an external one via `--nats-url`)
2. Auto-starts the **`instrument-script-server` daemon** (unless `--no-iss` is given)
3. Sets up NATS measurement handlers (when `--device-config` and `--wiremap` are provided)

### Quickstart with hub config

```bash
instrument-hub start \
  --hub-config instrument_hub_config.yaml \
  --iss-lib-path /path/to/vcpkg/lib \
  --working-dir /my/data
```

`--hub-config` reads `instrument_hub_config.yaml` and fills in `--device-config`, `--wiremap`, and `--nats-url` automatically.

### All start flags

| Flag | Default | Description |
|------|---------|-------------|
| `--hub-config` | — | Load device-config, wiremap, nats-url from YAML |
| `--device-config` | — | Path to quantum dot device configuration YAML |
| `--wiremap` | — | Path to wiremap YAML |
| `--nats-url` | — | External NATS URL; omit to start embedded NATS |
| `--working-dir` | `.` | Directory for logs, data, and datacache |
| `--packages` | — | Python modules containing instrument templates |
| `--iss-binary` | `/opt/falcon/bin/instrument-script-server` | Path to ISS binary |
| `--iss-lib-path` | — | Path prepended to `LD_LIBRARY_PATH` for ISS |
| `--no-iss` | false | Skip auto-starting ISS daemon |

### Minimal start (NATS + ISS only, no device config)

```bash
instrument-hub start \
  --iss-lib-path /opt/falcon/instrument-script-server/vcpkg_installed/x64-linux-dynamic/lib \
  --working-dir /tmp/hub-run
```

When `--device-config` and `--wiremap` are omitted the hub still starts NATS and ISS, but skips measurement handler registration.

### ISS library path

If `instrument-script-server` was built with vcpkg dynamic libraries, those libraries are not in the system linker path. Pass their location via `--iss-lib-path`:

```bash
--iss-lib-path /path/to/instrument-script-server/vcpkg_installed/x64-linux-dynamic/lib
```

On shutdown (SIGINT / SIGTERM) the hub sends `instrument-script-server daemon stop` before exiting.

## Configuration

### `instrument_hub_config.yaml`

The hub config file maps high-level paths and settings in one place:

```yaml
wiremap: /configs/wiremap.yaml
quantum-dot-config: /configs/qdot.yaml
inst-config: /configs/instruments
nats-url: nats://localhost:4222
instrument-server-port: 5555
local-database: /data
user-measurement-luas: /lua/user
```

Pass it to the hub with `--hub-config`. Any flags given explicitly on the command line take precedence over values from this file.

### CLI flags (legacy / explicit)

The `start` command also accepts the following flags directly:

- `--nats-url`: the URL of the NATS server to connect to.
- `--working-dir`: the directory where the server will store its data and logs.
- `--packages`: Python modules for instrument drivers (comma-separated).
- `--device-config`: path to the quantum dot device configuration YAML.
- `--wiremap`: path to the wiremap YAML.

### Packages

All of the packages need to import `falcon-autotuning/instrument-templates` and follow
the guidelines there for creating instrument driver packages.
If you have multiple packages, simply list them comma separated `--packages pkg1,pkg2,pkg3`

### Device configuration

Right now this configuration file can only be used to configure 1D array style
quantum dot devices with a parallel set of charge sensors across the central
screening gate.

A screening gate is a large gate that is used to isolate the region of the device
from the bulk.
A plunger gate is a gate that is used to control the potential of the quantum dots.
A barrier gate is a gate that is used to control the potential of the barrier
between quantum dots.
A reservoir gate is a large gate that is used to deliver the electrons to the
quantum dots.

There are three regions in the configuration file. Combined together, they represent
the full configuration of the device.

#### Region 1: Global name categorization

````markdown
```yaml
ScreeningGates: S1;S2 ...
PlungerGates: P1;P2 ...
Ohmics: O1;O2 ...
BarrierGates: B1;B2 ...
ReservoirGates: R1;R2 ...
num-unique-channels: 3
```
````

This region of the configuration file defines the gates of each type that
are present in the device.
There are also a list of all possible ohmic connections to the device.
Notice that all the connections are `;` delimited strings.
The `.` is a forbidden character in the names of the gates.

#### Region 2: Specific channel registration

````markdown
```yaml
- groups:
    group1:
      Name: I_O2
      NumDots: 4
      ScreeningGates: S1;S2
      ReservoirGates: R1;R2
      PlungerGates: P1;P2 ...
      BarrierGates: B1;B2 ...
      Order: O1;R1;B1;P1;B2;P2; ... ;R2;O2
    ...
```
````

The second region specifies connections pertaining to an individual readout
channel on the device.
Every group needs to be uniquely identified by a name: `group#` where `#` is a
number.
The order lists all of the gates in the same order that they pattern over the
location of the ideal 1D channel.
The number of groups should be equal to the `num-unique-connections` from earlier.

#### Region 3: DC wiring

````markdown
```yaml
- wiringDC:
    <connection1>:
      resistance: value
      capacitance: value
    ...
```
````

The third region specifies bulk resistance and capacitance for each DC connection
on the device.
They are expected to be in Ohms and Farads respectively.

### Wire-map

A wire-map details the physical connections between the instruments in your
measurement setup. We assume that there are two types of possible connections.
Don't repeat the same connection twice!
This file consists of a list of all the different connection pairs.

To learn about the naming conventions reference `falcon-autotuning/instrument-templates`

#### Direct connection between instruments

````markdown
```yaml
<instrument1>.<index1>: <instrument2>.<index2>
```
````

#### Connection between an instrument and a device

````markdown
```yaml
<instrument1>.<index1>: <device-connection>
```
````

## Docs

See our full documentation at https://falcon-autotuning.github.io/falcon-instrument-hub/.

## Contributing

Contributions are welcome! Please follow the project's coding standards, run `go test ./...` in the `runtime/` directory, and ensure all tests pass before submitting a pull request.

## License

See [LICENSE](LICENSE) for details.
