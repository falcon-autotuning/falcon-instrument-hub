//go:build cgo && falcon_core

package handlers

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/autotuner-interfaces/contexts/acquisitioncontext"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listlabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
)

type measurementResponseTarget struct {
	BufferData     []float64
	PortJSON       string
	ConnectionJSON string
	InstrumentType string
	UnitsJSON      string
}

// buildMeasurementResponseJSON constructs a falcon-core MeasurementResponse
// from the raw buffer data and port metadata and returns the cereal JSON string.
// The caller is responsible for wrapping this in the NATS wire envelope.
//
// Parameters:
//   - bufferData:      float64 samples from ISS
//   - setterConnJSON:  cereal JSON for the setter port's pseudo-name (connection)
//   - getterInstrType: instrument type string of the getter (e.g. "VOLTMETER")
//   - getterUnitsJSON: cereal JSON for the getter's units (symbolunit)
//   - hash:            unused here; kept for a consistent signature
func buildMeasurementResponseJSON(
	bufferData []float64,
	portJSON string,
	setterConnJSON string,
	getterInstrType string,
	getterUnitsJSON string,
	hash int64,
) (string, error) {
	return buildMeasurementResponseJSONForTargets([]measurementResponseTarget{
		{
			BufferData:     bufferData,
			PortJSON:       portJSON,
			ConnectionJSON: setterConnJSON,
			InstrumentType: getterInstrType,
			UnitsJSON:      getterUnitsJSON,
		},
	}, hash)
}

func buildMeasurementResponseJSONForTargets(
	targets []measurementResponseTarget,
	hash int64,
) (string, error) {
	if len(targets) == 0 {
		return "", fmt.Errorf("buildMeasurementResponseJSONForTargets requires at least one target")
	}

	labelledArrays := make([]*labelledmeasuredarray.Handle, 0, len(targets))
	for _, target := range targets {
		var ac *acquisitioncontext.Handle
		if target.PortJSON != "" {
			port, err := instrumentport.FromJSON(target.PortJSON)
			if err != nil {
				return "", fmt.Errorf("buildMeasurementResponseJSON instrumentport.FromJSON: %w", err)
			}
			ac, err = acquisitioncontext.NewFromPort(port)
			port.Close()
			if err != nil {
				return "", fmt.Errorf("buildMeasurementResponseJSON acquisitioncontext.NewFromPort: %w", err)
			}
		} else {
			conn, err := connection.FromJSON(target.ConnectionJSON)
			if err != nil {
				return "", fmt.Errorf("buildMeasurementResponseJSON connection.FromJSON: %w", err)
			}

			units, err := symbolunit.FromJSON(target.UnitsJSON)
			if err != nil {
				conn.Close()
				return "", fmt.Errorf("buildMeasurementResponseJSON symbolunit.FromJSON: %w", err)
			}

			ac, err = acquisitioncontext.New(conn, target.InstrumentType, units)
			units.Close()
			conn.Close()
			if err != nil {
				return "", fmt.Errorf("buildMeasurementResponseJSON acquisitioncontext.New: %w", err)
			}
		}

		bufferData := target.BufferData
		if len(bufferData) == 0 {
			bufferData = []float64{}
		}
		fa, err := farraydouble.FromData(bufferData, []uint64{uint64(len(bufferData))})
		if err != nil {
			ac.Close()
			return "", fmt.Errorf("buildMeasurementResponseJSON farraydouble.FromData: %w", err)
		}

		lma, err := labelledmeasuredarray.FromFArray(fa, ac)
		fa.Close()
		ac.Close()
		if err != nil {
			return "", fmt.Errorf("buildMeasurementResponseJSON labelledmeasuredarray.FromFArray: %w", err)
		}
		labelledArrays = append(labelledArrays, lma)
	}
	defer func() {
		for _, labelledArray := range labelledArrays {
			labelledArray.Close()
		}
	}()

	list, err := listlabelledmeasuredarray.New(labelledArrays)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON listlabelledmeasuredarray.New: %w", err)
	}
	defer list.Close()

	arrays, err := labelledarrayslabelledmeasuredarray.NewFromList(list)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON labelledarrayslabelledmeasuredarray.NewFromList: %w", err)
	}
	defer arrays.Close()

	resp, err := measurementresponse.New(arrays)
	if err != nil {
		return "", fmt.Errorf("buildMeasurementResponseJSON measurementresponse.New: %w", err)
	}
	defer resp.Close()

	return resp.ToJSON()
}
