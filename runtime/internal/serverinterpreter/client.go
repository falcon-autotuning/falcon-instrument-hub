// Package serverinterpreter provides the gRPC client for instrument-script-server.
package serverinterpreter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	daemonv1 "github.com/falcon-autotuning/instrument-server/runtime/internal/issproto/instserver/daemon/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultISSHost      = "127.0.0.1"
	defaultISSPort      = 8555
	defaultISSLogLevel  = "info"
	defaultCallTimeout  = 30 * time.Second
	defaultPollInterval = 25 * time.Millisecond
)

// ScriptServerClient is a gRPC client for the instrument-script-server daemon.
type ScriptServerClient struct {
	host       string
	port       int
	issBinary  string
	issLibPath string
	conn       *grpc.ClientConn
	client     daemonv1.DaemonServiceClient
	initErr    error
}

// ScriptServerClientOptions configures optional local-process helpers.
type ScriptServerClientOptions struct {
	ISSBinary  string
	ISSLibPath string
}

// NewScriptServerClient creates a client for the instrument-script-server gRPC API.
func NewScriptServerClient(host string, port int) *ScriptServerClient {
	return NewScriptServerClientWithOptions(host, port, ScriptServerClientOptions{})
}

// NewScriptServerClientWithOptions creates a client with optional CLI fallback paths.
func NewScriptServerClientWithOptions(host string, port int, opts ScriptServerClientOptions) *ScriptServerClient {
	if host == "" {
		host = defaultISSHost
	}
	if port == 0 {
		port = defaultISSPort
	}
	if opts.ISSBinary == "" {
		opts.ISSBinary = "instrument-script-server"
	}

	c := &ScriptServerClient{
		host:       host,
		port:       port,
		issBinary:  opts.ISSBinary,
		issLibPath: opts.ISSLibPath,
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

// ListInstruments returns the list of available instruments.
func (c *ScriptServerClient) ListInstruments() ([]string, error) {
	ctx, cancel, err := c.callContext(defaultCallTimeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	resp, err := c.client.ListInstruments(ctx, &daemonv1.ListInstrumentsRequest{})
	if err != nil {
		return nil, err
	}
	if err := standardError(resp.GetStandardResponse()); err != nil {
		return nil, err
	}
	return append([]string{}, resp.GetInstrumentName()...), nil
}

// StartInstrument sends the StartInstrument RPC to create an instrument.
func (c *ScriptServerClient) StartInstrument(configPath string, pluginPath string) (string, error) {
	ctx, cancel, err := c.callContext(defaultCallTimeout)
	if err != nil {
		return "", err
	}
	defer cancel()

	resp, err := c.client.StartInstrument(ctx, &daemonv1.StartInstrumentRequest{
		ConfigPath: configPath,
		PluginPath: pluginPath,
		LogLevel:   defaultISSLogLevel,
	})
	if err != nil {
		return "", err
	}
	if err := standardError(resp.GetStandardResponse()); err != nil {
		return "", err
	}

	// The new ISS API does not echo the instrument name. Keep the old return
	// shape for callers that ignore it, and let ListInstruments provide names.
	return "", nil
}

// StopInstrument sends the StopInstrument RPC to remove an instrument.
func (c *ScriptServerClient) StopInstrument(name string) error {
	ctx, cancel, err := c.callContext(defaultCallTimeout)
	if err != nil {
		return err
	}
	defer cancel()

	resp, err := c.client.StopInstrument(ctx, &daemonv1.StopInstrumentRequest{
		InstrumentName: name,
	})
	if err != nil {
		return err
	}
	return standardError(resp.GetStandardResponse())
}

// StopDaemon asks the daemon to shut down over gRPC.
func (c *ScriptServerClient) StopDaemon() error {
	ctx, cancel, err := c.callContext(5 * time.Second)
	if err != nil {
		return err
	}
	defer cancel()

	resp, err := c.client.StopDaemon(ctx, &daemonv1.DaemonStop{})
	if err != nil {
		return err
	}
	return standardError(resp)
}

// Measure runs a Lua script as an ISS job and returns the parsed call results.
func (c *ScriptServerClient) Measure(scriptPath string, globals map[string]interface{}, typeManifest map[string]interface{}) ([]ISSCallResult, error) {
	ctx, cancel, err := c.callContext(defaultCallTimeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	req, err := buildMeasureJobRequest(scriptPath, globals, typeManifest)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.MeasureJob(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := standardError(resp.GetStandardResponse()); err != nil {
		return nil, err
	}

	result, err := c.waitForMeasureJob(resp.GetJobId())
	if err != nil {
		return nil, err
	}
	return measureJobResultToCallResults(result), nil
}

func (c *ScriptServerClient) waitForMeasureJob(jobID uint32) (*daemonv1.MeasureJobResultResponse, error) {
	deadline := time.Now().Add(5 * time.Minute)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for ISS measure job %d", jobID)
		}

		ctx, cancel, err := c.callContext(defaultCallTimeout)
		if err != nil {
			return nil, err
		}
		statusResp, err := c.client.JobStatus(ctx, &daemonv1.JobStatusRequest{JobId: jobID})
		cancel()
		if err != nil {
			return nil, err
		}
		if err := standardError(statusResp.GetStandardResponse()); err != nil {
			return nil, err
		}

		switch statusResp.GetJob().GetStatus() {
		case daemonv1.JobStatus_JOB_STATUS_COMPLETED:
			ctx, cancel, err := c.callContext(defaultCallTimeout)
			if err != nil {
				return nil, err
			}
			resultResp, err := c.client.MeasureJobResult(ctx, &daemonv1.MeasureJobResultRequest{JobId: jobID})
			cancel()
			if err != nil {
				return nil, err
			}
			if err := standardError(resultResp.GetStandardResponse()); err != nil {
				return nil, err
			}
			return resultResp, nil
		case daemonv1.JobStatus_JOB_STATUS_FAILED,
			daemonv1.JobStatus_JOB_STATUS_CANCELLED,
			daemonv1.JobStatus_JOB_STATUS_CANCELING:
			return nil, fmt.Errorf("ISS measure job %d ended with status %s", jobID, statusResp.GetJob().GetStatus().String())
		default:
			time.Sleep(defaultPollInterval)
		}
	}
}

func buildMeasureJobRequest(scriptPath string, globals map[string]interface{}, typeManifest map[string]interface{}) (*daemonv1.MeasureJobRequest, error) {
	req := &daemonv1.MeasureJobRequest{
		ScriptPath: scriptPath,
		Globals: &daemonv1.Globals{
			Map: map[string]*daemonv1.VariableValue{},
		},
		TypeManifest: &daemonv1.TypeManifest{},
	}

	for k, v := range globals {
		value, err := interfaceToVariableValue(v)
		if err != nil {
			return nil, fmt.Errorf("global %q: %w", k, err)
		}
		req.Globals.Map[k] = value
	}

	params, err := extractManifestParameters(typeManifest)
	if err != nil {
		return nil, err
	}
	req.TypeManifest.Parameters = params
	return req, nil
}

func extractManifestParameters(typeManifest map[string]interface{}) ([]*daemonv1.Parameter, error) {
	if typeManifest == nil {
		return nil, nil
	}
	rawParams, ok := typeManifest["parameters"]
	if !ok {
		return nil, nil
	}

	items, ok := rawParams.([]map[string]interface{})
	if !ok {
		if genericItems, ok := rawParams.([]interface{}); ok {
			items = make([]map[string]interface{}, 0, len(genericItems))
			for _, item := range genericItems {
				m, ok := item.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("invalid type_manifest parameter entry %T", item)
				}
				items = append(items, m)
			}
		} else {
			return nil, fmt.Errorf("invalid type_manifest parameters %T", rawParams)
		}
	}

	params := make([]*daemonv1.Parameter, 0, len(items))
	for _, item := range items {
		name, _ := item["name"].(string)
		typeName, _ := item["type"].(string)
		if name == "" {
			continue
		}
		// ISS always injects ctx itself before applying manifest parameters.
		if name == "ctx" || typeName == "RuntimeContext" {
			continue
		}
		params = append(params, &daemonv1.Parameter{
			Name: name,
			Type: luaTypeFromManifest(typeName),
		})
	}
	return params, nil
}

func luaTypeFromManifest(typeName string) daemonv1.LuaTypes {
	switch strings.TrimSpace(typeName) {
	case "number", "float", "double":
		return daemonv1.LuaTypes_LUA_TYPES_DOUBLE
	case "integer", "int", "int64":
		return daemonv1.LuaTypes_LUA_TYPES_INT64
	case "boolean", "bool":
		return daemonv1.LuaTypes_LUA_TYPES_BOOL
	case "string":
		return daemonv1.LuaTypes_LUA_TYPES_STRING
	case "DataBuffer":
		return daemonv1.LuaTypes_LUA_TYPES_DATA_BUFFER
	case "CallStack":
		return daemonv1.LuaTypes_LUA_TYPES_CALL_STACK
	default:
		if strings.HasPrefix(typeName, "{") || strings.HasPrefix(typeName, "[") {
			return daemonv1.LuaTypes_LUA_TYPES_MIXED_ARRAY
		}
		return daemonv1.LuaTypes_LUA_TYPES_UNSPECIFIED
	}
}

func interfaceToVariableValue(v interface{}) (*daemonv1.VariableValue, error) {
	switch value := v.(type) {
	case nil:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_IsNil{IsNil: true}}, nil
	case bool:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_B{B: value}}, nil
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
	case []int:
		values := make([]int64, len(value))
		for i, item := range value {
			values[i] = int64(item)
		}
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_IArray{IArray: &daemonv1.Int64Array{Values: values}}}, nil
	case []float64:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_DArray{DArray: &daemonv1.DoubleArray{Values: append([]float64{}, value...)}}}, nil
	case []string:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_SArray{SArray: &daemonv1.StringArray{Values: append([]string{}, value...)}}}, nil
	case []bool:
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_BArray{BArray: &daemonv1.BoolArray{Values: append([]bool{}, value...)}}}, nil
	case []map[string]interface{}:
		items := make([]*daemonv1.VariableValue, 0, len(value))
		for _, item := range value {
			converted, err := interfaceToVariableValue(item)
			if err != nil {
				return nil, err
			}
			items = append(items, converted)
		}
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_MArray{MArray: &daemonv1.MixedArray{Values: items}}}, nil
	case []interface{}:
		return interfaceSliceToVariableValue(value)
	case map[string]bool:
		return scalarMapToVariableValue(value)
	case map[string]int:
		return scalarMapToVariableValue(value)
	case map[string]int64:
		return scalarMapToVariableValue(value)
	case map[string]float64:
		return scalarMapToVariableValue(value)
	case map[string]string:
		return scalarMapToVariableValue(value)
	case map[string]interface{}:
		values := make(map[string]*daemonv1.VariableValue, len(value))
		for k, item := range value {
			converted, err := interfaceToVariableValue(item)
			if err != nil {
				return nil, fmt.Errorf("map key %q: %w", k, err)
			}
			values[k] = converted
		}
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_MMap{MMap: &daemonv1.MixedMap{Values: values}}}, nil
	default:
		return nil, fmt.Errorf("unsupported value type %T", v)
	}
}

func scalarMapToVariableValue[T bool | int | int64 | float64 | string](items map[string]T) (*daemonv1.VariableValue, error) {
	values := make(map[string]*daemonv1.VariableValue, len(items))
	for k, item := range items {
		converted, err := interfaceToVariableValue(item)
		if err != nil {
			return nil, fmt.Errorf("map key %q: %w", k, err)
		}
		values[k] = converted
	}
	return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_MMap{MMap: &daemonv1.MixedMap{Values: values}}}, nil
}

func interfaceSliceToVariableValue(items []interface{}) (*daemonv1.VariableValue, error) {
	if len(items) == 0 {
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_MArray{MArray: &daemonv1.MixedArray{}}}, nil
	}

	allInts := true
	ints := make([]int64, 0, len(items))
	allNumbers := true
	numbers := make([]float64, 0, len(items))
	allStrings := true
	stringsOut := make([]string, 0, len(items))
	allBools := true
	bools := make([]bool, 0, len(items))

	for _, item := range items {
		switch v := item.(type) {
		case int:
			ints = append(ints, int64(v))
			numbers = append(numbers, float64(v))
			allStrings = false
			allBools = false
		case int64:
			ints = append(ints, v)
			numbers = append(numbers, float64(v))
			allStrings = false
			allBools = false
		case float64:
			allInts = false
			numbers = append(numbers, v)
			allStrings = false
			allBools = false
		case string:
			allInts = false
			allNumbers = false
			stringsOut = append(stringsOut, v)
			allBools = false
		case bool:
			allInts = false
			allNumbers = false
			allStrings = false
			bools = append(bools, v)
		default:
			allInts = false
			allNumbers = false
			allStrings = false
			allBools = false
		}
	}

	switch {
	case allInts && len(ints) == len(items):
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_IArray{IArray: &daemonv1.Int64Array{Values: ints}}}, nil
	case allNumbers && len(numbers) == len(items):
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_DArray{DArray: &daemonv1.DoubleArray{Values: numbers}}}, nil
	case allStrings && len(stringsOut) == len(items):
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_SArray{SArray: &daemonv1.StringArray{Values: stringsOut}}}, nil
	case allBools && len(bools) == len(items):
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_BArray{BArray: &daemonv1.BoolArray{Values: bools}}}, nil
	default:
		mixed := make([]*daemonv1.VariableValue, 0, len(items))
		for _, item := range items {
			converted, err := interfaceToVariableValue(item)
			if err != nil {
				return nil, err
			}
			mixed = append(mixed, converted)
		}
		return &daemonv1.VariableValue{Value: &daemonv1.VariableValue_MArray{MArray: &daemonv1.MixedArray{Values: mixed}}}, nil
	}
}

func measureJobResultToCallResults(resp *daemonv1.MeasureJobResultResponse) []ISSCallResult {
	var results []ISSCallResult
	for _, cmd := range resp.GetResults() {
		params := cmd.GetParam()
		if len(params) == 0 {
			results = append(results, ISSCallResult{
				Index:        len(results),
				Instrument:   cmd.GetInstrumentName(),
				Verb:         cmd.GetVerb(),
				ExecutedAtMs: timestampMillis(cmd),
				Return:       ISSReturnValue{Type: "void"},
			})
			continue
		}

		for _, param := range params {
			results = append(results, ISSCallResult{
				Index:        len(results),
				Instrument:   cmd.GetInstrumentName(),
				Verb:         cmd.GetVerb(),
				ExecutedAtMs: timestampMillis(cmd),
				Return:       typedParameterToReturn(param),
			})
		}
	}
	return results
}

func timestampMillis(cmd *daemonv1.CommandResult) int64 {
	ts := cmd.GetExecutedAt()
	if ts == nil {
		return 0
	}
	return ts.AsTime().UnixMilli()
}

func typedParameterToReturn(param *daemonv1.TypedParameter) ISSReturnValue {
	value := param.GetValue()
	switch param.GetType() {
	case daemonv1.LuaTypes_LUA_TYPES_DOUBLE:
		return ISSReturnValue{Type: "double", Value: value.GetD()}
	case daemonv1.LuaTypes_LUA_TYPES_INT64:
		return ISSReturnValue{Type: "integer", Value: float64(value.GetI())}
	case daemonv1.LuaTypes_LUA_TYPES_BOOL:
		return ISSReturnValue{Type: "boolean", Value: value.GetB()}
	case daemonv1.LuaTypes_LUA_TYPES_STRING:
		return ISSReturnValue{Type: "string", Value: value.GetS()}
	case daemonv1.LuaTypes_LUA_TYPES_DATA_BUFFER:
		meta := param.GetDbmeta()
		return ISSReturnValue{
			Type:         "buffer",
			BufferID:     value.GetS(),
			ElementCount: int(meta.GetElementCount()),
			DataType:     strconv.FormatUint(uint64(meta.GetDataType()), 10),
		}
	default:
		return variableValueToReturn(value)
	}
}

func variableValueToReturn(value *daemonv1.VariableValue) ISSReturnValue {
	switch v := value.GetValue().(type) {
	case *daemonv1.VariableValue_I:
		return ISSReturnValue{Type: "integer", Value: float64(v.I)}
	case *daemonv1.VariableValue_D:
		return ISSReturnValue{Type: "double", Value: v.D}
	case *daemonv1.VariableValue_B:
		return ISSReturnValue{Type: "boolean", Value: v.B}
	case *daemonv1.VariableValue_S:
		return ISSReturnValue{Type: "string", Value: v.S}
	case *daemonv1.VariableValue_DArray:
		return ISSReturnValue{Type: "double_array", Value: append([]float64{}, v.DArray.GetValues()...)}
	case *daemonv1.VariableValue_IArray:
		values := make([]float64, len(v.IArray.GetValues()))
		for i, item := range v.IArray.GetValues() {
			values[i] = float64(item)
		}
		return ISSReturnValue{Type: "integer_array", Value: values}
	case *daemonv1.VariableValue_IsNil:
		return ISSReturnValue{Type: "void"}
	default:
		return ISSReturnValue{Type: "unknown"}
	}
}

// ReadBuffer retrieves float64 data for a buffer id. ISS 2.0.0 does not expose
// buffer reads over gRPC, so this shells out to the installed CLI as a temporary
// compatibility adapter.
func (c *ScriptServerClient) ReadBuffer(bufferID string) ([]float64, error) {
	cmd := exec.Command(c.issBinary, "buffer", "read", bufferID, "--json")
	cmd.Env = c.envWithRuntimePaths()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("instrument-script-server buffer read failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var payload struct {
		OK     bool              `json:"ok"`
		Error  []string          `json:"error"`
		Output []json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse buffer read JSON: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !payload.OK {
		return nil, fmt.Errorf("buffer read failed: %s", strings.Join(payload.Error, "; "))
	}

	for _, raw := range payload.Output {
		var candidate struct {
			Data []float64 `json:"data"`
		}
		if err := json.Unmarshal(raw, &candidate); err == nil && candidate.Data != nil {
			return candidate.Data, nil
		}
	}
	return nil, fmt.Errorf("buffer read JSON did not contain data for %s", bufferID)
}

func (c *ScriptServerClient) envWithRuntimePaths() []string {
	env := os.Environ()
	if c.issLibPath != "" {
		env = setOrPrependEnv(env, "LD_LIBRARY_PATH", c.issLibPath)
	}
	if c.issBinary != "" {
		env = setOrPrependEnv(env, "PATH", filepath.Dir(c.issBinary))
	}
	env = setOrReplaceEnv(env, "INSTRUMENT_SCRIPT_SERVER_RPC_PORT", strconv.Itoa(c.port))
	return env
}

func setOrPrependEnv(env []string, key string, prefix string) []string {
	if prefix == "" {
		return env
	}
	for i, item := range env {
		if strings.HasPrefix(item, key+"=") {
			current := strings.TrimPrefix(item, key+"=")
			if current == "" {
				env[i] = key + "=" + prefix
			} else {
				env[i] = key + "=" + prefix + ":" + current
			}
			return env
		}
	}
	return append(env, key+"="+prefix)
}

func setOrReplaceEnv(env []string, key string, value string) []string {
	for i, item := range env {
		if strings.HasPrefix(item, key+"=") {
			env[i] = key + "=" + value
			return env
		}
	}
	return append(env, key+"="+value)
}
