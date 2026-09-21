# Falcon Instrument Hub

Falcon Instrument Hub bridges falcon-core measurement requests to physical
instruments through the instrument-script-server and user-provided measurement
scripts.

## Build

```bash
make build-go
```

The hub binary is written to:

```text
runtime/bin/instrument-hub
```

Install it to `INSTALL_PREFIX`:

```bash
make install
```

## Test

Run the Go tests and schema validator tests:

```bash
make test
```

For routine development without live ISS lifecycle tests, use
`make test-go-short`. Full tests can stop an existing system ISS daemon and
should run only in a dedicated integration environment. Retired Python
measurement-specific targets fail with a migration notice instead of running
the same Go suite under misleading names.

Run only the wiremap schema validator tests:

```bash
make test-schema
```

The schema validator build lives in:

```text
build/wiremap-validator
```

## Wiremap Validation

The hub ships `validate-wiremap-config`, a C++ validator for wiremap YAML files.
It validates `schemas/wiremap.schema.json` and checks for duplicate logical
connection names and duplicate physical instrument endpoints.

Validate the test fixture:

```bash
build/wiremap-validator/validate-wiremap-config \
  test_data/2-dot-1-chargesensor-wiremap.yml
```

See [Configuration Validation](docs/CONFIG_VALIDATION.md) for wiremap format,
validation rules, and installed paths.

## Documentation

- [Hub Overview](docs/index.md)
- [Configuration Validation](docs/CONFIG_VALIDATION.md)
- [Lua Script Authoring](docs/LUA_SCRIPT_AUTHORING.md)
- [Server & Interpreter](docs/server-interpreter.md)
- [NATS Protocol](docs/nats-protocol.md)
- [Data Viewer](docs/data-viewer.md)
- [Developer Cleanup Tracker](docs/DEVELOPER_HYGIENE_CLEANUP.md)

## License

See [LICENSE](LICENSE).
