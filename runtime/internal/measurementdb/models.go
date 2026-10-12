package measurementdb

import (
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

type CoreEntry struct {
	RunID           string
	MeasurementID   string
	MeasurementName string
}

type Instrument struct {
	Name         string `json:"name"`
	ChannelGroup string `json:"channel_group"`
	Channel      int    `json:"channel"`
}
type Wiremap struct {
	Connection  falconcore.ConnectionName `json:"connection"`
	Instruments []Instrument              `json:"instruments"`
}
type SweepInfo struct {
	Wiremap Wiremap `json:"wiremap"`

	Axis   int `json:"axis"`
	Points int `json:"points"`

	Start falconcore.Quantity `json:"start"`
	End   falconcore.Quantity `json:"end"`

	ACAmplitude falconcore.Quantity `json:"ac_amplitude"`
	ACFrequency falconcore.Quantity `json:"ac_frequency"`
}
type GetterInfo struct {
	Wiremap Wiremap         `json:"wiremap"`
	Unit    falconcore.Unit `json:"unit"`
}

type InsertEntry struct {
	CoreEntry

	Setters []SweepInfo  `json:"setters"`
	Getters []GetterInfo `json:"getters"`
	Shape   []int        `json:"shape"`
}

type CatalogEntry struct {
	InsertEntry

	// The personnel responsible for collecting the data
	Owner      string    `json:"owner"`
	FilePath   string    `json:"file_path"`
	RecordedAt time.Time `json:"recorded_at"`

	NumPoints int `json:"num_points"`
}

type SearchQuery struct {
	CoreEntry

	// The personnel responsible for collecting the data
	Owner string

	StartTime *time.Time
	EndTime   *time.Time

	Limit int
}

type ValidationReport struct {
	MissingFiles []CatalogEntry
	OrphanFiles  []string
	BadFiles     []string
}

type DatabaseEntry = falconcore.DatabaseEntry
