package cartesianmeasure

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdb"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
)

func buildWiremap(
	port falconcore.InstrumentPort,
	ports *config.ConnectedPorts,
) (measurementdb.Wiremap, error) {
	cp, found := settingrouter.ResolvePort(
		port,
		ports,
	)
	if !found {
		return measurementdb.Wiremap{}, fmt.Errorf(
			"port %q not found",
			port.DefaultName,
		)
	}

	return measurementdb.Wiremap{
		Connection: port.PseudoName.Name,
		Instruments: []measurementdb.Instrument{
			{
				Name:         string(cp.InstrumentName),
				ChannelGroup: cp.ChannelGroup,
				Channel:      cp.Channel,
			},
		},
	}, nil
}

func buildGetterInfo(
	getters falconcore.Ports,
	ports *config.ConnectedPorts,
) ([]measurementdb.GetterInfo, error) {
	out := make(
		[]measurementdb.GetterInfo,
		0,
		len(getters),
	)

	for _, getter := range getters {
		wiremap, err := buildWiremap(
			getter,
			ports,
		)
		if err != nil {
			return nil, err
		}

		out = append(
			out,
			measurementdb.GetterInfo{
				Wiremap: wiremap,
				Unit:    getter.Unit,
			},
		)
	}

	return out, nil
}

func buildSweepInfo(
	waveform falconcore.Waveform,
	ports *config.ConnectedPorts,
) ([]measurementdb.SweepInfo, error) {
	var out []measurementdb.SweepInfo

	for axis, coupled := range waveform.Space.Axes {

		arrays, err := waveform.Space.Space.CreateArray(
			falconcore.AxesInt{axis},
		)
		if err != nil {
			return nil, err
		}

		if len(arrays) == 0 {
			return nil, fmt.Errorf(
				"axis %d contains no points",
				axis,
			)
		}

		points := len(arrays[0].Data)

		for _, labelled := range coupled {

			wiremap, err := buildWiremap(
				labelled.InstrumentPort,
				ports,
			)
			if err != nil {
				return nil, err
			}

			out = append(
				out,
				measurementdb.SweepInfo{
					Wiremap: wiremap,
					Axis:    axis,
					Points:  points,
					Start: falconcore.Quantity{
						Value: labelled.Domain.LesserBound,
						Unit:  labelled.InstrumentPort.Unit,
					},
					End: falconcore.Quantity{
						Value: labelled.Domain.GreaterBound,
						Unit:  labelled.InstrumentPort.Unit,
					},
				},
			)
		}
	}

	return out, nil
}

func generateID(measurementID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(measurementID))
	return int(h.Sum32())
}

func (h *CartesianMeasurementHandler) BuildInsertEntry(
	req *MeasurementRequest,
	ports *config.ConnectedPorts,
	shape Shape,
	measurementID string,
) (measurementdb.InsertEntry, error) {
	if len(req.Waveforms) == 0 {
		return measurementdb.InsertEntry{}, fmt.Errorf(
			"measurement request contains no waveforms",
		)
	}

	waveform := req.Waveforms[0]

	getters, err := buildGetterInfo(
		req.Getters,
		ports,
	)
	if err != nil {
		return measurementdb.InsertEntry{}, err
	}

	setters, err := buildSweepInfo(
		waveform,
		ports,
	)
	if err != nil {
		return measurementdb.InsertEntry{}, err
	}

	outShape := make([]int, len(shape))
	for i, v := range shape {
		outShape[i] = int(v)
	}

	return measurementdb.InsertEntry{
		CoreEntry: measurementdb.CoreEntry{
			RunID: measurementID,
			MeasurementID: strconv.Itoa(
				generateID(measurementID),
			),
			MeasurementName: h.Name(),
		},
		Setters: setters,
		Getters: getters,
		Shape:   outShape,
	}, nil
}

func buildRanges(
	getters falconcore.Ports,
	output PrimitiveMeasurementOutput,
	ports *config.ConnectedPorts,
	shape Shape,
) (falconcore.LabelledMeasuredArrays, error) {
	_, getterPorts, err := buildGetters(
		getters,
		ports,
	)
	if err != nil {
		return nil, err
	}

	out := make(
		falconcore.LabelledMeasuredArrays,
		0,
		len(output.Getters),
	)

	arrayShape := make([]int, len(shape))
	for i, d := range shape {
		arrayShape[i] = int(d)
	}

	for target, rawData := range output.Getters {
		port, ok := getterPorts[target]
		if !ok {
			return nil, fmt.Errorf(
				"missing port mapping for target %+v",
				target,
			)
		}
		expectedPoints := 1
		for _, d := range shape {
			expectedPoints *= int(d)
		}

		if len(rawData) != expectedPoints {
			return nil, fmt.Errorf(
				"target %+v contains %d points but shape %v requires %d",
				target,
				len(rawData),
				shape,
				expectedPoints,
			)
		}

		out = append(
			out,
			falconcore.LabelledMeasuredArray{
				AcquisitionContext: falconcore.AcquisitionContext{
					Connection:     port.PseudoName,
					Unit:           port.Unit,
					InstrumentType: port.InstrumentType,
				},
				MeasuredArray: falconcore.MeasuredArray{
					Data:  rawData,
					Shape: arrayShape,
				},
			},
		)
	}

	return out, nil
}

func buildMetadata(
	voltages map[falconcore.InstrumentPort]falconcore.Quantity,
) falconcore.Metadata {
	out := make(
		falconcore.Metadata,
	)

	for port, value := range voltages {
		unitsymbol, err := value.Unit.NewSymbolUnit()
		if err != nil {
			return falconcore.Metadata{}
		}

		unit, err := unitsymbol.Symbol()
		if err != nil {
			return falconcore.Metadata{}
		}

		out["voltage."+string(port.PseudoName.Name)] = fmt.Sprintf(
			"%g %s",
			value.Value,
			unit,
		)
	}

	return out
}

func (h *CartesianMeasurementHandler) BuildDatabaseEntry(
	req *MeasurementRequest,
	output PrimitiveMeasurementOutput,
	shape Shape,
	ports *config.ConnectedPorts,
	voltages map[falconcore.InstrumentPort]falconcore.Quantity,
	measurementID string,
) (falconcore.DatabaseEntry, error) {
	if len(req.Waveforms) == 0 {
		return falconcore.DatabaseEntry{}, fmt.Errorf(
			"measurement request contains no waveforms",
		)
	}

	waveform := req.Waveforms[0]

	ranges, err := buildRanges(
		req.Getters,
		output,
		ports,
		shape,
	)
	if err != nil {
		return falconcore.DatabaseEntry{}, err
	}
	axes := make(
		falconcore.AxesInt,
		0,
		len(shape),
	)

	for i := range shape {
		axes = append(axes, i)
	}
	if err != nil {
		return falconcore.DatabaseEntry{}, err
	}
	unitDomain, err := waveform.Space.Space.CreateArray(axes)
	if err != nil {
		return falconcore.DatabaseEntry{}, err
	}
	dbShape := make(falconcore.AxesInt, len(shape))
	for i, v := range shape {
		dbShape[i] = int(v)
	}

	return falconcore.DatabaseEntry{
		Shape:        dbShape,
		UnitDomain:   unitDomain,
		DomainLabels: waveform.Space.Axes,
		Ranges:       ranges,
		Metadata:     falconcore.Metadata{},
		Title:        h.Name(),
		Id:           generateID(measurementID),
		Timestamp:    int(time.Now().Unix()),
	}, nil
}

func BuildMeasurementResponse(
	ranges falconcore.LabelledMeasuredArrays,
	measurementID string,
) MeasurementResponse {
	message := fmt.Sprintf(
		"Measurement %s completed successfully",
		measurementID,
	)

	return MeasurementResponse{
		Arrays:  ranges,
		Message: message,
	}
}
