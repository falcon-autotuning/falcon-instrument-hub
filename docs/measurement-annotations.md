# Measurement script annotations

The hub can read a declarative header from a measurement script at startup.
The script selects instrument commands; the header identifies the API IO type
used to resolve target metadata. Falcon requests still supply physical connections
and a measurement name, without capability names in their request ports.

Place this block before executable code in `get_voltage.tl` or `get_voltage.lua`:

```lua
-- @falcon.metadata
-- schema_version: 1
-- measurement: get_voltage
-- targets:
--   getter:
--     capability: measured_voltage
--     role: input
-- @falcon.end
```

For `set_sample_rate`, declare `getter` with capability `sample_rate` and role
`setting`. A buffered sweep can declare both targets:

```lua
-- @falcon.metadata
-- schema_version: 1
-- measurement: measure_1D_buffered
-- targets:
--   setter:
--     capability: voltage
--     role: output
--   getter:
--     capability: stream
--     role: input
-- @falcon.end
```

`capability` is the exact `channel_groups[].io_types[].name` in an instrument
API, rather than a Lua function name. `role` must match that IO type's role.
`getter` and `setter` refer to the existing request target categories; a setting
can use either category according to its script's current calling convention.
Optional `request_source` must equal its target name; version 1 does not remap
request arguments.

## Validation and loading

The JSON Schema in `schemas/measurement-annotations.schema.json` describes the
YAML object inside the comments. The Go validator in
`runtime/internal/scriptmetadata` checks its structure without evaluating Lua.
The hub also checks capability/role pairs against loaded instrument APIs.

- Only one block is allowed in the leading line-comment header.
- `schema_version` must be `1`; measurement must match the script basename.
- At least one target is required; only `getter` and `setter` are supported.
- Capabilities use identifiers; roles are `input`, `output`, or `setting`.
- Unknown fields, duplicate YAML keys, and multiple YAML documents are errors.
- Matching annotated `.tl`/`.lua` companions are accepted; conflicting copies
  are errors. If compilation drops comments, deploy the annotated `.tl` beside
  the generated `.lua`. ISS execution still requires the `.lua` file.
- Invalid annotations prevent operational handler startup.

The hub scans the immediate `--user-measurement-luas` directory once at startup.
The directory must be readable. Unannotated scripts retain existing defaults
and optional `measurement-metadata.yml` definitions. Annotated targets replace
the target definitions for that measurement, taking precedence over both.
Changes to annotations require a hub restart.

Startup validation checks that some loaded API contains the declared IO type and
role. Request-time resolution then checks that the requested physical connection
has exactly one matching connected port. This avoids requiring a reusable script
to name a specific instrument instance in its header.

Units come from the selected API IO type. `instrument.instrument_type` is the
explicit canonical falcon-core type for every IO type exposed by that API, such
as `dc_voltage_source` or `voltmeter`. The hub does not infer it from a
protocol, role, unit, or IO name.

Version 1 covers target metadata. It does not add response-routing annotations,
change the existing measurement argument dispatch, or verify that script commands
actually agree with the declared capability. Transformed results, such as a script
converting voltage to resistance, need a future result-metadata contract.

Run the standalone parser tests from `runtime`:

```sh
go test ./internal/scriptmetadata
```
