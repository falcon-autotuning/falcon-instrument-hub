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
func (l Log) GetTimestamp() int64                     { return l.Timestamp }
func (m MeasurementReady) GetTimestamp() int64        { return m.Timestamp }
func (p ProcessData) GetTimestamp() int64             { return p.Timestamp }
func (s Status) GetTimestamp() int64                  { return s.Timestamp }
func (u UpdateDaemonProperty) GetTimestamp() int64    { return u.Timestamp }
func (u UploadData) GetTimestamp() int64              { return u.Timestamp }
func (c ConfirmInitialization) GetTimestamp() int64   { return c.Timestamp }
func (p PerformArbitraryMethod) GetTimestamp() int64  { return p.Timestamp }
func (r ReturnGet) GetTimestamp() int64               { return r.Timestamp }
func (s SetupInstrument) GetTimestamp() int64         { return s.Timestamp }
func (d DestroyInstrument) GetTimestamp() int64       { return d.Timestamp }
func (p PerformInstrumentMethod) GetTimestamp() int64 { return p.Timestamp }
func (b Busy) GetTimestamp() int64                    { return b.Timestamp }
func (p PortRequest) GetTimestamp() int64             { return p.Timestamp }
func (p PortPayload) GetTimestamp() int64             { return p.Timestamp }
func (d DeviceConfigRequest) GetTimestamp() int64     { return d.Timestamp }
func (d DeviceConfigResponse) GetTimestamp() int64    { return d.Timestamp }

// Implement TimestampSetter for all API types with Timestamp (pointer receivers for mutation)
func (l *Log) SetTimestamp(timestamp int64)                     { l.Timestamp = timestamp }
func (m *MeasurementReady) SetTimestamp(timestamp int64)        { m.Timestamp = timestamp }
func (p *ProcessData) SetTimestamp(timestamp int64)             { p.Timestamp = timestamp }
func (s *Status) SetTimestamp(timestamp int64)                  { s.Timestamp = timestamp }
func (u *UpdateDaemonProperty) SetTimestamp(timestamp int64)    { u.Timestamp = timestamp }
func (u *UploadData) SetTimestamp(timestamp int64)              { u.Timestamp = timestamp }
func (c *ConfirmInitialization) SetTimestamp(timestamp int64)   { c.Timestamp = timestamp }
func (p *PerformArbitraryMethod) SetTimestamp(timestamp int64)  { p.Timestamp = timestamp }
func (r *ReturnGet) SetTimestamp(timestamp int64)               { r.Timestamp = timestamp }
func (s *SetupInstrument) SetTimestamp(timestamp int64)         { s.Timestamp = timestamp }
func (d *DestroyInstrument) SetTimestamp(timestamp int64)       { d.Timestamp = timestamp }
func (p *PerformInstrumentMethod) SetTimestamp(timestamp int64) { p.Timestamp = timestamp }
func (b *Busy) SetTimestamp(timestamp int64)                    { b.Timestamp = timestamp }
func (p *PortRequest) SetTimestamp(timestamp int64)             { p.Timestamp = timestamp }
func (p *PortPayload) SetTimestamp(timestamp int64)             { p.Timestamp = timestamp }
func (d *DeviceConfigRequest) SetTimestamp(timestamp int64)     { d.Timestamp = timestamp }
func (d *DeviceConfigResponse) SetTimestamp(timestamp int64)    { d.Timestamp = timestamp }

// ToTime converts any timestamp to a Go time.Time
func ToTime[T TimestampConverter](t T) time.Time {
	timestamp := t.GetTimestamp()
	if timestamp == 0 {
		return time.Now()
	}
	// Convert microseconds to time.Time
	return time.Unix(0, int64(timestamp)*1000) // Convert microseconds to nanoseconds
}

// SetCurrentTimestamp sets the timestamp to current time in microseconds for any TimestampSetter
func SetCurrentTimestamp[T TimestampSetter](t T) {
	timestamp := int64(time.Now().UnixMicro())
	t.SetTimestamp(timestamp)
}
