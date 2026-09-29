//go:build cgo

package instrumenttarget

/*
#cgo pkg-config: instrument-target
#include <stdlib.h>
#include <instrument-target/instrument-target.h>
*/
import "C"

import (
	"fmt"
	"math"
	"strings"
	"unsafe"
)

const maxStringBytes = 63

// Descriptor identifies one instrument target.
type Target struct {
	Instrument string
	Group      string
	Channel    int
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

func (t Target) Serialize() (string, error) {
	if err := validateField("instrument", t.Instrument); err != nil {
		return "", err
	}
	if err := validateField("group", t.Group); err != nil {
		return "", err
	}
	if t.Channel < math.MinInt32 || t.Channel > math.MaxInt32 {
		return "", fmt.Errorf("channel %d is outside the native int range", t.Channel)
	}

	instrument := C.CString(t.Instrument)
	defer C.free(unsafe.Pointer(instrument))
	group := C.CString(t.Group)
	defer C.free(unsafe.Pointer(group))

	target := C.instrument_target_create(
		instrument,
		group,
		C.int(t.Channel),
	)
	if target == nil {
		return "", fmt.Errorf("create native InstrumentTarget")
	}
	defer C.instrument_target_free(target)

	serialized := C.instrument_target_serialize(target)
	if serialized == nil {
		return "", fmt.Errorf("serialize native InstrumentTarget")
	}
	defer C.free(unsafe.Pointer(serialized))

	return C.GoString(serialized), nil
}

func deserialize(raw string) (*Target, error) {
	cRaw := C.CString(raw)
	defer C.free(unsafe.Pointer(cRaw))

	target := C.instrument_target_deserialize(cRaw)
	if target == nil {
		return nil, fmt.Errorf("deserialize native InstrumentTarget")
	}
	defer C.instrument_target_free(target)

	return &Target{
		Instrument: C.GoString(C.instrument_target_get_instrument_name(target)),
		Group:      C.GoString(C.instrument_target_get_channel_group(target)),
		Channel:    int(C.instrument_target_get_channel(target)),
	}, nil
}
