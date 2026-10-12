package measurementdb

import (
	"fmt"
	"log"
	"sync"
)

var (
	globalMu      sync.RWMutex
	globalManager *Manager
)

func Startup(
	catalogPath string,
	hdf5Directory string,
	ownerName string,
) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalManager != nil {
		return fmt.Errorf(
			"measurement database already started",
		)
	}

	manager, err := Open(
		catalogPath,
		hdf5Directory,
		ownerName,
	)
	if err != nil {
		return err
	}

	report, err := manager.Validate()
	if err != nil {
		_ = manager.Close()
		return err
	}

	globalManager = manager

	if !report.Healthy() {
		log.Printf(
			"measurement database inconsistencies found: "+
				"%d missing, %d orphaned, %d unreadable",
			len(report.MissingFiles),
			len(report.OrphanFiles),
			len(report.BadFiles),
		)
	}

	return nil
}

func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalManager == nil {
		return nil
	}

	err := globalManager.Close()
	globalManager = nil

	return err
}

func Instance() (*Manager, error) {
	globalMu.RLock()
	defer globalMu.RUnlock()

	if globalManager == nil {
		return nil, fmt.Errorf(
			"measurement database not started",
		)
	}

	return globalManager, nil
}

func Search(
	query SearchQuery,
) ([]CatalogEntry, error) {
	manager, err := Instance()
	if err != nil {
		return nil, err
	}

	return manager.Search(query)
}

func Insert(
	insert InsertEntry,
	entry DatabaseEntry,
) (CatalogEntry, error) {
	manager, err := Instance()
	if err != nil {
		return CatalogEntry{}, err
	}

	return manager.Insert(insert, entry)
}

func Validate() (*ValidationReport, error) {
	manager, err := Instance()
	if err != nil {
		return nil, err
	}

	return manager.Validate()
}
