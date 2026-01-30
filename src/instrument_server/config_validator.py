"""
Configuration validator for instrument hub config.

This module provides validation of the YAML configuration file against the JSON schema.
Works cross-platform on both Linux and Windows.
"""
import json
from pathlib import Path
from typing import Any, Dict, List, Tuple

try:
    import yaml
except ImportError:
    raise ImportError(
        "PyYAML is required for config validation. Install with: pip install pyyaml"
    )

try:
    import jsonschema
    # Try to use the latest validator, but fall back to older versions
    try:
        from jsonschema import Draft202012Validator as SchemaValidator
    except ImportError:
        try:
            from jsonschema import Draft7Validator as SchemaValidator
        except ImportError:
            from jsonschema import Draft4Validator as SchemaValidator
except ImportError:
    raise ImportError(
        "jsonschema is required for config validation. Install with: pip install jsonschema"
    )


class ConfigValidationError(Exception):
    """Raised when configuration validation fails."""
    pass


def load_yaml_config(config_path: Path) -> Dict[str, Any]:
    """
    Load YAML configuration file.
    
    Args:
        config_path: Path to the YAML config file
        
    Returns:
        Dictionary containing the config data
        
    Raises:
        ConfigValidationError: If file cannot be loaded
    """
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            config = yaml.safe_load(f)
        return config
    except FileNotFoundError:
        raise ConfigValidationError(f"Config file not found: {config_path}")
    except yaml.YAMLError as e:
        raise ConfigValidationError(f"Invalid YAML syntax: {e}")
    except Exception as e:
        raise ConfigValidationError(f"Error loading config: {e}")


def load_json_schema(schema_path: Path) -> Dict[str, Any]:
    """
    Load JSON schema file.
    
    Args:
        schema_path: Path to the JSON schema file
        
    Returns:
        Dictionary containing the schema
        
    Raises:
        ConfigValidationError: If schema cannot be loaded
    """
    try:
        with open(schema_path, 'r', encoding='utf-8') as f:
            schema = json.load(f)
        return schema
    except FileNotFoundError:
        raise ConfigValidationError(f"Schema file not found: {schema_path}")
    except json.JSONDecodeError as e:
        raise ConfigValidationError(f"Invalid JSON schema: {e}")
    except Exception as e:
        raise ConfigValidationError(f"Error loading schema: {e}")


def validate_config(
    config: Dict[str, Any], 
    schema: Dict[str, Any]
) -> Tuple[bool, List[str]]:
    """
    Validate configuration against JSON schema.
    
    Args:
        config: Configuration dictionary to validate
        schema: JSON schema dictionary
        
    Returns:
        Tuple of (is_valid, list_of_errors)
    """
    validator = SchemaValidator(schema)
    errors = []
    
    for error in validator.iter_errors(config):
        error_path = ".".join(str(p) for p in error.path) if error.path else "root"
        error_msg = f"  [{error_path}] {error.message}"
        errors.append(error_msg)
    
    return len(errors) == 0, errors


def validate_config_files(
    config_path: Path | str, 
    schema_path: Path | str,
    verbose: bool = True
) -> bool:
    """
    Validate a YAML config file against a JSON schema file.
    
    Args:
        config_path: Path to the YAML configuration file
        schema_path: Path to the JSON schema file
        verbose: If True, print validation results
        
    Returns:
        True if validation passes, False otherwise
        
    Raises:
        ConfigValidationError: If files cannot be loaded or parsed
    """
    # Convert to Path objects for cross-platform compatibility
    config_path = Path(config_path).resolve()
    schema_path = Path(schema_path).resolve()
    
    if verbose:
        print(f"Loading config: {config_path}")
    config = load_yaml_config(config_path)
    
    if verbose:
        print(f"Loading schema: {schema_path}")
    schema = load_json_schema(schema_path)
    
    if verbose:
        print("Validating configuration against schema...")
    
    is_valid, errors = validate_config(config, schema)
    
    if is_valid:
        if verbose:
            print("✓ Configuration is valid!")
        return True
    else:
        if verbose:
            print("✗ Configuration validation failed:")
            for error in errors:
                print(error)
        return False


def get_default_paths() -> Tuple[Path, Path]:
    """
    Get default paths for config and schema files.
    
    Returns:
        Tuple of (config_path, schema_path)
    """
    # Assume we're in the project root or a subdirectory
    current = Path.cwd()
    
    # Try to find project root by looking for pyproject.toml
    root = current
    while root != root.parent:
        if (root / "pyproject.toml").exists():
            break
        root = root.parent
    else:
        # If not found, use current directory
        root = current
    
    config_path = root / "instrument_hub_config.yaml"
    schema_path = root / "config.schema.json"
    
    return config_path, schema_path
