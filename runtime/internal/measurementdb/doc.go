// Package measurementdb stores Falcon measurements as HDF5 files while
// maintaining a searchable SQLite catalog.
//
// # Architecture
//
// Measurement data is split into two storage layers:
//
//   - HDF5 files contain complete Falcon DatabaseEntry objects,
//     including sweep axes, domains, acquisition contexts,
//     measured ranges, metadata, and all other measurement data.
//
//   - SQLite stores a lightweight catalog of searchable metadata,
//     allowing measurements to be found without loading HDF5 files.
//
// Measurements are automatically organized into a date-based directory
// hierarchy:
//
//	hdf5/
//	└── YYYY/
//	    └── MM/
//	        └── DD/
//	            └── measurement-name-<unique-id>.h5
//
// New directories are created automatically as data is written. The
// storage location is determined from the measurement timestamp, allowing
// measurements to naturally roll over between days, months, and years.
//
// # Startup
//
// The package is typically initialized once during process startup:
//
//	err := measurementdb.Startup(
//		"./catalog.sqlite",
//		"./hdf5",
//		"Falcon Instrument Hub",
//	)
//	if err != nil {
//		return err
//	}
//	defer measurementdb.Close()
//
// Startup automatically validates the database catalog against the HDF5
// store and logs any inconsistencies that are detected.
//
// # Writing Measurements
//
// Measurements are added by storing a searchable CatalogEntry alongside
// a complete Falcon DatabaseEntry:
//
//	err := measurementdb.Insert(
//		measurementdb.CatalogEntry{
//			RunID:           "run-001",
//			MeasurementID:   "measurement-001",
//			MeasurementName: "Charge Stability Diagram",
//			Getter:          "Current",
//			Unit:            "A",
//			SampleCount:     2048,
//		},
//		entry,
//	)
//
// # Reading Measurements
//
// Measurements may be searched by metadata without loading HDF5 files:
//
//	results, err := measurementdb.Search(
//		measurementdb.SearchQuery{
//			RunID: "run-001",
//		},
//	)
package measurementdb
