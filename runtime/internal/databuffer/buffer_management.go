//go:build cgo

package databuffer

import (
	"fmt"
	"sync"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
)

type readableBuffer interface {
	dataBufferType() ArrayType

	float32Slice() []float32
	float64Slice() []float64

	int32Slice() []int32
	int64Slice() []int64

	uint32Slice() []uint32
	uint64Slice() []uint64

	uint8Slice() []byte
}

// Our connection to the raw C Buffer interface
var _ readableBuffer = (*Buffer)(nil)

type TrackedBuffer struct {
	RequestorID string
	BufferID    string

	Buffer   readableBuffer
	Metadata *BufferMetadata

	ClaimedAt time.Time
}

type bufferLibrary interface {
	getBuffer(string) (readableBuffer, error)
	getMetadata(string) (*BufferMetadata, error)
	releaseBuffer(string)
}
type bufferReleaser interface {
	ReleaseBuffer(bufferID string) error
}

// Our connection to the ISS
var _ bufferReleaser = (*instrumentserver.ScriptServerClient)(nil)

type DataBufferManager struct {
	mu sync.RWMutex

	bufferLib bufferLibrary
	releaser  bufferReleaser

	buffers map[string]*TrackedBuffer
}

func newDataBufferManager(
	bufferLib bufferLibrary,
	releaser bufferReleaser,
) *DataBufferManager {
	return &DataBufferManager{
		bufferLib: bufferLib,
		releaser:  releaser,
		buffers:   make(map[string]*TrackedBuffer),
	}
}

// Our connection to the Cgo code implementing buffer handling
type sharedMemoryBufferLibrary struct{}

func (sharedMemoryBufferLibrary) getBuffer(
	bufferID string,
) (readableBuffer, error) {
	return getBuffer(bufferID)
}

func (sharedMemoryBufferLibrary) getMetadata(
	bufferID string,
) (*BufferMetadata, error) {
	return getMetadata(bufferID)
}

func (sharedMemoryBufferLibrary) releaseBuffer(
	bufferID string,
) {
	releaseBuffer(bufferID)
}

var _ bufferLibrary = (*sharedMemoryBufferLibrary)(nil)

func NewDataBufferManager(
	releaser bufferReleaser,
) *DataBufferManager {
	return newDataBufferManager(sharedMemoryBufferLibrary{}, releaser)
}

func (m *DataBufferManager) RegisterBuffer(
	requestorID string,
	bufferID string,
) error {
	buffer, err := m.bufferLib.getBuffer(bufferID)
	if err != nil {
		return err
	}

	meta, err := m.bufferLib.getMetadata(bufferID)
	if err != nil {
		releaseBuffer(bufferID)
		return err
	}

	if err := m.releaser.ReleaseBuffer(bufferID); err != nil {
		releaseBuffer(bufferID)
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.buffers[bufferID] = &TrackedBuffer{
		RequestorID: requestorID,
		BufferID:    bufferID,
		Buffer:      buffer,
		Metadata:    meta,
		ClaimedAt:   time.Now(),
	}

	return nil
}

func (m *DataBufferManager) ListBuffers() []TrackedBuffer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]TrackedBuffer, 0, len(m.buffers))

	for _, buffer := range m.buffers {
		result = append(
			result,
			*buffer,
		)
	}

	return result
}

func (m *DataBufferManager) BuffersForRequestor(
	requestorID string,
) []TrackedBuffer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []TrackedBuffer

	for _, b := range m.buffers {
		if b.RequestorID == requestorID {
			result = append(
				result,
				*b,
			)
		}
	}

	return result
}

func (m *DataBufferManager) TotalBytes() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total uint64

	for _, buffer := range m.buffers {
		total += buffer.Metadata.ByteSize
	}

	return total
}

type ManagerStats struct {
	BufferCount int
	TotalBytes  uint64
}

func (m *DataBufferManager) Stats() ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total uint64

	for _, buffer := range m.buffers {
		total += buffer.Metadata.ByteSize
	}

	return ManagerStats{
		BufferCount: len(m.buffers),
		TotalBytes:  total,
	}
}

func (m *DataBufferManager) Metadata(
	bufferID string,
) (*BufferMetadata, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	buffer, ok := m.buffers[bufferID]
	if !ok {
		return nil, false
	}

	return buffer.Metadata, true
}

type BufferContents struct {
	Metadata *BufferMetadata
	Data     any
}

func (c *BufferContents) Float64() ([]float64, bool) {
	v, ok := c.Data.([]float64)
	return v, ok
}

func (c *BufferContents) Float32() ([]float32, bool) {
	v, ok := c.Data.([]float32)
	return v, ok
}

func (c *BufferContents) Int32() ([]int32, bool) {
	v, ok := c.Data.([]int32)
	return v, ok
}

func (c *BufferContents) Int64() ([]int64, bool) {
	v, ok := c.Data.([]int64)
	return v, ok
}

func (c *BufferContents) Uint32() ([]uint32, bool) {
	v, ok := c.Data.([]uint32)
	return v, ok
}

func (c *BufferContents) Uint64() ([]uint64, bool) {
	v, ok := c.Data.([]uint64)
	return v, ok
}

func (c *BufferContents) Uint8() ([]byte, bool) {
	v, ok := c.Data.([]byte)
	return v, ok
}

func (m *DataBufferManager) ReadBuffer(
	bufferID string,
) (*BufferContents, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracked, ok := m.buffers[bufferID]
	if !ok {
		return nil, fmt.Errorf(
			"buffer %q not found",
			bufferID,
		)
	}

	buffer := tracked.Buffer
	bufferType := buffer.dataBufferType()

	switch bufferType {
	case Float32:
		return &BufferContents{
			Metadata: tracked.Metadata,
			Data:     buffer.float32Slice(),
		}, nil

	case Float64:
		return &BufferContents{
			Metadata: tracked.Metadata,
			Data:     buffer.float64Slice(),
		}, nil

	case Int32:
		return &BufferContents{
			Metadata: tracked.Metadata,
			Data:     buffer.int32Slice(),
		}, nil

	case Int64:
		return &BufferContents{
			Metadata: tracked.Metadata,
			Data:     buffer.int64Slice(),
		}, nil

	case Uint32:
		return &BufferContents{
			Metadata: tracked.Metadata,
			Data:     buffer.uint32Slice(),
		}, nil

	case Uint64:
		return &BufferContents{
			Metadata: tracked.Metadata,
			Data:     buffer.uint64Slice(),
		}, nil

	case Uint8:
		return &BufferContents{
			Metadata: tracked.Metadata,
			Data:     buffer.uint8Slice(),
		}, nil

	default:
		return nil, fmt.Errorf(
			"unsupported buffer type %d", bufferType,
		)
	}
}

func (m *DataBufferManager) ReleaseBuffer(
	bufferID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	buffer, ok := m.buffers[bufferID]
	if !ok {
		return fmt.Errorf(
			"buffer %q not found",
			bufferID,
		)
	}

	m.bufferLib.releaseBuffer(bufferID)

	delete(
		m.buffers,
		buffer.BufferID,
	)

	return nil
}

func (m *DataBufferManager) ReleaseRequestor(
	requestorID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, buffer := range m.buffers {
		if buffer.RequestorID != requestorID {
			continue
		}

		releaseBuffer(id)

		delete(
			m.buffers,
			id,
		)
	}

	return nil
}
