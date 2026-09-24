//go:build cgo

package databuffer

import (
	"reflect"
	"testing"
	"unsafe"
)

func TestBufferLifecycle_Integration(t *testing.T) {
	expected := []float64{
		1.1,
		2.2,
		3.3,
		4.4,
	}

	bufferID, err := createBuffer(
		"IntegrationTestInstrument",
		"MeasureVoltage",
		Float64,
		uint64(len(expected)),
		unsafe.Pointer(&expected[0]),
	)
	if err != nil {
		t.Fatalf(
			"createBuffer failed: %v",
			err,
		)
	}

	t.Logf("created buffer %s", bufferID)

	meta, err := getMetadata(bufferID)
	if err != nil {
		t.Fatalf(
			"getMetadata failed: %v",
			err,
		)
	}

	if meta.BufferID != bufferID {
		t.Fatalf(
			"buffer id = %q, want %q",
			meta.BufferID,
			bufferID,
		)
	}

	if meta.InstrumentName != "IntegrationTestInstrument" {
		t.Fatalf(
			"instrument = %q",
			meta.InstrumentName,
		)
	}

	if meta.CommandID != "MeasureVoltage" {
		t.Fatalf(
			"command id = %q",
			meta.CommandID,
		)
	}

	if meta.Type != Float64 {
		t.Fatalf(
			"type = %d, want %d",
			meta.Type,
			Float64,
		)
	}

	if meta.ElementCount != uint64(len(expected)) {
		t.Fatalf(
			"element count = %d, want %d",
			meta.ElementCount,
			len(expected),
		)
	}

	buffer, err := getBuffer(bufferID)
	if err != nil {
		t.Fatalf(
			"getBuffer failed: %v",
			err,
		)
	}

	if buffer.dataBufferType() != Float64 {
		t.Fatalf(
			"type = %d, want Float64",
			buffer.dataBufferType(),
		)
	}

	if buffer.elementCount() != len(expected) {
		t.Fatalf(
			"element count = %d, want %d",
			buffer.elementCount(),
			len(expected),
		)
	}

	actual := buffer.float64Slice()

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"data mismatch\nwant=%v\ngot=%v",
			expected,
			actual,
		)
	}

	releaseBuffer(bufferID)
}

func TestCreateBufferZeroCopy_Integration(t *testing.T) {
	bufferID, ptr, err := createBufferZeroCopy(
		"IntegrationTestInstrument",
		"ZeroCopyCommand",
		Float64,
		4,
	)
	if err != nil {
		t.Fatalf(
			"createBufferZeroCopy failed: %v",
			err,
		)
	}

	values := unsafe.Slice(
		(*float64)(ptr),
		4,
	)

	values[0] = 10
	values[1] = 20
	values[2] = 30
	values[3] = 40

	buffer, err := getBuffer(bufferID)
	if err != nil {
		t.Fatalf(
			"getBuffer failed: %v",
			err,
		)
	}

	got := buffer.float64Slice()

	want := []float64{
		10,
		20,
		30,
		40,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"want=%v got=%v",
			want,
			got,
		)
	}

	releaseBuffer(bufferID)
}
