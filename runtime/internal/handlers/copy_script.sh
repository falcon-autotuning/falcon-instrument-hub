#!/bin/bash
set -e

# Create scripts directory if it doesn't exist
mkdir -p scripts

# Copy the launch daemon script and launch interpreter script from the appropriate location
# Adjust this path based on where your launch.py actually lives
cp ../../../../scripts/launch_instrument_daemon.py scripts/launch_instrument_daemon.py || echo "launch_instrument_daemon.py not found in expected location"
cp ../../../../scripts/launch_interpreter.py scripts/launch_interpreter.py || echo "launch_interpreter.py not found in expected location"
