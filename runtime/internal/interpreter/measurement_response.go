//go:build cgo

package interpreter

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/autotuner-interfaces/contexts/acquisitioncontext"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listlabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"
)

// newScalarMeasurementResponse attributes one scalar result to the original
// request port. Keeping the original port preserves its full Falcon identity.
func newScalarMeasurementResponse(
	value float64,
	port *instrumentport.Handle,
) (*FalconMeasurementResponse, error) {
	context, err := acquisitioncontext.NewFromPort(port)
	if err != nil {
		return nil, fmt.Errorf(
			"create acquisition context from getter: %w",
			err,
		)
	}
	defer context.Close()

	data, err := farraydouble.FromData([]float64{value}, []uint64{1})
	if err != nil {
		return nil, fmt.Errorf("create scalar measurement data: %w", err)
	}
	defer data.Close()

	array, err := labelledmeasuredarray.FromFArray(data, context)
	if err != nil {
		return nil, fmt.Errorf("label scalar measurement data: %w", err)
	}
	defer array.Close()

	list, err := listlabelledmeasuredarray.New(
		[]*labelledmeasuredarray.Handle{array},
	)
	if err != nil {
		return nil, fmt.Errorf("create measurement array list: %w", err)
	}
	defer list.Close()

	arrays, err := labelledarrayslabelledmeasuredarray.NewFromList(list)
	if err != nil {
		return nil, fmt.Errorf("create labelled measurement arrays: %w", err)
	}
	defer arrays.Close()

	response, err := measurementresponse.New(arrays)
	if err != nil {
		return nil, fmt.Errorf("create MeasurementResponse: %w", err)
	}

	return &FalconMeasurementResponse{handle: response}, nil
}
