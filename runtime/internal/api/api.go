package api

// Status: Provide the status of the process
type Status struct {
	Status    bool  `yaml:"status" json:"status"`       // At compilation of this message the state of the process
	Timestamp int64 `yaml:"timestamp" json:"timestamp"` // When the response was completed
}

// PortRequest: Request all current instrument ports
type PortRequest struct {
	Timestamp int64 `yaml:"timestamp" json:"timestamp"` // When the response was completed
}

// PortPayload: All of the current instrument ports
type PortPayload struct {
	Timestamp int64  `yaml:"timestamp" json:"timestamp"` // When the response was completed
	Knobs     string `yaml:"knobs" json:"knobs"`         // All of the knobs attached to the instrument server
	Meters    string `yaml:"meters" json:"meters"`       // All of the meters attached to the instrument server
	Settings  string `yaml:"settings" json:"settings"`   // All of the settings attached to the instrument server
}

// DeviceStateRequest: A request for the device state
type DeviceStateRequest struct {
	Timestamp int64 `yaml:"timestamp" json:"timestamp"` // When the response was completed
}

// DeviceStateResponse: A response containing the device state
type DeviceStateResponse struct {
	Timestamp int64  `yaml:"timestamp" json:"timestamp"` // When the response was completed
	Response  string `yaml:"response" json:"response"`   // The device state for use understanding the current applied voltages
}

// DeviceConfigRequest: A request for the device configuration
type DeviceConfigRequest struct {
	Timestamp int64 `yaml:"timestamp" json:"timestamp"` // When the response was completed
}

// DeviceConfigResponse: A response containing the device configuration
type DeviceConfigResponse struct {
	Timestamp int64  `yaml:"timestamp" json:"timestamp"` // When the response was completed
	Response  string `yaml:"response" json:"response"`   // The device config for use understanding the device layout
}

// MeasureCommand: issued to runtime to request a measurement from the instrument server
type MeasureCommand struct {
	Request   string `yaml:"request" json:"request"`     // the measurement request to be taken
	Timestamp int64  `yaml:"timestamp" json:"timestamp"` // Correlates this command with its response
	Hash      int64  `yaml:"hash" json:"hash"`           // the hash for the requesting unit
}

// MeasureResponse: Recieve a response from the runtime as to the measurement performed
type MeasureResponse struct {
	Stream    string `yaml:"stream" json:"stream"`       // the MeasurementResponse JSON from the hub
	Channel   string `yaml:"channel" json:"channel"`     // channel identifier
	Timestamp int64  `yaml:"timestamp" json:"timestamp"` // Correlates this response with its command
	Hash      int64  `yaml:"hash" json:"hash"`           // correlation hash from the MeasureCommand
	Response  string `yaml:"response" json:"response"`   // uploaded data from the runtime
}

// SettingCommand: issued to runtime to request a setting from the instrument server
type SettingCommand struct {
	Request   string `yaml:"request" json:"request"`     // the measurement request to be taken
	Timestamp int64  `yaml:"timestamp" json:"timestamp"` // Correlates this command with its response
	Hash      int64  `yaml:"hash" json:"hash"`           // the hash for the requesting unit
}

// SettingResponse: Recieve a response from the runtime as to the setting performed
type SettingResponse struct {
	Timestamp int64  `yaml:"timestamp" json:"timestamp"` // Correlates this response with its command
	Hash      int64  `yaml:"hash" json:"hash"`           // correlation hash from the MeasureCommand
	Response  string `yaml:"response" json:"response"`   // uploaded data from the runtime
}

// CommandRegistry maps command names to empty struct instances
var CommandRegistry = map[string]any{
	"STATUS":                 Status{},
	"PORT_REQUEST":           PortRequest{},
	"PORT_PAYLOAD":           PortPayload{},
	"DEVICE_STATE_REQUEST":   DeviceStateRequest{},
	"DEVICE_STATE_RESPONSE":  DeviceStateResponse{},
	"DEVICE_CONFIG_REQUEST":  DeviceConfigRequest{},
	"DEVICE_CONFIG_RESPONSE": DeviceConfigResponse{},
	"MEASURE_COMMAND":        MeasureCommand{},
	"MEASURE_RESPONSE":       MeasureResponse{},
	"SETTING_COMMAND":        SettingCommand{},
	"SETTING_RESPONSE":       SettingResponse{},
}
