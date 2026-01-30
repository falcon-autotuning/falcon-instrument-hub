# Configuration Validation

This directory contains tools for validating the instrument hub configuration against its JSON schema.

## Quick Start

### Option 1: Using the standalone script

```bash
# From project root
python bin/validate_config.py
```

### Option 2: Using Python directly

```python
from pathlib import Path
from instrument_server.config_validator import validate_config_files

# Validate config
is_valid = validate_config_files(
    config_path="instrument_hub_config.yaml",
    schema_path="config.schema.json"
)
```

## Installation

Install the required dependencies:

```bash
pip install -r requirements.txt
```

Or if using the package:

```bash
pip install -e .
```

## Command Line Usage

```bash
# Validate with default paths (looks for files in project root)
python bin/validate_config.py

# Validate with custom paths
python bin/validate_config.py --config /path/to/config.yaml --schema /path/to/schema.json

# Quiet mode (only exit codes)
python bin/validate_config.py --quiet
```

### Exit Codes

- `0`: Validation successful
- `1`: Validation failed or error occurred

## Programmatic Usage

```python
from instrument_server.config_validator import (
    validate_config_files,
    load_yaml_config,
    load_json_schema,
    validate_config,
    ConfigValidationError
)

# Full validation from files
try:
    is_valid = validate_config_files("config.yaml", "schema.json")
    if is_valid:
        print("Config is valid!")
except ConfigValidationError as e:
    print(f"Validation error: {e}")

# Load and validate separately
config = load_yaml_config("config.yaml")
schema = load_json_schema("schema.json")
is_valid, errors = validate_config(config, schema)

if not is_valid:
    for error in errors:
        print(error)
```

## Cross-Platform Compatibility

The validation tools are designed to work on both Linux and Windows:

- Uses `pathlib.Path` for cross-platform path handling
- Handles different path separators automatically
- Works with both Unix and Windows line endings
- No platform-specific dependencies

## Integration with CI/CD

You can integrate validation into your CI/CD pipeline:

```bash
# In your CI script
python bin/validate_config.py || exit 1
```

Or create a pre-commit hook:

```bash
#!/bin/bash
# .git/hooks/pre-commit
python bin/validate_config.py --quiet
if [ $? -ne 0 ]; then
    echo "Config validation failed. Please fix errors before committing."
    exit 1
fi
```

## Features

- ✓ Validates YAML config against JSON Schema (Draft 2020-12)
- ✓ Clear, human-readable error messages
- ✓ Cross-platform (Linux/Windows/macOS)
- ✓ Can be used as library or standalone script
- ✓ Automatic project root detection
- ✓ Detailed validation error reporting
- ✓ Type hints for better IDE support
