//go:build cgo

package databuffer

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"
)

type mockBuffer struct {
	bufferType ArrayType

	float64Data []float64
	int32Data   []int32
}

func (m *mockBuffer) dataBufferType() ArrayType {
	return m.bufferType
}

func (m *mockBuffer) float64Slice() []float64 {
	return m.float64Data
}

func (m *mockBuffer) int32Slice() []int32 {
	return m.int32Data
}

func (m *mockBuffer) float32Slice() []float32 { return nil }
func (m *mockBuffer) int64Slice() []int64     { return nil }
func (m *mockBuffer) uint32Slice() []uint32   { return nil }
func (m *mockBuffer) uint64Slice() []uint64   { return nil }
func (m *mockBuffer) uint8Slice() []byte      { return nil }

type mockBufferLibrary struct {
	buffer    readableBuffer
	bufferErr error

	metadata    *BufferMetadata
	metadataErr error

	released []string
}

func (m *mockBufferLibrary) getBuffer(
	id string,
) (readableBuffer, error) {
	return m.buffer, m.bufferErr
}

func (m *mockBufferLibrary) getMetadata(
	id string,
) (*BufferMetadata, error) {
	return m.metadata, m.metadataErr
}

func (m *mockBufferLibrary) releaseBuffer(
	id string,
) {
	m.released = append(
		m.released,
		id,
	)
}

type mockBufferReleaser struct {
	released []string
	err      error
}

func (m *mockBufferReleaser) ReleaseBuffer(
	bufferID string,
) error {
	m.released = append(
		m.released,
		bufferID,
	)

	return m.err
}

func TestNewDataBufferManager(t *testing.T) {
	releaser := &mockBufferReleaser{}

	manager := newDataBufferManager(
		&mockBufferLibrary{},
		releaser,
	)

	if manager == nil {
		t.Fatal("expected manager")
	}

	if manager.bufferLib == nil {
		t.Fatal("bufferLib nil")
	}

	if manager.releaser == nil {
		t.Fatal("releaser nil")
	}

	if len(manager.buffers) != 0 {
		t.Fatal("expected empty buffer map")
	}
}

func TestRegisterBuffer(t *testing.T) {
	lib := &mockBufferLibrary{
		buffer: &Buffer{},
		metadata: &BufferMetadata{
			BufferID: "buf1",
			ByteSize: 100,
		},
	}

	releaser := &mockBufferReleaser{}

	manager := newDataBufferManager(
		lib,
		releaser,
	)

	err := manager.RegisterBuffer(
		"requestor1",
		"buf1",
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(manager.buffers) != 1 {
		t.Fatalf(
			"got %d buffers",
			len(manager.buffers),
		)
	}

	if len(releaser.released) != 1 {
		t.Fatal("expected ISS release")
	}

	if releaser.released[0] != "buf1" {
		t.Fatal("wrong buffer id")
	}
}

func TestRegisterBuffer_GetBufferError(
	t *testing.T,
) {
	lib := &mockBufferLibrary{
		bufferErr: fmt.Errorf("boom"),
	}

	manager := newDataBufferManager(
		lib,
		&mockBufferReleaser{},
	)

	err := manager.RegisterBuffer(
		"requestor1",
		"buf1",
	)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterBuffer_MetadataError(
	t *testing.T,
) {
	lib := &mockBufferLibrary{
		buffer:      &Buffer{},
		metadataErr: fmt.Errorf("boom"),
	}

	manager := newDataBufferManager(
		lib,
		&mockBufferReleaser{},
	)

	err := manager.RegisterBuffer(
		"requestor1",
		"buf1",
	)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterBuffer_ReleaseError(
	t *testing.T,
) {
	lib := &mockBufferLibrary{
		buffer: &Buffer{},
		metadata: &BufferMetadata{
			BufferID: "buf1",
		},
	}

	releaser := &mockBufferReleaser{
		err: fmt.Errorf("boom"),
	}

	manager := newDataBufferManager(
		lib,
		releaser,
	)

	err := manager.RegisterBuffer(
		"requestor1",
		"buf1",
	)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListBuffers(t *testing.T) {
	manager := newDataBufferManager(
		&mockBufferLibrary{},
		&mockBufferReleaser{},
	)

	manager.buffers["a"] = &TrackedBuffer{
		BufferID: "a",
	}

	manager.buffers["b"] = &TrackedBuffer{
		BufferID: "b",
	}

	got := manager.ListBuffers()

	if len(got) != 2 {
		t.Fatalf(
			"got %d",
			len(got),
		)
	}
}

func TestBuffersForRequestor(t *testing.T) {
	manager := newDataBufferManager(
		&mockBufferLibrary{},
		&mockBufferReleaser{},
	)

	manager.buffers["1"] = &TrackedBuffer{
		RequestorID: "job1",
	}

	manager.buffers["2"] = &TrackedBuffer{
		RequestorID: "job2",
	}

	manager.buffers["3"] = &TrackedBuffer{
		RequestorID: "job1",
	}

	got := manager.BuffersForRequestor(
		"job1",
	)

	if len(got) != 2 {
		t.Fatalf(
			"got %d",
			len(got),
		)
	}
}

func TestTotalBytes(t *testing.T) {
	manager := newDataBufferManager(
		&mockBufferLibrary{},
		&mockBufferReleaser{},
	)

	manager.buffers["1"] = &TrackedBuffer{
		Metadata: &BufferMetadata{
			ByteSize: 100,
		},
	}

	manager.buffers["2"] = &TrackedBuffer{
		Metadata: &BufferMetadata{
			ByteSize: 200,
		},
	}

	if got := manager.TotalBytes(); got != 300 {
		t.Fatalf("got %d", got)
	}
}

func TestStats(t *testing.T) {
	manager := newDataBufferManager(
		&mockBufferLibrary{},
		&mockBufferReleaser{},
	)

	manager.buffers["1"] = &TrackedBuffer{
		Metadata: &BufferMetadata{
			ByteSize: 100,
		},
	}

	manager.buffers["2"] = &TrackedBuffer{
		Metadata: &BufferMetadata{
			ByteSize: 200,
		},
	}

	stats := manager.Stats()

	if stats.BufferCount != 2 {
		t.Fatalf(
			"count=%d",
			stats.BufferCount,
		)
	}

	if stats.TotalBytes != 300 {
		t.Fatalf(
			"bytes=%d",
			stats.TotalBytes,
		)
	}
}

func TestMetadata(t *testing.T) {
	manager := newDataBufferManager(
		&mockBufferLibrary{},
		&mockBufferReleaser{},
	)

	meta := &BufferMetadata{
		BufferID: "buf1",
	}

	manager.buffers["buf1"] = &TrackedBuffer{
		Metadata: meta,
	}

	got, ok := manager.Metadata("buf1")

	if !ok {
		t.Fatal("expected metadata")
	}

	if got != meta {
		t.Fatal("wrong metadata")
	}
}

func TestMetadata_NotFound(t *testing.T) {
	manager := newDataBufferManager(
		&mockBufferLibrary{},
		&mockBufferReleaser{},
	)

	_, ok := manager.Metadata("missing")

	if ok {
		t.Fatal("expected false")
	}
}

func TestReleaseBuffer(t *testing.T) {
	lib := &mockBufferLibrary{}

	manager := newDataBufferManager(
		lib,
		&mockBufferReleaser{},
	)

	manager.buffers["buf1"] = &TrackedBuffer{
		BufferID: "buf1",
	}

	if err := manager.ReleaseBuffer("buf1"); err != nil {
		t.Fatal(err)
	}

	if len(manager.buffers) != 0 {
		t.Fatal("buffer not removed")
	}

	if len(lib.released) != 1 {
		t.Fatal("release not called")
	}
}

func TestReleaseBuffer_NotFound(
	t *testing.T,
) {
	manager := newDataBufferManager(
		&mockBufferLibrary{},
		&mockBufferReleaser{},
	)

	err := manager.ReleaseBuffer("missing")

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReleaseRequestor(t *testing.T) {
	manager := newDataBufferManager(
		&mockBufferLibrary{},
		&mockBufferReleaser{},
	)

	manager.buffers["1"] = &TrackedBuffer{
		RequestorID: "job1",
	}

	manager.buffers["2"] = &TrackedBuffer{
		RequestorID: "job1",
	}

	manager.buffers["3"] = &TrackedBuffer{
		RequestorID: "job2",
	}

	err := manager.ReleaseRequestor(
		"job1",
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(manager.buffers) != 1 {
		t.Fatalf(
			"got %d buffers",
			len(manager.buffers),
		)
	}
}

func TestReadBuffer_Float64(t *testing.T) {
	manager := newDataBufferManager(
		&mockBufferLibrary{
			buffer: &mockBuffer{
				bufferType: Float64,
				float64Data: []float64{
					1.0,
					2.0,
				},
			},
		},
		&mockBufferReleaser{},
	)

	manager.buffers["buf1"] = &TrackedBuffer{
		BufferID: "buf1",
		Buffer: &mockBuffer{
			bufferType: Float64,
			float64Data: []float64{
				1.0,
				2.0,
			},
		},
		Metadata: &BufferMetadata{},
	}

	contents, err := manager.ReadBuffer("buf1")
	if err != nil {
		t.Fatal(err)
	}

	data, ok := contents.Float64()
	if !ok {
		t.Fatal("expected float64")
	}

	if len(data) != 2 {
		t.Fatal("wrong length")
	}
}

func TestDataBufferManager_HappyPath(
	t *testing.T,
) {
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

	releaser := &mockBufferReleaser{}

	manager := NewDataBufferManager(
		releaser,
	)

	err = manager.RegisterBuffer(
		"request-123",
		bufferID,
	)
	if err != nil {
		t.Fatalf(
			"RegisterBuffer failed: %v",
			err,
		)
	}

	if len(releaser.released) != 1 {
		t.Fatalf(
			"release calls=%d want=1",
			len(releaser.released),
		)
	}

	if releaser.released[0] != bufferID {
		t.Fatalf(
			"released=%q want=%q",
			releaser.released[0],
			bufferID,
		)
	}

	stats := manager.Stats()

	if stats.BufferCount != 1 {
		t.Fatalf(
			"buffer count=%d want=1",
			stats.BufferCount,
		)
	}

	contents, err := manager.ReadBuffer(
		bufferID,
	)
	if err != nil {
		t.Fatalf(
			"ReadBuffer failed: %v",
			err,
		)
	}

	data, ok := contents.Float64()
	if !ok {
		t.Fatal(
			"expected float64 buffer",
		)
	}

	if !reflect.DeepEqual(
		data,
		expected,
	) {
		t.Fatalf(
			"data mismatch\nwant=%v\ngot=%v",
			expected,
			data,
		)
	}

	meta, ok := manager.Metadata(
		bufferID,
	)
	if !ok {
		t.Fatal(
			"metadata missing",
		)
	}

	if meta.BufferID != bufferID {
		t.Fatalf(
			"metadata buffer id=%q want=%q",
			meta.BufferID,
			bufferID,
		)
	}

	buffers := manager.BuffersForRequestor(
		"request-123",
	)

	if len(buffers) != 1 {
		t.Fatalf(
			"buffers=%d want=1",
			len(buffers),
		)
	}

	err = manager.ReleaseBuffer(
		bufferID,
	)
	if err != nil {
		t.Fatalf(
			"ReleaseBuffer failed: %v",
			err,
		)
	}

	if len(manager.ListBuffers()) != 0 {
		t.Fatal(
			"expected no tracked buffers",
		)
	}
}

func TestDataBufferManager_AllTypes(
	t *testing.T,
) {
	releaser := &mockBufferReleaser{}

	tests := []struct {
		name string
		typ  ArrayType
		data any
	}{
		{
			name: "float32",
			typ:  Float32,
			data: []float32{1, 2, 3},
		},
		{
			name: "float64",
			typ:  Float64,
			data: []float64{1, 2, 3},
		},
		{
			name: "int32",
			typ:  Int32,
			data: []int32{1, 2, 3},
		},
		{
			name: "int64",
			typ:  Int64,
			data: []int64{1, 2, 3},
		},
		{
			name: "uint32",
			typ:  Uint32,
			data: []uint32{1, 2, 3},
		},
		{
			name: "uint64",
			typ:  Uint64,
			data: []uint64{1, 2, 3},
		},
		{
			name: "uint8",
			typ:  Uint8,
			data: []byte{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewDataBufferManager(
				releaser,
			)

			var (
				bufferID string
				err      error
			)

			switch v := tt.data.(type) {
			case []float32:
				bufferID, err = createBuffer(
					"test",
					tt.name,
					tt.typ,
					uint64(len(v)),
					unsafe.Pointer(&v[0]),
				)

			case []float64:
				bufferID, err = createBuffer(
					"test",
					tt.name,
					tt.typ,
					uint64(len(v)),
					unsafe.Pointer(&v[0]),
				)

			case []int32:
				bufferID, err = createBuffer(
					"test",
					tt.name,
					tt.typ,
					uint64(len(v)),
					unsafe.Pointer(&v[0]),
				)

			case []int64:
				bufferID, err = createBuffer(
					"test",
					tt.name,
					tt.typ,
					uint64(len(v)),
					unsafe.Pointer(&v[0]),
				)

			case []uint32:
				bufferID, err = createBuffer(
					"test",
					tt.name,
					tt.typ,
					uint64(len(v)),
					unsafe.Pointer(&v[0]),
				)

			case []uint64:
				bufferID, err = createBuffer(
					"test",
					tt.name,
					tt.typ,
					uint64(len(v)),
					unsafe.Pointer(&v[0]),
				)

			case []byte:
				bufferID, err = createBuffer(
					"test",
					tt.name,
					tt.typ,
					uint64(len(v)),
					unsafe.Pointer(&v[0]),
				)
			}

			if err != nil {
				t.Fatal(err)
			}

			if err := manager.RegisterBuffer(
				"requestor",
				bufferID,
			); err != nil {
				t.Fatal(err)
			}

			contents, err := manager.ReadBuffer(
				bufferID,
			)
			if err != nil {
				t.Fatal(err)
			}

			switch expected := tt.data.(type) {

			case []float32:
				got, ok := contents.Float32()
				if !ok {
					t.Fatal("expected float32")
				}
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("want=%v got=%v",
						expected, got)
				}

			case []float64:
				got, ok := contents.Float64()
				if !ok {
					t.Fatal("expected float64")
				}
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("want=%v got=%v",
						expected, got)
				}

			case []int32:
				got, ok := contents.Int32()
				if !ok {
					t.Fatal("expected int32")
				}
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("want=%v got=%v",
						expected, got)
				}

			case []int64:
				got, ok := contents.Int64()
				if !ok {
					t.Fatal("expected int64")
				}
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("want=%v got=%v",
						expected, got)
				}

			case []uint32:
				got, ok := contents.Uint32()
				if !ok {
					t.Fatal("expected uint32")
				}
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("want=%v got=%v",
						expected, got)
				}

			case []uint64:
				got, ok := contents.Uint64()
				if !ok {
					t.Fatal("expected uint64")
				}
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("want=%v got=%v",
						expected, got)
				}

			case []byte:
				got, ok := contents.Uint8()
				if !ok {
					t.Fatal("expected uint8")
				}
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("want=%v got=%v",
						expected, got)
				}
			}

			if err := manager.ReleaseBuffer(
				bufferID,
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}
