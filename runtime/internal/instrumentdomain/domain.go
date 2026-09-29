//go:build cgo

package instrumentdomain

/*
#cgo pkg-config: instrument-domain

#include <instrument-domain/instrument-domain.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type Domain struct {
	Min float64
	Max float64
}

func (d Domain) Serialize() (string, error) {
	domain := C.instrument_domain_create(
		C.double(d.Min),
		C.double(d.Max),
	)
	if domain == nil {
		return "", fmt.Errorf("create native InstrumentDomain")
	}
	defer C.instrument_domain_free(domain)

	serialized := C.instrument_domain_serialize(domain)
	if serialized == nil {
		return "", fmt.Errorf("serialize native InstrumentDomain")
	}
	defer C.free(unsafe.Pointer(serialized))

	return C.GoString(serialized), nil
}

func unserialize(raw string) (*Domain, error) {
	cRaw := C.CString(raw)
	defer C.free(unsafe.Pointer(cRaw))

	domain := C.instrument_domain_deserialize(cRaw)
	if domain == nil {
		return nil, fmt.Errorf("unserialize native InstrumentDomain")
	}
	defer C.instrument_domain_free(domain)

	return &Domain{
		Min: float64(C.instrument_domain_get_min(domain)),
		Max: float64(C.instrument_domain_get_max(domain)),
	}, nil
}
