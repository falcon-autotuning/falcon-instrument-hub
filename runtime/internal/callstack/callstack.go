//go:build cgo && falcon_core

// DEPRECATED: THe hub should not need this anymore

// Package callstack owns the native CallStack serialization boundary used by
// measurement handlers.
package callstack

/*
#cgo pkg-config: instrument-call-stack
#include <stdlib.h>
#include <instrument-call-stack/instrument-call-stack.h>
*/
import "C"

import (
	"fmt"
	"math"
	"strings"
	"unsafe"
)

const maxStringBytes = 63

// Descriptor identifies one instrument command.
type Descriptor struct {
	Instrument string
	Group      string
	Channel    int
	Command    string
}

func validateField(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maxStringBytes {
		return fmt.Errorf("%s exceeds %d bytes", name, maxStringBytes)
	}
	if strings.ContainsAny(value, "|\x00") {
		return fmt.Errorf("%s contains an unsupported delimiter", name)
	}
	return nil
}

// Serialize creates the wire representation consumed by ISS. The installed
// native library currently returns a malloc-allocated buffer, so this Linux
// build releases it with C.free until the library exposes its own free API.
func (d Descriptor) Serialize() (string, error) {
	if err := validateField("instrument", d.Instrument); err != nil {
		return "", err
	}
	if err := validateField("group", d.Group); err != nil {
		return "", err
	}
	if err := validateField("command", d.Command); err != nil {
		return "", err
	}
	if d.Channel < math.MinInt32 || d.Channel > math.MaxInt32 {
		return "", fmt.Errorf("channel %d is outside the native int range", d.Channel)
	}

	instrument := C.CString(d.Instrument)
	defer C.free(unsafe.Pointer(instrument))
	group := C.CString(d.Group)
	defer C.free(unsafe.Pointer(group))
	command := C.CString(d.Command)
	defer C.free(unsafe.Pointer(command))

	stack := C.instrument_call_stack_create(
		instrument,
		group,
		C.int(d.Channel),
		command,
	)
	if stack == nil {
		return "", fmt.Errorf("create native CallStack")
	}
	defer C.instrument_call_stack_free(stack)

	serialized := C.instrument_call_stack_serialize(stack)
	if serialized == nil {
		return "", fmt.Errorf("serialize native CallStack")
	}
	defer C.free(unsafe.Pointer(serialized))

	return C.GoString(serialized), nil
}
