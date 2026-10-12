// Package measurementdb stores Falcon measurements as HDF5 files while
// maintaining a searchable SQLite catalog.
//
// # Architecture
//
// Measurement data is split into two storage layers:
//
//   - HDF5 files contain complete Falcon DatabaseEntry objects,
//     including sweep definitions, acquisition data, measured values,
//     device state snapshots, metadata, and all other measurement data.
//
//   - SQLite stores a lightweight searchable catalog, allowing
//     measurements to be discovered without loading HDF5 files.
//
// Measurements are automatically organized into a date-based directory
// hierarchy:
//
//	hdf5/
//	└── YYYY/
//	    └── MM/
//	        └── DD/
//	            └── measurement-name-<measurement-id>.h5
//
// New directories are created automatically as data is written. The
// storage location is determined from the measurement timestamp, allowing
// measurements to naturally roll over between days, months, and years.
//
// # Identifiers
//
// Every stored measurement belongs to a Run and has two identifiers:
//
//   - RunID identifies a logical measurement workflow or experiment.
//     A single run may generate many stored measurements.
//
//   - MeasurementID uniquely identifies a single catalog record and
//     HDF5 file.
//
// UUIDs are recommended for both identifiers to ensure uniqueness across
// processes, machines, and database instances.
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
// Startup automatically validates the SQLite catalog against the HDF5
// store and reports any inconsistencies that are detected.
//
// # Writing Measurements
//
// Measurements are written by storing a searchable catalog entry
// alongside a complete Falcon DatabaseEntry:
//
//	catalog, err := measurementdb.Insert(
//		measurementdb.InsertEntry{
//			RunID:           uuid.NewString(),
//			MeasurementName: "Charge Stability Diagram",
//		},
//		entry,
//	)
//
// The returned CatalogEntry contains the final persisted metadata,
// including the generated file location and measurement identifiers.
//
// # Searching Measurements
//
// Measurements may be searched by catalog metadata without loading
// HDF5 files:
//
//	results, err := measurementdb.Search(
//		measurementdb.SearchQuery{
//			RunID: runID,
//		},
//	)
package measurementdb
