package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/hdf5data"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringstring"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescontrolarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescoupledlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesint"
)

type DatabaseEntry struct {
	Shape        AxesInt
	UnitDomain   AxesControlArray
	DomainLabels AxesCoupledLabelledDomain
	Ranges       LabelledMeasuredArrays
	Metadata     Metadata
	Title        string
	Id           int
	Timestamp    int
}

func (d DatabaseEntry) NewFalconDatabaseEntry() (*hdf5data.Handle, error) {
	shape, err := d.Shape.NewFalconAxesInt()
	if err != nil {
		return nil, err
	}
	defer shape.Close()
	dom, err := d.UnitDomain.NewFalconAxesControlArray()
	if err != nil {
		return nil, err
	}
	defer dom.Close()
	labels, err := d.DomainLabels.NewFalconAxesCoupledLabelledDomain()
	if err != nil {
		return nil, err
	}
	defer labels.Close()
	ranges, err := d.Ranges.NewFalconLabelledMeasuredArrays()
	if err != nil {
		return nil, err
	}
	defer ranges.Close()
	meta, err := d.Metadata.NewFalconMetadata()
	if err != nil {
		return nil, err
	}
	defer meta.Close()
	return hdf5data.New(shape, dom, labels, ranges, meta, d.Title, int32(d.Id), int32(d.Timestamp))
}

type FalconDatabaseReader interface {
	Shape() (*axesint.Handle, error)
	UnitDomain() (*axescontrolarray.Handle, error)
	DomainLabels() (*axescoupledlabelleddomain.Handle, error)
	Ranges() (*labelledarrayslabelledmeasuredarray.Handle, error)
	Metadata() (*mapstringstring.Handle, error)
	MeasurementTitle() (string, error)
	UniqueId() (int32, error)
	Timestamp() (int32, error)
}

var _ FalconDatabaseReader = (*hdf5data.Handle)(nil)

func FalconDatabaseEntryFromFalcon(d *hdf5data.Handle) (DatabaseEntry, error) {
	var out DatabaseEntry
	shape, err := d.Shape()
	if err != nil {
		return out, err
	}
	defer shape.Close()
	out.Shape, err = AxesIntFromFalcon(shape)
	if err != nil {
		return out, err
	}
	dom, err := d.UnitDomain()
	if err != nil {
		return out, err
	}
	defer dom.Close()
	out.UnitDomain, err = AxesControlArrayFromFalcon(dom)
	if err != nil {
		return out, err
	}
	labels, err := d.DomainLabels()
	if err != nil {
		return out, err
	}
	defer labels.Close()
	out.DomainLabels, err = AxesCoupledLabelledDomainFromFalcon(labels)
	if err != nil {
		return out, err
	}
	ranges, err := d.Ranges()
	if err != nil {
		return out, err
	}
	defer ranges.Close()
	out.Ranges, err = LabelledMeasuredArraysFromFalcon(ranges)
	if err != nil {
		return out, err
	}
	meta, err := d.Metadata()
	if err != nil {
		return out, err
	}
	defer meta.Close()
	out.Metadata, err = MetadataFromFalcon(meta)
	if err != nil {
		return out, err
	}
	out.Title, err = d.MeasurementTitle()
	if err != nil {
		return out, err
	}
	id, err := d.UniqueId()
	if err != nil {
		return out, err
	}
	out.Id = int(id)
	time, err := d.Timestamp()
	if err != nil {
		return out, err
	}
	out.Timestamp = int(time)
	return out, nil
}
