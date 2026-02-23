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
  - [Configuration](#configuration)
  - [Contributing](#contributing)
  - [License](#license)
<!--toc:end-->

## Build

Given that this application needs to interface directly with real hardware, we
support compiling both into Linux and Windows executables to support most hardware.

### Linux

```bash
cd runtime
make build
```

### Windows

```bash
cd runtime
# Windows builds use Go cross-compilation
GOOS=windows GOARCH=amd64 go build -o bin/falcon-instrument-hub.exe ./cmd/
```

## Configuration

The go server is designed to accept a few inputs on startup:

- nats-url: the URL of the NATS server to connect to.
- working-dir: the directory where the server will store its data and logs.
- packages: any packages to load to start instrument drivers
- device-config: a path to a yaml file containing the configuration for the
  device(see next section)
- wiremap: a path to a yaml file containing the wiremap for the device(see next section)

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
