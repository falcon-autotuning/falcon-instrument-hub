//go:build cgo

package measurementdb

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/hdf5data"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

type HDF5Store struct{}

func NewHDF5Store() *HDF5Store {
	return &HDF5Store{}
}

func (s *HDF5Store) Write(
	path string,
	entry falconcore.DatabaseEntry,
) error {
	data, err := entry.NewFalconDatabaseEntry()
	if err != nil {
		return err
	}
	defer data.Close()

	return data.ToFile(path)
}

func (s *HDF5Store) Read(
	path string,
) (falconcore.DatabaseEntry, error) {
	data, err := hdf5data.NewFromFile(path)
	if err != nil {
		return falconcore.DatabaseEntry{}, err
	}
	defer data.Close()

	return falconcore.FalconDatabaseEntryFromFalcon(data)
}
