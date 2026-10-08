package api

import "time"

// TimestampConverter interface for types that have a Timestamp field
type TimestampConverter interface {
	GetTimestamp() int64
}

// TimestampSetter interface for types that can have their timestamp set
type TimestampSetter interface {
	SetTimestamp(timestamp int64)
}

// Implement TimestampConverter for all API types with Timestamp
func (s Status) GetTimestamp() int64               { return s.Timestamp }
func (p PortRequest) GetTimestamp() int64          { return p.Timestamp }
func (p PortPayload) GetTimestamp() int64          { return p.Timestamp }
func (d DeviceConfigRequest) GetTimestamp() int64  { return d.Timestamp }
func (d DeviceConfigResponse) GetTimestamp() int64 { return d.Timestamp }
func (d DeviceStateRequest) GetTimestamp() int64   { return d.Timestamp }
func (d DeviceStateResponse) GetTimestamp() int64  { return d.Timestamp }
func (d MeasureCommand) GetTimestamp() int64       { return d.Timestamp }
func (d MeasureResponse) GetTimestamp() int64      { return d.Timestamp }
func (d SettingCommand) GetTimestamp() int64       { return d.Timestamp }
func (d SettingResponse) GetTimestamp() int64      { return d.Timestamp }

// Implement TimestampSetter for all API types with Timestamp (pointer receivers
// for mutation)
func (s *Status) SetTimestamp(
	timestamp int64,
) {
	s.Timestamp = timestamp
}

func (p *PortRequest) SetTimestamp(
	timestamp int64,
) {
	p.Timestamp = timestamp
}

func (p *PortPayload) SetTimestamp(
	timestamp int64,
) {
	p.Timestamp = timestamp
}

func (d *DeviceConfigRequest) SetTimestamp(
	timestamp int64,
) {
	d.Timestamp = timestamp
}

func (d *DeviceConfigResponse) SetTimestamp(
	timestamp int64,
) {
	d.Timestamp = timestamp
}

func (d *DeviceStateRequest) SetTimestamp(
	timestamp int64,
) {
	d.Timestamp = timestamp
}

func (d *DeviceStateResponse) SetTimestamp(
	timestamp int64,
) {
	d.Timestamp = timestamp
}

func (d *MeasureCommand) SetTimestamp(
	timestamp int64,
) {
	d.Timestamp = timestamp
}

func (d *MeasureResponse) SetTimestamp(
	timestamp int64,
) {
	d.Timestamp = timestamp
}

func (d *SettingCommand) SetTimestamp(
	timestamp int64,
) {
	d.Timestamp = timestamp
}

func (d *SettingResponse) SetTimestamp(
	timestamp int64,
) {
	d.Timestamp = timestamp
}

// ToTime converts any timestamp to a Go time.Time
func ToTime[T TimestampConverter](t T) time.Time {
	timestamp := t.GetTimestamp()
	if timestamp == 0 {
		return time.Now()
	}
	// Convert microseconds to time.Time
	return time.Unix(
		0,
		int64(timestamp)*1000,
	) // Convert microseconds to nanoseconds
}

// SetCurrentTimestamp sets the timestamp to current time in microseconds for
// any TimestampSetter
func SetCurrentTimestamp[T TimestampSetter](t T) {
	timestamp := int64(time.Now().UnixMicro())
	t.SetTimestamp(timestamp)
}
