// Package serverinterpreter adapts falcon-core measurement requests and dispatches
// user-provided Lua scripts to instrument-script-server (ISS).
//
// ScriptDispatcher uses ScriptServerClient to submit and poll gRPC measurement
// jobs. Script names select Lua files in the configured measurement directory;
// an optional type manifest describes positional arguments for compiled Teal
// scripts. Buffer references in ISS results are resolved through the ISS CLI.
//
// The handlers package owns script annotation loading and physical-port routing.
// This package does not maintain an independent script or instrument registry.
// FalconMeasurementRequest wraps the falcon-core bindings when built with cgo
// and falcon_core tags. Results are not automatically archived to viewer datasets.
package serverinterpreter
