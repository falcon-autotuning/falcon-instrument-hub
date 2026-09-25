package interpreter

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/databuffer"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
)

// measurementClient represents a single measurement execution context.
//
// A MeasurementClient combines the ability to execute Lua scripts through
// the instrument-script-server and register any DataBuffers returned by
// those scripts. Each MeasurementClient is scoped to a single measurement
// request and is associated with exactly one requestor ID.
//
// The requestor ID is used by the DataBufferManager to track ownership
// of buffers produced during the measurement.
type measurementClient interface {
	Measure(
		scriptPath string,
		variables []instrumentserver.MeasureVariable,
	) ([]instrumentserver.CallResult, error)

	RegisterBuffer(
		bufferID string,
	) error
}

// scriptDispatcher executes a single user measurement.
//
// It is responsible for:
//
//   - Resolving script names to Lua file paths.
//   - Executing the measurement through a MeasurementClient.
//   - Scanning measurement results for DataBuffer references.
//   - Registering discovered buffers with the DataBufferManager.
//
// ScriptDispatcher does not read buffer contents. It only ensures that
// the returned buffers are claimed and tracked.
type scriptDispatcher struct {
	client      measurementClient
	requestorID string
	scriptsPath string
}

func newScriptDispatcher(
	client measurementClient,
	scriptsPath string,
) *scriptDispatcher {
	return &scriptDispatcher{
		client:      client,
		scriptsPath: scriptsPath,
	}
}

// registerBuffers scans a returned ISS value and registers any
// DataBuffer IDs it contains.
//
// Most ISS return types are left untouched. Only DataBuffer and
// DataBufferArray require special handling because they reference
// external shared-memory resources which must be registered with
// the DataBufferManager before the measurement completes.
func (d *scriptDispatcher) registerBuffers(
	value instrumentserver.VariableValue,
) error {
	switch v := value.Value.(type) {

	case instrumentserver.DataBuffer:
		return d.client.RegisterBuffer(string(v))

	case instrumentserver.DataBufferArray:
		for _, bufferID := range v {
			if err := d.client.RegisterBuffer(bufferID); err != nil {
				return err
			}
		}
		return nil

	default:
		return nil
	}
}

// RunMeasurement executes a single Lua measurement script and registers
// all DataBuffers returned by ISS.
//
// Any discovered buffers are associated with the requestor that owns
// this ScriptDispatcher so they can later be enumerated, read, or
// released through the DataBufferManager.
func (d *scriptDispatcher) RunMeasurement(
	scriptName string,
	variables []instrumentserver.MeasureVariable,
) ([]instrumentserver.CallResult, error) {
	scriptPath := filepath.Join(
		d.scriptsPath,
		scriptName+".lua",
	)

	returns, err := d.client.Measure(
		scriptPath,
		variables,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"measure script %s: %w",
			scriptName,
			err,
		)
	}

	for _, call := range returns {
		for _, ret := range call.Return {
			if err := d.registerBuffers(ret.Value); err != nil {
				return nil, err
			}
		}
	}

	return returns, nil
}

// globalMeasurementID generates process-wide unique measurement IDs.
//
// Using a global counter guarantees uniqueness even when multiple
// MeasurementDispatcher instances exist simultaneously.
var (
	globalMeasurementID atomic.Uint64
)

func nextMeasurementID() string {
	id := globalMeasurementID.Add(1)

	return fmt.Sprintf(
		"measurement-%d",
		id,
	)
}

// MeasurementExecutor represents a service capable of executing
// measurement scripts.
//
// This abstraction allows MeasurementDispatcher tests to use a mock
// executor instead of a real ScriptServerClient.
type MeasurementExecutor interface {
	Measure(
		scriptPath string,
		variables []instrumentserver.MeasureVariable,
	) ([]instrumentserver.CallResult, error)
}

var _ MeasurementExecutor = (*instrumentserver.ScriptServerClient)(nil)

// BufferRegistrar tracks ownership of DataBuffers produced by
// measurements.
//
// The primary implementation is DataBufferManager, but the interface
// allows unit tests to substitute a mock implementation.
type BufferRegistrar interface {
	RegisterBuffer(
		requestorID string,
		bufferID string,
	) error
}

var _ BufferRegistrar = (*databuffer.DataBufferManager)(nil)

// MeasurementDispatcher coordinates concurrent measurements.
//
// A dispatcher owns:
//
//   - A shared measurement executor.
//   - A shared buffer manager.
//   - A script search path.
//
// For each measurement request it creates an isolated
// MeasurementClient with a unique requestor ID, launches the
// measurement, and collects the results.
//
// Multiple measurements may execute concurrently.
type MeasurementDispatcher struct {
	executor MeasurementExecutor
	buffers  BufferRegistrar

	scriptsPath string
}

func NewMeasurementDispatcher(
	executor MeasurementExecutor,
	buffers BufferRegistrar,
	scriptsPath string,
) *MeasurementDispatcher {
	return &MeasurementDispatcher{
		executor:    executor,
		buffers:     buffers,
		scriptsPath: scriptsPath,
	}
}

// MeasurementResult contains the outcome of a single dispatched
// measurement execution.
//
// The ID uniquely identifies the measurement and is also the
// requestor ID used for any DataBuffers registered during execution.
type MeasurementResult struct {
	ID      string
	Results []instrumentserver.CallResult
	Err     error
}

func (d *MeasurementDispatcher) registerBuffers(
	requestorID string,
	value instrumentserver.VariableValue,
) error {
	switch v := value.Value.(type) {

	case instrumentserver.DataBuffer:
		return d.buffers.RegisterBuffer(
			requestorID,
			string(v),
		)

	case instrumentserver.DataBufferArray:
		for _, bufferID := range v {
			if err := d.buffers.RegisterBuffer(
				requestorID,
				bufferID,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

// measurementAdapter is a measurement-scoped adapter that binds a
// requestor ID to buffer registration operations.
//
// All measurements share the same executor and buffer manager, but each
// measurement receives its own requestor ID so that buffer ownership
// can be tracked independently.
type measurementAdapter struct {
	executor    MeasurementExecutor
	buffers     BufferRegistrar
	requestorID string
}

func (c *measurementAdapter) Measure(
	scriptPath string,
	variables []instrumentserver.MeasureVariable,
) ([]instrumentserver.CallResult, error) {
	return c.executor.Measure(
		scriptPath,
		variables,
	)
}

// RegisterBuffer registers a buffer on behalf of this measurement.
//
// The caller supplies only the buffer ID. The requestor ID is injected
// automatically by the measurementClient so ownership information
// cannot be forgotten.
func (c *measurementAdapter) RegisterBuffer(
	bufferID string,
) error {
	return c.buffers.RegisterBuffer(
		c.requestorID,
		bufferID,
	)
}

var _ measurementClient = (*measurementAdapter)(nil)

// newClient creates a measurement-scoped client.
//
// The returned client shares the dispatcher's executor and buffer
// manager while binding all buffer registrations to the supplied
// requestor ID.
func (d *MeasurementDispatcher) newClient(
	requestorID string,
) measurementClient {
	return &measurementAdapter{
		executor:    d.executor,
		buffers:     d.buffers,
		requestorID: requestorID,
	}
}

// runMeasurement executes a single measurement request.
//
// A dedicated MeasurementClient is created for the measurement so that
// all produced DataBuffers are tracked under the measurement's unique
// requestor ID.
func (d *MeasurementDispatcher) runMeasurement(
	id string,
	script string,
	vars []instrumentserver.MeasureVariable,
) MeasurementResult {
	client := d.newClient(id)

	dispatcher := newScriptDispatcher(
		client,
		d.scriptsPath,
	)

	results, err := dispatcher.RunMeasurement(
		script,
		vars,
	)
	if err != nil {
		return MeasurementResult{
			ID:  id,
			Err: err,
		}
	}

	return MeasurementResult{
		ID:      id,
		Results: results,
	}
}

// MeasurementRequest describes a single Lua script invocation that
// should be executed by the dispatcher.
type MeasurementRequest struct {
	Script    string
	Variables []instrumentserver.MeasureVariable
}

// RunAll executes multiple measurements concurrently.
//
// Each request receives:
//
//   - A unique measurement ID.
//   - An isolated MeasurementClient.
//   - Independent buffer ownership tracking.
//
// Results are returned in the same order as the supplied requests,
// regardless of completion order.
func (d *MeasurementDispatcher) RunAll(
	requests []MeasurementRequest,
) []MeasurementResult {
	results := make(
		[]MeasurementResult,
		len(requests),
	)

	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)

		go func(
			idx int,
			req MeasurementRequest,
		) {
			defer wg.Done()

			id := nextMeasurementID()

			results[idx] = d.runMeasurement(
				id,
				req.Script,
				req.Variables,
			)
		}(i, req)
	}

	wg.Wait()

	return results
}
