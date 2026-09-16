package handlers

import "github.com/falcon-autotuning/instrument-server/runtime/internal/ports"

type measurementResponseTarget struct {
	BufferData     []float64
	PortJSON       string
	ConnectionJSON string
	InstrumentType string
	UnitsJSON      string
	ConnectedPort  *ports.ConnectedPort
}
