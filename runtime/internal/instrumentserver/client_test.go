package instrumentserver

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	daemonv1 "github.com/falcon-autotuning/instrument-server/runtime/internal/issproto/instserver/daemon/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func TestLoadMeasurementTimeout_ValidEnv(t *testing.T) {
	t.Setenv("MEASUREMENT_TIMEOUT_SEC", "123")

	got := loadMeasurementTimeout()

	if got != 123*time.Second {
		t.Fatalf("got %v, want %v", got, 123*time.Second)
	}
}

func TestCurrentMeasurementTimeout(t *testing.T) {
	old := baseMeasurementTimeout
	baseMeasurementTimeout = 10 * time.Second
	defer func() {
		baseMeasurementTimeout = old
	}()

	inflightMeasurements.Store(0)

	if got := currentMeasurementTimeout(); got != 10*time.Second {
		t.Fatalf("got %v", got)
	}

	inflightMeasurements.Store(3)

	if got := currentMeasurementTimeout(); got != 30*time.Second {
		t.Fatalf("got %v", got)
	}
}

type mockInstrument struct {
	ConfigPath string
	PluginPath string
	LogLevel   string
}
type mockDaemonClient struct {
	instruments map[string]mockInstrument

	daemonRunning   bool
	daemonStatusErr error
	stopDaemonCalls int

	measureJobReqs       []*daemonv1.MeasureJobRequest
	jobStatusRequests    []*daemonv1.JobStatusRequest
	measureResultRequest []*daemonv1.MeasureJobResultRequest

	jobStatuses []daemonv1.JobStatus

	measureJobResponse *daemonv1.MeasureJobResponse
	measureJobResult   *daemonv1.MeasureJobResultResponse
}

func (m *mockDaemonClient) ListInstruments(
	ctx context.Context,
	req *daemonv1.ListInstrumentsRequest,
	opts ...grpc.CallOption,
) (*daemonv1.ListInstrumentsResponse, error) {
	names := make([]string, 0, len(m.instruments))

	for name := range m.instruments {
		names = append(names, name)
	}

	sort.Strings(names)

	return &daemonv1.ListInstrumentsResponse{
		StandardResponse: &daemonv1.StandardResponse{
			Ok: true,
		},
		InstrumentName: names,
	}, nil
}

func (m *mockDaemonClient) MeasureJob(
	ctx context.Context,
	req *daemonv1.MeasureJobRequest,
	opts ...grpc.CallOption,
) (*daemonv1.MeasureJobResponse, error) {
	m.measureJobReqs = append(m.measureJobReqs, req)

	if m.measureJobResponse != nil {
		return m.measureJobResponse, nil
	}

	return &daemonv1.MeasureJobResponse{
		JobId: 1,
		StandardResponse: &daemonv1.StandardResponse{
			Ok: true,
		},
	}, nil
}

func (m *mockDaemonClient) JobStatus(
	ctx context.Context,
	req *daemonv1.JobStatusRequest,
	opts ...grpc.CallOption,
) (*daemonv1.JobStatusResponse, error) {
	m.jobStatusRequests = append(
		m.jobStatusRequests,
		req,
	)

	status := daemonv1.JobStatus_JOB_STATUS_COMPLETED

	if len(m.jobStatuses) > 0 {
		status = m.jobStatuses[0]
		m.jobStatuses = m.jobStatuses[1:]
	}

	return &daemonv1.JobStatusResponse{
		Job: &daemonv1.Job{
			Type:       daemonv1.JobType_JOB_TYPE_MEASURE,
			Status:     status,
			CreatedAt:  timestamppb.Now(),
			StartedAt:  timestamppb.Now(),
			FinishedAt: timestamppb.Now(),
		},
	}, nil
}

func (m *mockDaemonClient) MeasureJobResult(
	ctx context.Context,
	req *daemonv1.MeasureJobResultRequest,
	opts ...grpc.CallOption,
) (*daemonv1.MeasureJobResultResponse, error) {
	m.measureResultRequest = append(
		m.measureResultRequest,
		req,
	)

	return m.measureJobResult, nil
}

func (m *mockDaemonClient) StopDaemon(
	context.Context,
	*daemonv1.DaemonStop,
	...grpc.CallOption,
) (*daemonv1.StandardResponse, error) {
	m.stopDaemonCalls++

	return &daemonv1.StandardResponse{
		Ok: true,
	}, nil
}

func (m *mockDaemonClient) StartInstrument(
	ctx context.Context,
	req *daemonv1.StartInstrumentRequest,
	opts ...grpc.CallOption,
) (*daemonv1.StartInstrumentResponse, error) {
	if m.instruments == nil {
		m.instruments = make(map[string]mockInstrument)
	}

	name := req.GetConfigPath()

	m.instruments[name] = mockInstrument{
		ConfigPath: req.GetConfigPath(),
		PluginPath: req.GetPluginPath(),
		LogLevel:   req.GetLogLevel(),
	}

	return &daemonv1.StartInstrumentResponse{
		StandardResponse: &daemonv1.StandardResponse{
			Ok: true,
		},
	}, nil
}

func (m *mockDaemonClient) StopInstrument(
	ctx context.Context,
	req *daemonv1.StopInstrumentRequest,
	opts ...grpc.CallOption,
) (*daemonv1.StopInstrumentResponse, error) {
	delete(m.instruments, req.GetInstrumentName())

	return &daemonv1.StopInstrumentResponse{
		StandardResponse: &daemonv1.StandardResponse{
			Ok: true,
		},
	}, nil
}

func (m *mockDaemonClient) CancelJob(
	context.Context,
	*daemonv1.CancelJobRequest,
	...grpc.CallOption,
) (*daemonv1.CancelJobResponse, error) {
	panic("unexpected call")
}

func (m *mockDaemonClient) JobList(
	context.Context,
	*daemonv1.JobListRequest,
	...grpc.CallOption,
) (*daemonv1.JobListResponse, error) {
	panic("unexpected call")
}

func (m *mockDaemonClient) DaemonStatus(
	ctx context.Context,
	req *daemonv1.DaemonStatusRequest,
	opts ...grpc.CallOption,
) (*daemonv1.DaemonStatusResponse, error) {
	if m.daemonStatusErr != nil {
		return nil, m.daemonStatusErr
	}

	return &daemonv1.DaemonStatusResponse{
		StandardResponse: &daemonv1.StandardResponse{
			Ok: true,
		},
		Running: m.daemonRunning,
	}, nil
}

func (m *mockDaemonClient) InstrumentStatus(
	context.Context,
	*daemonv1.InstrumentStatusRequest,
	...grpc.CallOption,
) (*daemonv1.InstrumentStatusResponse, error) {
	panic("unexpected call")
}

func (m *mockDaemonClient) Discover(
	context.Context,
	*daemonv1.DiscoverRequest,
	...grpc.CallOption,
) (*daemonv1.DiscoverResponse, error) {
	panic("unexpected call")
}

func (m *mockDaemonClient) ListDataBuffers(
	context.Context,
	*daemonv1.ListDataBuffersRequest,
	...grpc.CallOption,
) (*daemonv1.ListDataBuffersResponse, error) {
	panic("unexpected call")
}

func (m *mockDaemonClient) ReleaseBuffer(
	context.Context,
	*daemonv1.ReleaseBufferRequest,
	...grpc.CallOption,
) (*daemonv1.ReleaseBufferResponse, error) {
	panic("unexpected call")
}

func (m *mockDaemonClient) GetBufferMetadata(
	context.Context,
	*daemonv1.GetBufferMetadataRequest,
	...grpc.CallOption,
) (*daemonv1.GetBufferMetadataResponse, error) {
	panic("unexpected call")
}

var _ daemonv1.DaemonServiceClient = (*mockDaemonClient)(nil)

func TestStandardError(t *testing.T) {
	tests := []struct {
		name    string
		resp    *daemonv1.StandardResponse
		wantErr bool
	}{
		{
			name:    "nil response",
			resp:    nil,
			wantErr: true,
		},
		{
			name: "success",
			resp: &daemonv1.StandardResponse{
				Ok: true,
			},
			wantErr: false,
		},
		{
			name: "error message",
			resp: &daemonv1.StandardResponse{
				Ok: false,
				Error: &daemonv1.ErrorDetails{
					Message: "boom",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := standardError(tt.resp)

			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStandardError_MessageField(t *testing.T) {
	err := standardError(
		&daemonv1.StandardResponse{
			Ok:      false,
			Message: "some failure",
		},
	)

	if err == nil {
		t.Fatal("expected error")
	}

	if err.Error() != "some failure" {
		t.Fatalf("got %q", err.Error())
	}
}

func TestStandardError_DefaultFailure(t *testing.T) {
	err := standardError(
		&daemonv1.StandardResponse{
			Ok: false,
		},
	)

	if err == nil {
		t.Fatal("expected error")
	}

	if err.Error() != "ISS request failed" {
		t.Fatalf("got %q", err.Error())
	}
}

func TestVariableValueLuaType(t *testing.T) {
	tests := []struct {
		name string
		in   VariableValue
		want LuaType
	}{
		{
			name: "nil",
			in:   VariableValue{Value: nil},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_UNSPECIFIED),
		},
		{
			name: "int64",
			in:   VariableValue{Value: int64(1)},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_INT64),
		},
		{
			name: "double",
			in:   VariableValue{Value: float64(1.5)},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_DOUBLE),
		},
		{
			name: "bool",
			in:   VariableValue{Value: true},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_BOOL),
		},
		{
			name: "string",
			in:   VariableValue{Value: "hello"},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_STRING),
		},
		{
			name: "data buffer",
			in: VariableValue{
				Value: DataBuffer("buffer-1"),
			},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_DATA_BUFFER),
		},
		{
			name: "call stack",
			in: VariableValue{
				Value: CallStack("stack-1"),
			},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_CALL_STACK),
		},
		{
			name: "int64 array",
			in: VariableValue{
				Value: Int64Array{1, 2},
			},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_INT64_ARRAY),
		},
		{
			name: "double array",
			in: VariableValue{
				Value: DoubleArray{1.1, 2.2},
			},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_DOUBLE_ARRAY),
		},
		{
			name: "bool array",
			in: VariableValue{
				Value: BoolArray{true},
			},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_BOOL_ARRAY),
		},
		{
			name: "string array",
			in: VariableValue{
				Value: StringArray{"a"},
			},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_STRING_ARRAY),
		},
		{
			name: "data buffer array",
			in: VariableValue{
				Value: DataBufferArray{"buf1"},
			},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_DATA_BUFFER_ARRAY),
		},
		{
			name: "call stack array",
			in: VariableValue{
				Value: CallStackArray{"stack1"},
			},
			want: LuaType(daemonv1.LuaTypes_LUA_TYPES_CALL_STACK_ARRAY),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.LuaType()

			if got != tt.want {
				t.Fatalf(
					"got %v want %v",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestToGrpcVariableValue_PrimitiveVariants(t *testing.T) {
	tests := []VariableValue{
		{Value: int(1)},
		{Value: int8(1)},
		{Value: int16(1)},
		{Value: int32(1)},
		{Value: int64(1)},
		{Value: uint(1)},
		{Value: uint8(1)},
		{Value: uint16(1)},
		{Value: uint32(1)},
		{Value: uint64(1)},
		{Value: float32(1.5)},
		{Value: float64(1.5)},
	}

	for _, tt := range tests {
		value, err := toGrpcVariableValue(tt)
		if err != nil {
			t.Fatalf(
				"unexpected error for %#v: %v",
				tt,
				err,
			)
		}

		if value == nil {
			t.Fatalf(
				"nil grpc value for %#v",
				tt,
			)
		}
	}
}

func TestVariableValueRoundTrip(t *testing.T) {
	values := []VariableValue{
		{Value: nil},
		{Value: true},
		{Value: int64(123)},
		{Value: 4.5},
		{Value: "abc"},
		{Value: Int64Array{1, 2, 3}},
		{Value: DoubleArray{1.1, 2.2}},
		{Value: BoolArray{true}},
		{Value: StringArray{"x", "y"}},
	}

	for _, original := range values {
		grpcValue, err := toGrpcVariableValue(original)
		if err != nil {
			t.Fatal(err)
		}

		roundTrip := fromGrpcVariableValue(grpcValue)

		if !reflect.DeepEqual(original, roundTrip) {
			t.Fatalf(
				"roundtrip mismatch\nwant=%#v\ngot=%#v",
				original,
				roundTrip,
			)
		}
	}
}

func TestToGrpcVariableValue_Uint64Overflow(t *testing.T) {
	_, err := toGrpcVariableValue(
		VariableValue{
			Value: ^uint64(0),
		},
	)

	if err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestToGrpcVariableValue_UnsupportedType(t *testing.T) {
	_, err := toGrpcVariableValue(
		VariableValue{
			Value: struct{}{},
		},
	)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStopDaemon(t *testing.T) {
	mock := &mockDaemonClient{
		instruments: make(map[string]mockInstrument),
	}

	client := newScriptServerClientForTests(mock)

	if err := client.StopDaemon(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.stopDaemonCalls != 1 {
		t.Fatalf(
			"stop daemon calls=%d want=1",
			mock.stopDaemonCalls,
		)
	}
}

func TestDaemonStatus(t *testing.T) {
	mock := &mockDaemonClient{
		instruments:   make(map[string]mockInstrument),
		daemonRunning: true,
	}

	client := newScriptServerClientForTests(mock)

	running, err := client.DaemonStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !running {
		t.Fatal("expected daemon to be running")
	}
}

func TestDaemonStatus_NotRunning(t *testing.T) {
	mock := &mockDaemonClient{
		instruments:   make(map[string]mockInstrument),
		daemonRunning: false,
	}

	client := newScriptServerClientForTests(mock)

	running, err := client.DaemonStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if running {
		t.Fatal("expected daemon to be stopped")
	}
}

func TestDaemonStatus_RPCError(t *testing.T) {
	mock := &mockDaemonClient{
		daemonStatusErr: errors.New("rpc failure"),
	}

	client := newScriptServerClientForTests(mock)

	_, err := client.DaemonStatus()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInstrumentLifecycle(t *testing.T) {
	mock := &mockDaemonClient{}
	client := newScriptServerClientForTests(mock)

	err := client.StartInstrument(
		"/configs/dmm.yaml",
		"/plugins/libdmm.so",
	)
	if err != nil {
		t.Fatal(err)
	}

	instruments, err := client.ListInstruments()
	if err != nil {
		t.Fatal(err)
	}

	if len(instruments) != 1 {
		t.Fatalf(
			"expected 1 instrument got %d",
			len(instruments),
		)
	}

	if instruments[0] != "/configs/dmm.yaml" {
		t.Fatalf(
			"unexpected instrument %q",
			instruments[0],
		)
	}

	err = client.StopInstrument(
		"/configs/dmm.yaml",
	)
	if err != nil {
		t.Fatal(err)
	}

	instruments, err = client.ListInstruments()
	if err != nil {
		t.Fatal(err)
	}

	if len(instruments) != 0 {
		t.Fatalf(
			"expected no instruments got %d",
			len(instruments),
		)
	}
}

func TestMeasureJobResultToCallResultsMapsBufferReturn(t *testing.T) {
	bufferId := "buffer-123"
	verb := "GET_DATAPOINT"
	instrument := "Meter1"
	returnName := "return"
	var elementCount uint32 = 42
	resp := &daemonv1.MeasureJobResultResponse{
		Results: []*daemonv1.CommandResult{
			{
				InstrumentName: instrument,
				Verb:           verb,
				Param: []*daemonv1.TypedParameter{
					{
						Name: returnName,
						Type: daemonv1.LuaTypes_LUA_TYPES_DATA_BUFFER,
						Value: &daemonv1.VariableValue{
							Value: &daemonv1.VariableValue_S{
								S: bufferId,
							},
						},
						Dbmeta: &daemonv1.DataBufferMetadata{
							ElementCount: elementCount,
							DataType:     3,
						},
					},
				},
			},
		},
	}

	results := measureJobResultToCallResults(resp)

	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}

	result := results[0]

	if result.Instrument != instrument {
		t.Fatalf("instrument = %q, want Meter1", result.Instrument)
	}

	if result.Verb != verb {
		t.Fatalf("verb = %q, want %s", result.Verb, verb)
	}

	if len(result.Return) != 1 {
		t.Fatalf("return count = %d, want 1", len(result.Return))
	}
	if result.Return[0].Name != returnName {
		t.Fatalf(
			"return name = %q, want %s",
			result.Return[0].Name,
			returnName,
		)
	}

	gotValue, ok := result.Return[0].Value.Value.(string)
	if !ok {
		t.Fatalf(
			"return value type = %T, want string",
			result.Return[0].Value.Value,
		)
	}

	if gotValue != bufferId {
		t.Fatalf(
			"buffer id = %q, want %s",
			gotValue,
			bufferId,
		)
	}

	if result.Return[0].Metadata.ElementCount != elementCount {
		t.Fatalf(
			"element count = %d, want %d",
			result.Return[0].Metadata.ElementCount,
			elementCount,
		)
	}
}

func TestMeasure_HappyPath(t *testing.T) {
	channel := int64(1)
	mock := &mockDaemonClient{
		jobStatuses: []daemonv1.JobStatus{
			daemonv1.JobStatus_JOB_STATUS_QUEUED,
			daemonv1.JobStatus_JOB_STATUS_RUNNING,
			daemonv1.JobStatus_JOB_STATUS_COMPLETED,
		},

		measureJobResponse: &daemonv1.MeasureJobResponse{
			JobId: 42,
			StandardResponse: &daemonv1.StandardResponse{
				Ok: true,
			},
		},

		measureJobResult: &daemonv1.MeasureJobResultResponse{
			Results: []*daemonv1.CommandResult{
				{
					InstrumentName: "DMM6500",
					Verb:           "measure_voltage",
					Channel:        &channel,

					Param: []*daemonv1.TypedParameter{
						{
							Name: "voltage",
							Unit: "V",

							Value: &daemonv1.VariableValue{
								Value: &daemonv1.VariableValue_D{
									D: 1.234,
								},
							},
						},
					},
				},
			},
		},
	}

	client := newScriptServerClientForTests(mock)

	vars := []MeasureVariable{
		{
			Name: "count",
			Value: VariableValue{
				Value: int64(100),
			},
		},
		{
			Name: "name",
			Value: VariableValue{
				Value: "my-device",
			},
		},
		{
			Name: "enabled",
			Value: VariableValue{
				Value: true,
			},
		},
	}
	script := "/tmp/test.lua"

	results, err := client.Measure(
		script,
		vars,
	)
	if err != nil {
		t.Fatalf("Measure failed: %v", err)
	}

	if len(mock.measureJobReqs) != 1 {
		t.Fatalf(
			"expected 1 MeasureJob request, got %d",
			len(mock.measureJobReqs),
		)
	}

	req := mock.measureJobReqs[0]

	if req.GetScriptPath() != script {
		t.Fatalf(
			"wrong script path: %s",
			req.GetScriptPath(),
		)
	}

	if len(req.GetGlobals().GetMap()) != 3 {
		t.Fatalf(
			"expected 3 globals, got %d",
			len(req.GetGlobals().GetMap()),
		)
	}

	if got := req.Globals.Map["count"].GetI(); got != 100 {
		t.Fatalf("count=%d", got)
	}

	if got := req.Globals.Map["name"].GetS(); got != "my-device" {
		t.Fatalf("name=%q", got)
	}

	if got := req.Globals.Map["enabled"].GetB(); !got {
		t.Fatalf("enabled=%v", got)
	}

	if len(mock.jobStatusRequests) != 3 {
		t.Fatalf(
			"expected 3 status polls, got %d",
			len(mock.jobStatusRequests),
		)
	}

	if len(results) != 1 {
		t.Fatalf(
			"expected 1 result, got %d",
			len(results),
		)
	}

	result := results[0]

	if result.Instrument != "DMM6500" {
		t.Fatalf(
			"unexpected instrument: %s",
			result.Instrument,
		)
	}

	if result.Verb != "measure_voltage" {
		t.Fatalf(
			"unexpected verb: %s",
			result.Verb,
		)
	}

	if len(result.Return) != 1 {
		t.Fatalf(
			"expected 1 return value, got %d",
			len(result.Return),
		)
	}

	value, ok := result.Return[0].Value.Value.(float64)
	if !ok {
		t.Fatalf(
			"expected float64, got %T",
			result.Return[0].Value.Value,
		)
	}

	if value != 1.234 {
		t.Fatalf(
			"expected 1.234, got %f",
			value,
		)
	}
}

func TestMeasure_FailedJob(t *testing.T) {
	mock := &mockDaemonClient{
		jobStatuses: []daemonv1.JobStatus{
			daemonv1.JobStatus_JOB_STATUS_FAILED,
		},
		measureJobResponse: &daemonv1.MeasureJobResponse{
			JobId: 42,
			StandardResponse: &daemonv1.StandardResponse{
				Ok: true,
			},
		},
	}

	client := newScriptServerClientForTests(mock)

	_, err := client.Measure(
		"/tmp/test.lua",
		nil,
	)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMeasure_CancelledJob(t *testing.T) {
	mock := &mockDaemonClient{
		jobStatuses: []daemonv1.JobStatus{
			daemonv1.JobStatus_JOB_STATUS_CANCELLED,
		},
		measureJobResponse: &daemonv1.MeasureJobResponse{
			JobId: 42,
			StandardResponse: &daemonv1.StandardResponse{
				Ok: true,
			},
		},
	}

	client := newScriptServerClientForTests(mock)

	_, err := client.Measure(
		"/tmp/test.lua",
		nil,
	)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCallContext_InitError(t *testing.T) {
	client := &ScriptServerClient{
		initErr: assertErr{},
	}

	_, _, err := client.callContext(time.Second)

	if err == nil {
		t.Fatal("expected error")
	}
}

type assertErr struct{}

func (assertErr) Error() string {
	return "boom"
}

func TestCallContext_NilClient(t *testing.T) {
	client := &ScriptServerClient{}

	_, _, err := client.callContext(time.Second)

	if err == nil {
		t.Fatal("expected error")
	}

	if err.Error() != "ISS gRPC client is not initialized" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTimestampMillis_Nil(t *testing.T) {
	cmd := &daemonv1.CommandResult{}

	got := timestampMillis(cmd)

	if got != 0 {
		t.Fatalf("got %d want 0", got)
	}
}

func TestTimestampMillis_Value(t *testing.T) {
	ts := time.Date(
		2024,
		1,
		2,
		3,
		4,
		5,
		0,
		time.UTC,
	)

	cmd := &daemonv1.CommandResult{
		ExecutedAt: timestamppb.New(ts),
	}

	got := timestampMillis(cmd)

	if got != ts.UnixMilli() {
		t.Fatalf(
			"got %d want %d",
			got,
			ts.UnixMilli(),
		)
	}
}

func TestBuildMeasureJobRequest(t *testing.T) {
	req, err := buildMeasureJobRequest(
		"/tmp/test.lua",
		[]MeasureVariable{
			{
				Name: "count",
				Value: VariableValue{
					Value: int64(10),
				},
			},
			{
				Name: "enabled",
				Value: VariableValue{
					Value: true,
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if req.GetScriptPath() != "/tmp/test.lua" {
		t.Fatalf(
			"script path = %q",
			req.GetScriptPath(),
		)
	}

	if got := req.Globals.Map["count"].GetI(); got != 10 {
		t.Fatalf("count=%d", got)
	}

	if got := req.Globals.Map["enabled"].GetB(); !got {
		t.Fatalf("enabled=%v", got)
	}

	if len(req.TypeManifest.Parameters) != 2 {
		t.Fatalf(
			"parameter count=%d",
			len(req.TypeManifest.Parameters),
		)
	}

	if req.TypeManifest.Parameters[0].Type != daemonv1.LuaTypes_LUA_TYPES_INT64 {
		t.Fatal("wrong type for count")
	}

	if req.TypeManifest.Parameters[1].Type != daemonv1.LuaTypes_LUA_TYPES_BOOL {
		t.Fatal("wrong type for enabled")
	}
}

func TestListInstruments_Empty(t *testing.T) {
	mock := &mockDaemonClient{
		instruments: map[string]mockInstrument{},
	}

	client := newScriptServerClientForTests(mock)

	instruments, err := client.ListInstruments()
	if err != nil {
		t.Fatal(err)
	}

	if len(instruments) != 0 {
		t.Fatalf(
			"expected empty list got %d",
			len(instruments),
		)
	}
}

func TestClose_Connected(t *testing.T) {
	conn, err := grpc.NewClient(
		"127.0.0.1:12345",
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	client := &ScriptServerClient{
		conn: conn,
	}

	if err := client.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewScriptServerClient_Defaults(t *testing.T) {
	client := NewScriptServerClient("", 0)

	if client == nil {
		t.Fatal("expected client")
	}

	if client.host != DefaultISSHost {
		t.Fatalf(
			"host=%q want=%q",
			client.host,
			DefaultISSHost,
		)
	}

	if client.port != DefaultISSPort {
		t.Fatalf(
			"port=%d want=%d",
			client.port,
			DefaultISSPort,
		)
	}

	if client.issBinary != "instrument-script-server" {
		t.Fatalf(
			"binary=%q",
			client.issBinary,
		)
	}

	_ = client.Close()
}

func TestNewScriptServerClientWithOptions(t *testing.T) {
	client := NewScriptServerClientWithOptions(
		"10.1.2.3",
		9999,
		ScriptServerClientOptions{
			ISSBinary: "custom-iss",
		},
	)

	if client.host != "10.1.2.3" {
		t.Fatalf("host=%q", client.host)
	}

	if client.port != 9999 {
		t.Fatalf("port=%d", client.port)
	}

	if client.issBinary != "custom-iss" {
		t.Fatalf(
			"binary=%q",
			client.issBinary,
		)
	}

	_ = client.Close()
}
