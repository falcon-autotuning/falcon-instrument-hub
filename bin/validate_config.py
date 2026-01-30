#!/usr/bin/env python3
"""
Standalone script to validate instrument hub configuration.

Usage:
    python validate_config.py
    python validate_config.py --config path/to/config.yaml --schema path/to/schema.json
"""
import argparse
import sys
from pathlib import Path

# Add src to path for imports
src_path = Path(__file__).parent.parent / "src"
sys.path.insert(0, str(src_path))

from instrument_server.config_validator import (
    validate_config_files,
    get_default_paths,
    ConfigValidationError,
)


def main():
    """Main entry point for config validation."""
    parser = argparse.ArgumentParser(
        description="Validate instrument hub YAML configuration against JSON schema",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    
    parser.add_argument(
        "--config",
        type=str,
        help="Path to the YAML configuration file (default: instrument_hub_config.yaml in project root)",
    )
    
    parser.add_argument(
        "--schema",
        type=str,
        help="Path to the JSON schema file (default: config.schema.json in project root)",
    )
    
    parser.add_argument(
        "-q", "--quiet",
        action="store_true",
        help="Suppress output (exit code indicates success/failure)",
    )
    
    args = parser.parse_args()
    
    # Get paths
    if args.config and args.schema:
        config_path = Path(args.config)
        schema_path = Path(args.schema)
    elif args.config or args.schema:
        print("Error: Both --config and --schema must be provided together, or neither")
        sys.exit(1)
    else:
        config_path, schema_path = get_default_paths()
    
    # Validate
    try:
        is_valid = validate_config_files(
            config_path, 
            schema_path, 
            verbose=not args.quiet
        )
        
        if is_valid:
            sys.exit(0)
        else:
            sys.exit(1)
            
    except ConfigValidationError as e:
        if not args.quiet:
            print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        if not args.quiet:
            print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
