package serverinterpreter

// ISSCallResult is an instrument call returned by an ISS measurement job.
type ISSCallResult struct {
	Index        int            `json:"index"`
	Instrument   string         `json:"instrument"`
	Verb         string         `json:"verb"`
	ExecutedAtMs int64          `json:"executed_at_ms"`
	Return       ISSReturnValue `json:"return"`
}

// ISSReturnValue represents a scalar, buffer reference, or void result.
type ISSReturnValue struct {
	Type         string      `json:"type"`
	Value        interface{} `json:"value,omitempty"`
	BufferID     string      `json:"buffer_id,omitempty"`
	ElementCount int         `json:"element_count,omitempty"`
	DataType     string      `json:"data_type,omitempty"`
}
