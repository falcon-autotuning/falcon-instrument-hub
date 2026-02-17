# Data Viewer

The **FALCon Data Viewer** is a browser-based tool for visualising raw and averaged measurement data stored by the instrument hub. It reads from the [two-database architecture](server-interpreter.md) and renders interactive plots using Plotly.js.

## Quick Start

```bash
# Build and launch (from falcon-instrument-hub/)
make dataviewer

# Or with a custom data directory
make dataviewer DATA_DIR=/path/to/your/measurement/data
```

This builds the `dataviewer` binary and opens a local HTTP server at
[http://localhost:8089](http://localhost:8089). Open that URL in any browser.

### Manual Launch

```bash
# Build
cd runtime
go build -o bin/dataviewer ./cmd/dataviewer/

# Run
./bin/dataviewer --data-dir /path/to/measurement/data --port 8089
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | *(required)* | Path to the measurement data directory (must contain `averaged/` and `raw/` subdirectories) |
| `--port` | `8089` | HTTP port for the viewer |

## Generating Demo Data

If you don't have measurement data yet, generate a demo dataset with two
example measurements (a single Coulomb peak and a double-peak transition):

```bash
cd runtime
go test ./internal/serverinterpreter -run TestGenerate_DummyDataset -v
```

This writes to `test-outs/data/dummy_measurement/` and creates:

| Measurement | Gate | Channel | Voltage Range | Points | Sweeps | Description |
|-------------|------|---------|---------------|--------|--------|-------------|
| `coulomb-peak-001` | P1 | DMM1_0 | −1.5 → 0.5 V | 201 | 10 | Single Lorentzian Coulomb peak |
| `double-peak-002` | B2 | DMM2_0 | −2.0 → 1.0 V | 301 | 8 | Two peaks at different heights |

## User Interface

The viewer has three main areas:

### Sidebar — Measurement List

The left sidebar lists every measurement found in the data directory. Each entry
shows:

- **Measurement ID** — unique identifier
- **AVG** badge — averaged trace is available (always present)
- **RAW** badge — individual sweep traces are available
- **Gate name**, number of data points, and number of sweeps

Click any measurement to load and plot it.

### Toolbar

Two toolbar rows appear above the plot after selecting a measurement:

#### Row 1 — Channel & Display Mode

| Control | Purpose |
|---------|----------|
| **Channel** dropdown | Select which measurement channel to plot (e.g. `DMM1_0`, `DMM2_0`). Multi-channel measurements will show all available channels. |
| **Both / Averaged / Raw Only** toggle | Choose which traces to display. *Both* overlays the averaged trace on top of the raw sweeps. |
| **Raw opacity** | Adjust transparency of raw traces (0.05 → 1.0). Lower values help distinguish the averaged result from the raw noise. |

#### Row 2 — Units & Scaling

| Control | Purpose |
|---------|----------|
| **Y unit** dropdown | Select the display unit for the y-axis: Auto, A, mA, µA, nA, or pA. *Auto* picks the most readable unit based on the data's magnitude. |
| **X scale** | Multiply all x-axis values by an arbitrary factor (default 1). Useful for converting voltage units. |
| **Y scale** | Multiply all y-axis values by an arbitrary factor (default 1), applied *after* unit conversion. |
| **X label** | Override the auto-generated x-axis label with custom text (leave blank for auto). |
| **Y label** | Override the auto-generated y-axis label with custom text (leave blank for auto). |

### Plot Area

The interactive plot supports:

- **Zoom** — click-drag to zoom into a region; double-click to reset
- **Pan** — hold Shift + drag to pan
- **Hover** — move the cursor over the plot to see voltage and current values
  (unified hover shows all traces at the current x-position)
- **Trace toggling** — click a trace name in the legend to hide/show it; double-click to isolate it
- **Export** — use the camera icon in the Plotly toolbar to download a PNG snapshot

#### Colour Coding

| Trace | Colour |
|-------|--------|
| Averaged | **Bold yellow** line (width 2.5) |
| Raw sweeps | Coloured lines from a 10-colour palette, rendered at the configured opacity |

#### Auto-scaling Units

When *Y unit* is set to **Auto**, the y-axis automatically picks the most readable unit:

| Current range | Display unit |
|---------------|-------------|
| < 1 nA | pA |
| 1 nA – 1 µA | nA |
| 1 µA – 1 mA | µA |
| 1 mA – 1 A | mA |
| ≥ 1 A | A |

You can also force a specific unit via the **Y unit** dropdown, or apply an additional multiplicative **Y scale** factor.

## Data Directory Layout

The viewer expects the same on-disk layout produced by `MeasurementDatabase`:

```
<data-dir>/
  averaged/
    index.json                      # measurement index
    sweep_<measurement-id>.json     # averaged results
  raw/
    raw_index.json                  # raw trace index
    raw_<measurement-id>.json       # individual sweep traces
```

See [Server Interpreter — Two-Database Architecture](server-interpreter.md) for
details on how this layout is created during measurements.

## Architecture

The data viewer is a single Go binary with no external dependencies at runtime:

```
cmd/dataviewer/
  main.go                  # HTTP server + JSON API
  frontend/
    index.html             # Plotly.js UI (dark theme)
    plotly.min.js           # Plotly.js (bundled, no CDN needed)
```

- **Backend**: Go HTTP server that reads `index.json` at startup, then serves
  measurement data as JSON via a REST API.
- **Frontend**: Single-page HTML application using Plotly.js for interactive
  plotting. Embedded into the binary via Go's `//go:embed` directive — no
  Node.js, npm, or build step required.

### REST API

| Endpoint | Method | Response |
|----------|--------|----------|
| `/api/measurements` | GET | Array of measurement summaries (id, gate, points, sweeps, has_raw) |
| `/api/measurements/{id}/plot` | GET | Full plot payload: voltage array, averaged channels, raw traces |
| `/` | GET | Frontend HTML (served from embedded filesystem) |

### SSH Tunnelling

Because the viewer is a plain HTTP server, it works seamlessly over SSH tunnels:

```bash
# On remote machine
./dataviewer --data-dir /data/measurements --port 8089

# On your laptop
ssh -L 8089:localhost:8089 user@remote-host
# Open http://localhost:8089 in your local browser
```

This is especially useful when the instrument hub runs on a lab PC without a
display, or in a Docker container.
