# Configuration Validation

The hub ships a C++ wiremap validator for checking wiremap YAML before startup.
It validates the file against `schemas/wiremap.schema.json` and performs a
small semantic pass for duplicate mappings.

## Quick Start

From the repository root:

```bash
make test-schema
```

This configures a local CMake build in `build/wiremap-validator`, builds
`validate-wiremap-config`, and runs the validator test suite.

`make test` runs the existing Go tests plus these schema tests.

## Manual Usage

Build the validator:

```bash
make test-schema
```

Validate a wiremap directly:

```bash
build/wiremap-validator/validate-wiremap-config \
  test_data/2-dot-1-chargesensor-wiremap.yml
```

Successful validation prints:

```text
Validation succeeded.
```

Validation failures return exit code `2` and print one or more errors:

```text
Validation failed:
  - /wiremap/1/instrument: duplicate physical instrument endpoint 'Source1.analog.1'; first used at /wiremap/0
```

## Wiremap Format

Wiremaps use a top-level `wiremap` array. Each entry maps one logical device
connection to one instrument channel endpoint:

```yaml
wiremap:
  - name: P1
    instrument:
      name: Source1
      channel_name: analog
      index: 4
```

The hub converts each entry into an internal lookup key:

```text
Source1.analog.4 -> P1
```

## Validation Rules

The JSON schema checks:

- top-level document is an object with only `wiremap`
- `wiremap` is a non-empty array
- every entry has `name` and `instrument`
- every `instrument` has `name`, `channel_name`, and one-based integer `index`
- identifiers start with a letter and contain only letters, digits, underscores,
  or hyphens
- extra fields are rejected

The C++ semantic pass also checks:

- no duplicate logical connection names
- no duplicate physical endpoints

## Test Fixtures

The schema tests include the real fixture:

```text
test_data/2-dot-1-chargesensor-wiremap.yml
```

Invalid fixtures live under:

```text
tests/wiremap/
```

They cover missing required fields, bad names, unknown fields, duplicate logical
names, and duplicate physical endpoints.

## Installed Layout

When installed through CMake or the hub package, the validator and schema are
installed as:

```text
bin/validate-wiremap-config
share/falcon-instrument-hub/schemas/wiremap.schema.json
```
