// prepare_get_voltage_request.go
//
// Builds a Falcon "get_voltage" measurement payload (per falcon-measurement-lib schema)
// and wraps it into a falcon-core-libs MeasurementRequest.
//
// Requirements (for the falcon-core-libs part):
//   - CGO enabled
//   - falcon_core_c_api installed and discoverable via pkg-config (falcon_core_c_api.pc)
//   - go module can import: github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/...
//
// If you only need the JSON payload, see BuildGetVoltagePayloadJSON().

package main

import (
	"encoding/json"
	"fmt"
	"os"

	// falcon-core-libs (Go bindings over the falcon-core C-API)
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listwaveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/labelleddomain"
)

//
// ---- Schema-matching payload types (falcon-measurement-lib) ----
//
// falcon-measurement-lib/schemas/scripts/get_voltage.json:
// {
//   "type": "object",
//   "properties": { "getter": { "$ref": "../lib/instrument_target.json#/definitions/InstrumentTarget" } }
// }
//
// InstrumentTarget has:
//   id: string (required)
//   channel: integer (optional)
//

type InstrumentTarget struct {
	ID      string `json:"id"`
	Channel *int   `json:"channel,omitempty"`
}

type GetVoltagePayload struct {
	Getter InstrumentTarget `json:"getter"`
}

// BuildGetVoltagePayloadJSON returns a JSON payload that conforms to the
// falcon-measurement-lib get_voltage schema.
func BuildGetVoltagePayloadJSON(instrumentID string, channel *int) ([]byte, error) {
	if instrumentID == "" {
		return nil, fmt.Errorf("instrumentID cannot be empty")
	}

	payload := GetVoltagePayload{
		Getter: InstrumentTarget{
			ID:      instrumentID,
			Channel: channel,
		},
	}
	return json.Marshal(payload)
}

//
// ---- Wrapping into falcon-core MeasurementRequest ----
//
// The falcon-core MeasurementRequest has fields like:
//   - message (string)
//   - measurement_name (string)
//   - waveforms (list)
//   - getters (ports list)
//   - meter_transforms (map)
//   - time_domain (labelled domain)
//
// For get_voltage, the simplest strategy is:
//   measurement_name = "get_voltage"
//   message = <payload JSON string>
//   waveforms/getters/transforms empty
//   time_domain minimal execution clock domain
//

func BuildFalconCoreMeasurementRequestForGetVoltage(instrumentID string, channel *int) (*measurementrequest.Handle, error) {
	// 1) Build schema-valid payload JSON
	payloadJSON, err := BuildGetVoltagePayloadJSON(instrumentID, channel)
	if err != nil {
		return nil, err
	}

	// 2) Create minimal required falcon-core objects
	waveforms, err := listwaveform.NewEmpty()
	if err != nil {
		return nil, fmt.Errorf("listwaveform.NewEmpty: %w", err)
	}
	// NOTE: Close these handles after you’re done with the request handle.
	// In this sample we rely on main() to exit; in a server you should Close().

	getters, err := ports.NewEmpty()
	if err != nil {
		return nil, fmt.Errorf("ports.NewEmpty: %w", err)
	}

	meterTransforms, err := mapinstrumentportporttransform.NewEmpty()
	if err != nil {
		return nil, fmt.Errorf("mapinstrumentportporttransform.NewEmpty: %w", err)
	}

	clock, err := instrumentport.NewExecutionClock()
	if err != nil {
		return nil, fmt.Errorf("instrumentport.NewExecutionClock: %w", err)
	}

	// Minimal time domain: [0, 1] on the execution clock
	timeDomain, err := labelleddomain.NewFromPort(0, 1, clock, true, true)
	if err != nil {
		return nil, fmt.Errorf("labelleddomain.NewFromPort: %w", err)
	}

	// 3) Construct MeasurementRequest
	const measurementName = "get_voltage"
	req, err := measurementrequest.New(
		string(payloadJSON), // message (we store the script payload here)
		measurementName,     // measurement_name (used later to select script)
		waveforms,
		getters,
		meterTransforms,
		timeDomain,
	)
	if err != nil {
		return nil, fmt.Errorf("measurementrequest.New: %w", err)
	}

	return req, nil
}

func main() {
	// Example: get voltage from instrument "GPI1", channel 0
	ch := 0

	req, err := BuildFalconCoreMeasurementRequestForGetVoltage("GPI1", &ch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build request: %v\n", err)
		os.Exit(1)
	}
	defer req.Close()

	// Serialize to JSON (canonical falcon-core JSON for the MeasurementRequest object)
	reqJSON, err := req.ToJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to serialize MeasurementRequest to JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(reqJSON)

	// In your hub bridge, you’ll typically:
	//   - read req.MeasurementName() to choose the measurement script (get_voltage.lua)
	//   - provide req.Message() (payload JSON) to the script-server runtime (e.g., staging file or inline RPC if added)
}
