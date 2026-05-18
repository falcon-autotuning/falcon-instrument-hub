//go:build cgo && falcon_core

package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/autotuner-interfaces/contexts/acquisitioncontext"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listlabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
)

// buildMeasurementResponseJSON constructs a falcon-core MeasurementResponse
// from the raw buffer data and port metadata, then wraps it in a
// api.MeasureResponse JSON string.
//
// Parameters:
//   - bufferData:      float64 samples from ISS
//   - setterConnJSON:  cereal JSON for the setter port's pseudo-name (connection)
//   - getterInstrType: instrument type string of the getter (e.g. "VOLTMETER")
//   - getterUnitsJSON: cereal JSON for the getter's units (symbolunit)
//   - hash:            measurement hash from MeasureCommand, forwarded to response
func buildMeasurementResponseJSON(
	bufferData []float64,
	setterConnJSON string,
	getterInstrType string,
	getterUnitsJSON string,
	hash int64,
) (string, error) {
	// 1. Deserialise the setter connection.
	conn, err := connection.FromJSON(setterConnJSON)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON connection.FromJSON: %w", err)
	}
	defer conn.Close()

	// 2. Deserialise the getter units.
	units, err := symbolunit.FromJSON(getterUnitsJSON)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON symbolunit.FromJSON: %w", err)
	}
	defer units.Close()

	// 3. Build AcquisitionContext (setter connection + getter type/units).
	ac, err := acquisitioncontext.New(conn, getterInstrType, units)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON acquisitioncontext.New: %w", err)
	}
	defer ac.Close()

	// 4. Wrap buffer data in a 1-D FArrayDouble.
	if len(bufferData) == 0 {
		bufferData = []float64{}
	}
	fa, err := farraydouble.FromData(bufferData, []uint64{uint64(len(bufferData))})
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON farraydouble.FromData: %w", err)
	}
	defer fa.Close()

	// 5. Attach the label to the array.
	lma, err := labelledmeasuredarray.FromFArray(fa, ac)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON labelledmeasuredarray.FromFArray: %w", err)
	}
	defer lma.Close()

	// 6. Wrap in a list.
	list, err := listlabelledmeasuredarray.New([]*labelledmeasuredarray.Handle{lma})
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON listlabelledmeasuredarray.New: %w", err)
	}
	defer list.Close()

	// 7. Convert list to LabelledArraysLabelledMeasuredArray.
	arrays, err := labelledarrayslabelledmeasuredarray.NewFromList(list)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON labelledarrayslabelledmeasuredarray.NewFromList: %w", err)
	}
	defer arrays.Close()

	// 8. Build the MeasurementResponse.
	resp, err := measurementresponse.New(arrays)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON measurementresponse.New: %w", err)
	}
	defer resp.Close()

	// 9. Serialise the MeasurementResponse to cereal JSON.
	respJSON, err := resp.ToJSON()
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON resp.ToJSON: %w", err)
	}

	// 10. Wrap in the NATS wire format (api.MeasureResponse.Stream carries the cereal JSON).
	wrapped := api.MeasureResponse{
		Stream:    respJSON,
		Hash:      hash,
		Timestamp: time.Now().UnixMicro(),
	}
	data, err := json.Marshal(wrapped)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON json.Marshal: %w", err)
	}
	return string(data), nil
}
