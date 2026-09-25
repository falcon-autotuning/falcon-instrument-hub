// This package assumes that the instrument-script-server was already started via a
// exec for "instrument-script-server daemon start"
package instrumentserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	daemonv1 "github.com/falcon-autotuning/instrument-server/runtime/internal/issproto/instserver/daemon/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	DefaultISSHost      = "127.0.0.1"
	DefaultISSPort      = 8555
	DefaultISSLogLevel  = "info"
	defaultCallTimeout  = 5 * time.Second
	defaultPollInterval = 25 * time.Millisecond
)

// ScriptServerClient is a gRPC client for the instrument-script-server daemon.
type ScriptServerClient struct {
	host      string
	port      int
	issBinary string
	conn      *grpc.ClientConn
	client    daemonv1.DaemonServiceClient
	initErr   error
}

// ScriptServerClientOptions configures optional local-process helpers.
type ScriptServerClientOptions struct {
	ISSBinary string
}

// Used only for tests and injecting mocks into the system
func newScriptServerClientForTests(
	client daemonv1.DaemonServiceClient,
) *ScriptServerClient {
	return &ScriptServerClient{
		client: client,
	}
}

// NewScriptServerClient creates a client for the instrument-script-server gRPC API.
func NewScriptServerClient(host string, port int) *ScriptServerClient {
	return NewScriptServerClientWithOptions(host, port, ScriptServerClientOptions{})
}

// NewScriptServerClientWithOptions creates a client with optional CLI fallback paths.
func NewScriptServerClientWithOptions(host string, port int, opts ScriptServerClientOptions) *ScriptServerClient {
	if host == "" {
		host = DefaultISSHost
	}
	if port == 0 {
		port = DefaultISSPort
	}
	if opts.ISSBinary == "" {
		opts.ISSBinary = "instrument-script-server"
	}

	c := &ScriptServerClient{
		host:      host,
		port:      port,
		issBinary: opts.ISSBinary,
	}

	conn, err := grpc.NewClient(
		fmt.Sprintf("%s:%d", host, port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		c.initErr = err
		return c
	}
	c.conn = conn
	c.client = daemonv1.NewDaemonServiceClient(conn)
	return c
}

// Close releases the underlying gRPC connection.
func (c *ScriptServerClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *ScriptServerClient) callContext(timeout time.Duration) (context.Context, context.CancelFunc, error) {
	if c.initErr != nil {
		return nil, nil, c.initErr
	}
	if c.client == nil {
		return nil, nil, errors.New("ISS gRPC client is not initialized")
	}
	if timeout <= 0 {
		timeout = defaultCallTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return ctx, cancel, nil
}

func standardError(resp *daemonv1.StandardResponse) error {
	if resp == nil {
		return errors.New("missing standard_response")
	}
	if resp.GetOk() {
		return nil
	}
	if err := resp.GetError(); err != nil && err.GetMessage() != "" {
		return errors.New(err.GetMessage())
	}
	if resp.GetMessage() != "" {
		return errors.New(resp.GetMessage())
	}
	return errors.New("ISS request failed")
}

func withCallContext(c *ScriptServerClient, timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel, err := c.callContext(timeout)
	if err != nil {
		return err
	}
	defer cancel()

	return fn(ctx)
}

func withCallContextValue[T any](c *ScriptServerClient, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel, err := c.callContext(timeout)
	if err != nil {
		var zero T
		return zero, err
	}
	defer cancel()

	return fn(ctx)
}

// ListInstruments returns the list of available instruments.
func (c *ScriptServerClient) ListInstruments() ([]string, error) {
	return withCallContextValue(c, defaultCallTimeout, func(ctx context.Context) ([]string, error) {
		resp, err := c.client.ListInstruments(
			ctx,
			&daemonv1.ListInstrumentsRequest{},
		)
		if err != nil {
			return nil, err
		}

		if err := standardError(resp.GetStandardResponse()); err != nil {
			return nil, err
		}

		return append([]string{}, resp.GetInstrumentName()...), nil
	})
}

// StartInstrument sends the StartInstrument RPC to create an instrument.
func (c *ScriptServerClient) StartInstrument(configPath string, pluginPath string) error {
	return withCallContext(c, defaultCallTimeout, func(ctx context.Context) error {
		resp, err := c.client.StartInstrument(
			ctx,
			&daemonv1.StartInstrumentRequest{
				ConfigPath: configPath,
				PluginPath: pluginPath,
				LogLevel:   DefaultISSLogLevel,
			},
		)
		if err != nil {
			return err
		}
		return standardError(resp.GetStandardResponse())
	})
}

// StopInstrument sends the StopInstrument RPC to create an instrument.
func (c *ScriptServerClient) StopInstrument(name string) error {
	return withCallContext(c, defaultCallTimeout, func(ctx context.Context) error {
		resp, err := c.client.StopInstrument(
			ctx,
			&daemonv1.StopInstrumentRequest{
				InstrumentName: name,
			},
		)
		if err != nil {
			return err
		}
		return standardError(resp.GetStandardResponse())
	})
}

// StopDaemon asks the daemon to shut down over gRPC.
func (c *ScriptServerClient) StopDaemon() error {
	return withCallContext(c, defaultCallTimeout, func(ctx context.Context) error {
		resp, err := c.client.StopDaemon(
			ctx,
			&daemonv1.DaemonStop{},
		)
		if err != nil {
			return err
		}
		return standardError(resp)
	})
}

func (c *ScriptServerClient) DaemonStatus() (bool, error) {
	return withCallContextValue(c, defaultCallTimeout, func(ctx context.Context) (bool, error) {
		resp, err := c.client.DaemonStatus(
			ctx,
			&daemonv1.DaemonStatusRequest{},
		)
		if err != nil {
			return false, err
		}

		return resp.Running, standardError(resp.GetStandardResponse())
	})
}

func (c *ScriptServerClient) ReleaseBuffer(bufferID string) error {
	return withCallContext(c, defaultCallTimeout, func(ctx context.Context) error {
		resp, err := c.client.ReleaseBuffer(
			ctx,
			&daemonv1.ReleaseBufferRequest{BufferId: bufferID},
		)
		if err != nil {
			return err
		}

		return standardError(resp.GetStandardResponse())
	})
}

type jobID uint32

func (c *ScriptServerClient) requestMeasurement(req *daemonv1.MeasureJobRequest) (jobID, error) {
	return withCallContextValue(c, defaultCallTimeout, func(ctx context.Context) (jobID, error) {
		resp, err := c.client.MeasureJob(ctx, req)
		if err != nil {
			return 0, err
		}
		return jobID(resp.GetJobId()), standardError(resp.GetStandardResponse())
	})
}

func (c *ScriptServerClient) checkJobStatus(jobID jobID) (daemonv1.JobStatus, error) {
	return withCallContextValue(c, defaultCallTimeout, func(ctx context.Context) (daemonv1.JobStatus, error) {
		resp, err := c.client.JobStatus(
			ctx,
			&daemonv1.JobStatusRequest{JobId: uint32(jobID)},
		)
		return resp.Job.Status, err
	})
}

func (c *ScriptServerClient) collectMeasureJobResult(jobID jobID) (*daemonv1.MeasureJobResultResponse, error) {
	return withCallContextValue(c, defaultCallTimeout, func(ctx context.Context) (*daemonv1.MeasureJobResultResponse, error) {
		return c.client.MeasureJobResult(
			ctx,
			&daemonv1.MeasureJobResultRequest{JobId: uint32(jobID)},
		)
	})
}

// VariableValue represents a dynamically-typed measurement value.
//
// The Value field may contain one of:
//
//	nil
//	int64
//	float64
//	bool
//	string
//	Int64Array
//	DoubleArray
//	BoolArray
//	StringArray
//	DataBufferArray
//	CallStackArray
//	MixedArray
//	MixedMap
//
// Consumers should use a type switch when inspecting the value.
//
// Example:
//
//	switch v := value.Value.(type) {
//	case int64:
//		...
//	case DoubleArray:
//		...
//	case MixedMap:
//		...
//	}
type VariableValue struct {
	Value any
}

type (
	DataBuffer      string
	CallStack       string
	Int64Array      []int64
	DoubleArray     []float64
	BoolArray       []bool
	StringArray     []string
	DataBufferArray []string
	CallStackArray  []string
	// MixedArray      []VariableValue  TODO: Eventually revist ISS if this is necessary
	// MixedMap        map[string]VariableValue  TODO: Eventually revist ISS if this is necessary
)

// Type represents a string taken from the LuaTypes.
type MeasureVariable struct {
	Name  string
	Value VariableValue
}

type DataBufferMetadata struct {
	ElementCount uint32
	Type         LuaType
	Size         int64
}

type ReturnValue struct {
	Name     string
	Value    VariableValue
	Unit     string
	Metadata DataBufferMetadata
}

type CallResult struct {
	Instrument   string
	Channel      int64  // optional argument
	Group        string // optional argument
	Verb         string
	ExecutedAtMs int64
	Return       []ReturnValue
}

type LuaType int32

func (v VariableValue) LuaType() LuaType {
	switch v.Value.(type) {
	case nil:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_UNSPECIFIED)

	case int64:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_INT64)

	case float64:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_DOUBLE)

	case bool:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_BOOL)

	case string:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_STRING)

	case DataBuffer:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_DATA_BUFFER)

	case CallStack:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_CALL_STACK)

	case Int64Array:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_INT64_ARRAY)

	case DoubleArray:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_DOUBLE_ARRAY)

	case BoolArray:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_BOOL_ARRAY)

	case StringArray:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_STRING_ARRAY)

	case DataBufferArray:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_DATA_BUFFER_ARRAY)

	case CallStackArray:
		return LuaType(daemonv1.LuaTypes_LUA_TYPES_CALL_STACK_ARRAY)

	//  TODO: Eventually revist ISS if this is necessary
	// case MixedArray:
	// 	return LuaType(daemonv1.LuaTypes_LUA_TYPES_MIXED_ARRAY)

	default:
		panic(fmt.Sprintf(
			"unsupported VariableValue type %T",
			v.Value,
		))
	}
}

func toGrpcVariableValue(v VariableValue) (*daemonv1.VariableValue, error) {
	switch value := v.Value.(type) {
	case nil:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_IsNil{IsNil: true}}, nil
	case bool:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_B{B: bool(value)}}, nil
	case int:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: int64(value)}}, nil
	case int8:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: int64(value)}}, nil
	case int16:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: int64(value)}}, nil
	case int32:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: int64(value)}}, nil
	case int64:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: value}}, nil
	case uint:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: int64(value)}}, nil
	case uint8:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: int64(value)}}, nil
	case uint16:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: int64(value)}}, nil
	case uint32:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: int64(value)}}, nil
	case uint64:
		const maxInt64 = uint64(1<<63 - 1)
		if value > maxInt64 {
			return nil, fmt.Errorf("uint64 value %d overflows int64", value)
		}
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_I{I: int64(value)}}, nil
	case float32:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_D{D: float64(value)}}, nil
	case float64:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_D{D: value}}, nil
	case string:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_S{S: value}}, nil
	case Int64Array:
		values := make([]int64, len(value))
		for i, item := range value {
			values[i] = int64(item)
		}
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_IArray{IArray: &daemonv1.Int64Array{Values: values}}}, nil
	case DoubleArray:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_DArray{DArray: &daemonv1.DoubleArray{Values: append([]float64{}, value...)}}}, nil
	case StringArray:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_SArray{SArray: &daemonv1.StringArray{Values: append([]string{}, value...)}}}, nil
	case BoolArray:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_BArray{BArray: &daemonv1.BoolArray{Values: append([]bool{}, value...)}}}, nil
	case DataBufferArray:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_DbArray{DbArray: &daemonv1.DataBufferArray{Values: append([]string{}, value...)}}}, nil
	case CallStackArray:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_CsArray{CsArray: &daemonv1.CallStackArray{Values: append([]string{}, value...)}}}, nil
	default:
		return nil, fmt.Errorf("unsupported value type %T", v)
	}
}

func fromGrpcVariableValue(value *daemonv1.VariableValue) VariableValue {
	if value == nil {
		return VariableValue{Value: nil}
	}

	switch v := value.GetValue().(type) {
	case *daemonv1.VariableValue_IsNil:
		return VariableValue{Value: nil}

	case *daemonv1.VariableValue_B:
		return VariableValue{Value: v.B}

	case *daemonv1.VariableValue_I:
		return VariableValue{Value: int64(v.I)}

	case *daemonv1.VariableValue_D:
		return VariableValue{Value: v.D}

	case *daemonv1.VariableValue_S:
		return VariableValue{Value: v.S}

	case *daemonv1.VariableValue_IArray:
		return VariableValue{
			Value: Int64Array(append([]int64{}, v.IArray.GetValues()...)),
		}

	case *daemonv1.VariableValue_DArray:
		return VariableValue{
			Value: DoubleArray(append([]float64{}, v.DArray.GetValues()...)),
		}

	case *daemonv1.VariableValue_BArray:
		return VariableValue{
			Value: BoolArray(append([]bool{}, v.BArray.GetValues()...)),
		}

	case *daemonv1.VariableValue_SArray:
		return VariableValue{
			Value: StringArray(append([]string{}, v.SArray.GetValues()...)),
		}

	case *daemonv1.VariableValue_DbArray:
		return VariableValue{
			Value: DataBufferArray(append([]string{}, v.DbArray.GetValues()...)),
		}

	case *daemonv1.VariableValue_CsArray:
		return VariableValue{
			Value: CallStackArray(append([]string{}, v.CsArray.GetValues()...)),
		}

	default:
		panic(fmt.Sprintf(
			"unsupported grpc VariableValue type %T",
			v,
		))
	}
}

func buildMeasureJobRequest(scriptPath string, variables []MeasureVariable) (*daemonv1.MeasureJobRequest, error) {
	req := &daemonv1.MeasureJobRequest{
		ScriptPath: scriptPath,
		Globals: &daemonv1.Globals{
			Map: map[string]*daemonv1.VariableValue{},
		},
		TypeManifest: &daemonv1.TypeManifest{},
	}
	params := make([]*daemonv1.Parameter, 0, len(variables))
	for _, variable := range variables {
		value, err := toGrpcVariableValue(variable.Value)
		if err != nil {
			return nil, fmt.Errorf("could not extract the value safely for variable %q: %w", variable.Name, err)
		}
		req.Globals.Map[variable.Name] = value
		params = append(params, &daemonv1.Parameter{
			Name: variable.Name,
			Type: daemonv1.LuaTypes(variable.Value.LuaType()),
		})
	}
	req.TypeManifest.Parameters = params
	return req, nil
}

func (c *ScriptServerClient) waitForMeasureJob(jobID jobID) (*daemonv1.MeasureJobResultResponse, error) {
	deadline := time.Now().Add(currentMeasurementTimeout())
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for ISS measure job %d", jobID)
		}
		status, err := c.checkJobStatus(jobID)
		if err != nil {
			return nil, err
		}

		switch status {
		case daemonv1.JobStatus_JOB_STATUS_COMPLETED:
			return c.collectMeasureJobResult(jobID)
		case daemonv1.JobStatus_JOB_STATUS_FAILED,
			daemonv1.JobStatus_JOB_STATUS_CANCELLED,
			daemonv1.JobStatus_JOB_STATUS_CANCELING:
			return nil, fmt.Errorf("ISS measure job %d ended with status %s", jobID, status.String())
		default:
			time.Sleep(defaultPollInterval)
		}
	}
}

func timestampMillis(cmd *daemonv1.CommandResult) int64 {
	ts := cmd.GetExecutedAt()
	if ts == nil {
		return 0
	}
	return ts.AsTime().UnixMilli()
}

func measureJobResultToCallResults(resp *daemonv1.MeasureJobResultResponse) []CallResult {
	var results []CallResult
	for _, cmd := range resp.GetResults() {
		params := cmd.GetParam()
		returnValues := make([]ReturnValue, 0, len(params))
		for _, param := range params {
			unformattedMetadata := param.GetDbmeta()
			// TODO: transfer the Type field if important
			metadata := DataBufferMetadata{
				ElementCount: uint32(unformattedMetadata.GetElementCount()),
				Size:         int64(unformattedMetadata.GetByteSize()),
			}

			returnValues = append(returnValues, ReturnValue{
				Name:     param.GetName(),
				Value:    fromGrpcVariableValue(param.Value),
				Unit:     param.GetUnit(),
				Metadata: metadata,
			})
		}

		results = append(results, CallResult{
			Instrument:   cmd.GetInstrumentName(),
			Channel:      cmd.GetChannel(),
			Group:        cmd.GetGroup(),
			Verb:         cmd.GetVerb(),
			ExecutedAtMs: timestampMillis(cmd),
			Return:       returnValues,
		})
	}
	return results
}

// This keeps track of the queue on the input to the ISS
// TODO: This could be improved if the ISS reported the queue size over Grpc
// but right now this is the only sender so it is okay
const (
	defaultMeasurementTimeout = 5 * time.Minute
)

// We reuse the MEASUREMENT TIMEOUT that the ISS is using
func loadMeasurementTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("MEASUREMENT_TIMEOUT_SEC"))
	if value == "" {
		return defaultMeasurementTimeout
	}

	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return defaultMeasurementTimeout
	}

	return time.Duration(seconds) * time.Second
}

var (
	inflightMeasurements   atomic.Int64
	baseMeasurementTimeout = loadMeasurementTimeout()
)

func currentMeasurementTimeout() time.Duration {
	active := inflightMeasurements.Load()

	if active < 1 {
		active = 1
	}

	return time.Duration(active) * baseMeasurementTimeout
}

// Measure runs a Lua script as an ISS job and returns the parsed call results.
func (c *ScriptServerClient) Measure(scriptPath string, variables []MeasureVariable) ([]CallResult, error) {
	inflightMeasurements.Add(1)
	defer inflightMeasurements.Add(-1)
	req, err := buildMeasureJobRequest(scriptPath, variables)
	if err != nil {
		return nil, err
	}
	jobID, err := c.requestMeasurement(req)
	if err != nil {
		return nil, err
	}

	result, err := c.waitForMeasureJob(jobID)
	if err != nil {
		return nil, err
	}
	return measureJobResultToCallResults(result), nil
}
