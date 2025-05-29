#!/bin/bash

# Bash script to update the API version in the specified files
# run this in the src directory to update to the latest API version
export GOPRIVATE=*/falcon-autotuning
go install github.com/falcon-autotuning/falcon-api/cmd/py-api-loader@dev
go install github.com/falcon-autotuning/falcon-api/cmd/go-api-loader@dev

py-api-loader --repo=server-interpreter --output=./src/server_daemons/api/interpreter.py
go-api-loader --repo=server-interpreter --output=./internal/api/interpreter.py

py-api-loader --repo=instrument-templates --output=./src/server_daemons/api/instrument.py
go-api-loader --repo=instrument-templates --output=./internal/api/instrument.py
