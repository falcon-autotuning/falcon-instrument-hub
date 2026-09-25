package interpreter

import (
	"errors"
	"sync"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExecutor struct {
	results []instrumentserver.CallResult
	err     error

	mu    sync.Mutex
	calls int
}

func (m *mockExecutor) Measure(
	scriptPath string,
	variables []instrumentserver.MeasureVariable,
) ([]instrumentserver.CallResult, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	return m.results, nil
}

type mockMeasurementClient struct {
	results []instrumentserver.CallResult
	err     error

	registered  []string
	registerErr error
}

func (m *mockMeasurementClient) Measure(
	scriptPath string,
	variables []instrumentserver.MeasureVariable,
) ([]instrumentserver.CallResult, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.results, nil
}

func (m *mockMeasurementClient) RegisterBuffer(
	bufferID string,
) error {
	if m.registerErr != nil {
		return m.registerErr
	}

	m.registered = append(
		m.registered,
		bufferID,
	)

	return nil
}

var _ measurementClient = (*mockMeasurementClient)(nil)

type registerCall struct {
	requestorID string
	bufferID    string
}

type mockRegistrar struct {
	mu    sync.Mutex
	calls []registerCall

	err error
}

func (m *mockRegistrar) RegisterBuffer(
	requestorID string,
	bufferID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return m.err
	}

	m.calls = append(
		m.calls,
		registerCall{
			requestorID: requestorID,
			bufferID:    bufferID,
		},
	)

	return nil
}

func TestMeasurementClientMeasure(t *testing.T) {
	exec := &mockExecutor{
		results: []instrumentserver.CallResult{
			{
				Verb: "test",
			},
		},
	}

	client := &measurementAdapter{
		executor: exec,
	}

	results, err := client.Measure(
		"test.lua",
		nil,
	)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "test", results[0].Verb)
	assert.Equal(t, 1, exec.calls)
}

func TestMeasurementClientRegisterBuffer(t *testing.T) {
	reg := &mockRegistrar{}

	client := &measurementAdapter{
		buffers:     reg,
		requestorID: "measurement-123",
	}

	err := client.RegisterBuffer("buffer-456")

	require.NoError(t, err)
	require.Len(t, reg.calls, 1)

	assert.Equal(
		t,
		"measurement-123",
		reg.calls[0].requestorID,
	)

	assert.Equal(
		t,
		"buffer-456",
		reg.calls[0].bufferID,
	)
}

func TestScriptDispatcherRunMeasurement_NoBuffers(
	t *testing.T,
) {
	client := &mockMeasurementClient{
		results: []instrumentserver.CallResult{
			{
				Verb: "TEST",
			},
		},
	}

	dispatcher := newScriptDispatcher(
		client,
		"/tmp",
	)

	results, err := dispatcher.RunMeasurement(
		"measure",
		nil,
	)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Empty(t, client.registered)
}

func TestScriptDispatcherRunMeasurement_DataBuffer(
	t *testing.T,
) {
	client := &mockMeasurementClient{
		results: []instrumentserver.CallResult{
			{
				Return: []instrumentserver.ReturnValue{
					{
						Value: instrumentserver.VariableValue{
							Value: instrumentserver.DataBuffer(
								"buffer-1",
							),
						},
					},
				},
			},
		},
	}

	dispatcher := newScriptDispatcher(
		client,
		"/tmp",
	)

	_, err := dispatcher.RunMeasurement(
		"script",
		nil,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{"buffer-1"},
		client.registered,
	)
}

func TestScriptDispatcherRunMeasurement_DataBufferArray(
	t *testing.T,
) {
	client := &mockMeasurementClient{
		results: []instrumentserver.CallResult{
			{
				Return: []instrumentserver.ReturnValue{
					{
						Value: instrumentserver.VariableValue{
							Value: instrumentserver.DataBufferArray{
								"buf1",
								"buf2",
								"buf3",
							},
						},
					},
				},
			},
		},
	}

	dispatcher := newScriptDispatcher(
		client,
		"/tmp",
	)

	_, err := dispatcher.RunMeasurement(
		"script",
		nil,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{
			"buf1",
			"buf2",
			"buf3",
		},
		client.registered,
	)
}

func TestScriptDispatcherRunMeasurement_RegisterFails(
	t *testing.T,
) {
	client := &mockMeasurementClient{
		registerErr: errors.New("boom"),

		results: []instrumentserver.CallResult{
			{
				Return: []instrumentserver.ReturnValue{
					{
						Value: instrumentserver.VariableValue{
							Value: instrumentserver.DataBuffer(
								"buf1",
							),
						},
					},
				},
			},
		},
	}

	dispatcher := newScriptDispatcher(
		client,
		"/tmp",
	)

	_, err := dispatcher.RunMeasurement(
		"script",
		nil,
	)

	require.Error(t, err)
}

func TestMeasurementDispatcherRunMeasurement(
	t *testing.T,
) {
	exec := &mockExecutor{
		results: []instrumentserver.CallResult{
			{
				Verb: "RUN",
			},
		},
	}

	reg := &mockRegistrar{}

	d := NewMeasurementDispatcher(
		exec,
		reg,
		"/tmp",
	)

	result := d.runMeasurement(
		"measurement-1",
		"script",
		nil,
	)

	require.NoError(t, result.Err)

	assert.Equal(
		t,
		"measurement-1",
		result.ID,
	)

	assert.Len(t, result.Results, 1)
}

func TestMeasurementDispatcherRunMeasurement_Error(
	t *testing.T,
) {
	exec := &mockExecutor{
		err: errors.New("measure failed"),
	}

	reg := &mockRegistrar{}

	d := NewMeasurementDispatcher(
		exec,
		reg,
		"/tmp",
	)

	result := d.runMeasurement(
		"id",
		"script",
		nil,
	)

	require.Error(t, result.Err)
	assert.Nil(t, result.Results)
}

func TestMeasurementDispatcherRunAll(
	t *testing.T,
) {
	exec := &mockExecutor{
		results: []instrumentserver.CallResult{
			{
				Verb: "RUN",
			},
		},
	}

	reg := &mockRegistrar{}

	d := NewMeasurementDispatcher(
		exec,
		reg,
		"/tmp",
	)

	results := d.RunAll(
		[]MeasurementRequest{
			{Script: "s1"},
			{Script: "s2"},
			{Script: "s3"},
		},
	)

	require.Len(t, results, 3)

	for _, result := range results {
		require.NoError(t, result.Err)
		assert.NotEmpty(t, result.ID)
	}
}

func TestNextMeasurementIDUnique(
	t *testing.T,
) {
	id1 := nextMeasurementID()
	id2 := nextMeasurementID()

	assert.NotEqual(t, id1, id2)
}
