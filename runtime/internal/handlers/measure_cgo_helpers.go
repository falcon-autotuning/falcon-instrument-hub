//go:build cgo && falcon_core

package handlers

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
)

// gateNameFromConnectionJSON deserialises a cereal-format connection JSON using
// the falcon-core C library and returns the gate name (e.g. "P1").
func gateNameFromConnectionJSON(connectionJSON string) (string, error) {
	h, err := connection.FromJSON(connectionJSON)
	if err != nil {
		return "", fmt.Errorf("gateNameFromConnectionJSON: %w", err)
	}
	defer h.Close()
	return h.Name()
}
