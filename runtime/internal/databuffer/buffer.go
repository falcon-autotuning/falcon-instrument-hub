package databuffer

/*
#cgo pkg-config: instrument-data

#include <instrument-data.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type ArrayType uint8

const (
	Float32 ArrayType = iota
	Float64
	Int32
	Int64
	Uint32
	Uint64
	Uint8
)

func createBuffer(
	instrument string,
	commandID string,
	dataType ArrayType,
	elementCount uint64,
	data unsafe.Pointer,
) (string, error) {
	cInstrument := C.CString(instrument)
	defer C.free(unsafe.Pointer(cInstrument))

	cCommand := C.CString(commandID)
	defer C.free(unsafe.Pointer(cCommand))

	id := C.data_manager_create_buffer(
		cInstrument,
		cCommand,
		C.ArrayType(dataType),
		C.size_t(elementCount),
		data,
	)

	if id == nil {
		return "", fmt.Errorf(
			"failed creating buffer",
		)
	}

	return C.GoString(id), nil
}

func createBufferZeroCopy(
	instrument string,
	commandID string,
	dataType ArrayType,
	elementCount uint64,
) (string, unsafe.Pointer, error) {
	cInstrument := C.CString(instrument)
	defer C.free(unsafe.Pointer(cInstrument))

	cCommand := C.CString(commandID)
	defer C.free(unsafe.Pointer(cCommand))

	var ptr unsafe.Pointer

	id := C.data_manager_create_buffer_zero_copy(
		cInstrument,
		cCommand,
		C.ArrayType(dataType),
		C.size_t(elementCount),
		(*unsafe.Pointer)(&ptr),
	)

	if id == nil {
		return "", nil, fmt.Errorf(
			"failed creating zero-copy buffer",
		)
	}

	return C.GoString(id), ptr, nil
}

func releaseBuffer(bufferID string) {
	cID := C.CString(bufferID)
	defer C.free(unsafe.Pointer(cID))

	C.data_manager_release_buffer(cID)
}

type BufferMetadata struct {
	BufferID       string
	InstrumentName string
	CommandID      string

	Type ArrayType

	ElementCount uint64
	ByteSize     uint64

	TimestampMS uint64

	GlobalRefCount uint32
}

func getMetadata(bufferID string) (*BufferMetadata, error) {
	cID := C.CString(bufferID)
	defer C.free(unsafe.Pointer(cID))

	var meta C.SharedMetadata

	ok := C.data_manager_get_metadata(
		cID,
		&meta,
	)

	if !bool(ok) {
		return nil, fmt.Errorf(
			"buffer %q not found",
			bufferID,
		)
	}

	return &BufferMetadata{
		BufferID:       C.GoString(&meta.buffer_id[0]),
		InstrumentName: C.GoString(&meta.instrument_name[0]),
		CommandID:      C.GoString(&meta.command_id[0]),

		Type: ArrayType(meta._type),

		ElementCount: uint64(meta.element_count),
		ByteSize:     uint64(meta.byte_size),

		TimestampMS: uint64(meta.timestamp_ms),

		GlobalRefCount: uint32(meta.global_ref_count),
	}, nil
}

type Buffer struct {
	ptr *C.DataBuffer
}

func getBuffer(bufferID string) (*Buffer, error) {
	cID := C.CString(bufferID)
	defer C.free(unsafe.Pointer(cID))

	ptr := C.data_manager_get_buffer(cID)
	if ptr == nil {
		return nil, fmt.Errorf(
			"buffer %q not found",
			bufferID,
		)
	}

	return &Buffer{
		ptr: ptr,
	}, nil
}

func (b *Buffer) elementCount() int {
	return int(
		C.data_buffer_element_count(b.ptr),
	)
}

func (b *Buffer) dataBufferType() ArrayType {
	return ArrayType(
		C.data_buffer_type(b.ptr),
	)
}

func (b *Buffer) data() unsafe.Pointer {
	return C.data_buffer_data(b.ptr)
}

func (b *Buffer) float32Slice() []float32 {
	return unsafe.Slice(
		(*float32)(b.data()),
		b.elementCount(),
	)
}

func (b *Buffer) float64Slice() []float64 {
	return unsafe.Slice(
		(*float64)(b.data()),
		b.elementCount(),
	)
}

func (b *Buffer) int32Slice() []int32 {
	return unsafe.Slice(
		(*int32)(b.data()),
		b.elementCount(),
	)
}

func (b *Buffer) int64Slice() []int64 {
	return unsafe.Slice(
		(*int64)(b.data()),
		b.elementCount(),
	)
}

func (b *Buffer) uint32Slice() []uint32 {
	return unsafe.Slice(
		(*uint32)(b.data()),
		b.elementCount(),
	)
}

func (b *Buffer) uint64Slice() []uint64 {
	return unsafe.Slice(
		(*uint64)(b.data()),
		b.elementCount(),
	)
}

func (b *Buffer) uint8Slice() []byte {
	return unsafe.Slice(
		(*byte)(b.data()),
		b.elementCount(),
	)
}
