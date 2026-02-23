// Package serverinterpreter provides JSON schema validation for falcon measurement requests.
//
// Incoming measurement requests from falcon-core are validated against the canonical
// JSON schemas defined in falcon-measurement-lib/schemas/scripts/.
// This ensures that malformed or incomplete requests are rejected early with clear
// error messages before reaching the orchestration layer.
package serverinterpreter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SchemaValidationError represents one or more validation failures.
type SchemaValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e SchemaValidationError) Error() string {
	return fmt.Sprintf("validation error on field %q: %s", e.Field, e.Message)
}

// SchemaValidationResult accumulates validation errors.
type SchemaValidationResult struct {
	Errors []SchemaValidationError `json:"errors,omitempty"`
}

// OK returns true if there are no validation errors.
func (r *SchemaValidationResult) OK() bool {
	return len(r.Errors) == 0
}

// Error returns a combined error string or empty string if valid.
func (r *SchemaValidationResult) Error() string {
	if r.OK() {
		return ""
	}
	msgs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// addError appends a validation error.
func (r *SchemaValidationResult) addError(field, message string) {
	r.Errors = append(r.Errors, SchemaValidationError{Field: field, Message: message})
}

// ValidateInstrumentTarget checks that an InstrumentTarget has a non-empty ID.
func ValidateInstrumentTarget(target FalconInstrumentTarget, fieldPrefix string) []SchemaValidationError {
	var errs []SchemaValidationError
	if target.ID == "" {
		errs = append(errs, SchemaValidationError{
			Field:   fieldPrefix + ".id",
			Message: "instrument target id must not be empty",
		})
	}
	return errs
}

// ValidateDomain checks that a Domain has min < max.
func ValidateDomain(domain FalconDomain, fieldName string) []SchemaValidationError {
	var errs []SchemaValidationError
	if domain.Min >= domain.Max {
		errs = append(errs, SchemaValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("domain min (%.6f) must be less than max (%.6f)", domain.Min, domain.Max),
		})
	}
	return errs
}

// ===========================================================================
// Per-schema validators (aligned with falcon-measurement-lib schemas)
// ===========================================================================

// Validate1DBufferedRequest validates a measure_1D_buffered request against
// the falcon-measurement-lib/schemas/scripts/measure_1D_buffered.json schema.
func Validate1DBufferedRequest(req FalconMeasure1DBufferedRequest) *SchemaValidationResult {
	result := &SchemaValidationResult{}

	// Required: bufferedGetters (non-empty array)
	if len(req.BufferedGetters) == 0 {
		result.addError("bufferedGetters", "must contain at least one getter")
	}
	for i, g := range req.BufferedGetters {
		for _, e := range ValidateInstrumentTarget(g, fmt.Sprintf("bufferedGetters[%d]", i)) {
			result.Errors = append(result.Errors, e)
		}
	}

	// bufferedSetters: must have at least one for a sweep
	if len(req.BufferedSetters) == 0 {
		result.addError("bufferedSetters", "must contain at least one setter for a 1D sweep")
	}
	for i, s := range req.BufferedSetters {
		for _, e := range ValidateInstrumentTarget(s, fmt.Sprintf("bufferedSetters[%d]", i)) {
			result.Errors = append(result.Errors, e)
		}
	}

	// setVoltageDomains: every bufferedSetter must have a matching domain
	for _, setter := range req.BufferedSetters {
		key := setter.Serialize()
		if _, ok := req.SetVoltageDomains[key]; !ok {
			result.addError("setVoltageDomains",
				fmt.Sprintf("missing voltage domain for setter %s", key))
		}
	}
	for key, dom := range req.SetVoltageDomains {
		for _, e := range ValidateDomain(dom, fmt.Sprintf("setVoltageDomains[%s]", key)) {
			result.Errors = append(result.Errors, e)
		}
	}

	// sampleRate: must be positive
	if req.SampleRate <= 0 {
		result.addError("sampleRate", "must be a positive number")
	}

	// numPoints: must be positive
	if req.NumPoints <= 0 {
		result.addError("numPoints", "must be a positive integer")
	}

	// numSteps: must be >= 2 for a sweep
	if req.NumSteps < 2 {
		result.addError("numSteps", "must be at least 2 for a sweep")
	}

	return result
}

// Validate2DBufferedRequest validates a measure_2D_buffered request against
// the falcon-measurement-lib/schemas/scripts/measure_2D_buffered.json schema.
func Validate2DBufferedRequest(req FalconMeasure2DBufferedRequest) *SchemaValidationResult {
	result := &SchemaValidationResult{}

	// Required: bufferedGetters (non-empty array)
	if len(req.BufferedGetters) == 0 {
		result.addError("bufferedGetters", "must contain at least one getter")
	}
	for i, g := range req.BufferedGetters {
		for _, e := range ValidateInstrumentTarget(g, fmt.Sprintf("bufferedGetters[%d]", i)) {
			result.Errors = append(result.Errors, e)
		}
	}

	// bufferedXSetters: must have at least one for X axis
	if len(req.BufferedXSetters) == 0 {
		result.addError("bufferedXSetters", "must contain at least one X-axis setter")
	}
	for i, s := range req.BufferedXSetters {
		for _, e := range ValidateInstrumentTarget(s, fmt.Sprintf("bufferedXSetters[%d]", i)) {
			result.Errors = append(result.Errors, e)
		}
	}

	// bufferedYSetters: must have at least one for Y axis
	if len(req.BufferedYSetters) == 0 {
		result.addError("bufferedYSetters", "must contain at least one Y-axis setter")
	}
	for i, s := range req.BufferedYSetters {
		for _, e := range ValidateInstrumentTarget(s, fmt.Sprintf("bufferedYSetters[%d]", i)) {
			result.Errors = append(result.Errors, e)
		}
	}

	// setXVoltageDomains: every X setter must have a matching domain
	for _, setter := range req.BufferedXSetters {
		key := setter.Serialize()
		if _, ok := req.SetXVoltageDomains[key]; !ok {
			result.addError("setXVoltageDomains",
				fmt.Sprintf("missing X voltage domain for setter %s", key))
		}
	}
	for key, dom := range req.SetXVoltageDomains {
		for _, e := range ValidateDomain(dom, fmt.Sprintf("setXVoltageDomains[%s]", key)) {
			result.Errors = append(result.Errors, e)
		}
	}

	// setYVoltageDomains: every Y setter must have a matching domain
	for _, setter := range req.BufferedYSetters {
		key := setter.Serialize()
		if _, ok := req.SetYVoltageDomains[key]; !ok {
			result.addError("setYVoltageDomains",
				fmt.Sprintf("missing Y voltage domain for setter %s", key))
		}
	}
	for key, dom := range req.SetYVoltageDomains {
		for _, e := range ValidateDomain(dom, fmt.Sprintf("setYVoltageDomains[%s]", key)) {
			result.Errors = append(result.Errors, e)
		}
	}

	// sampleRate: must be positive
	if req.SampleRate <= 0 {
		result.addError("sampleRate", "must be a positive number")
	}

	// numPoints: must be positive
	if req.NumPoints <= 0 {
		result.addError("numPoints", "must be a positive integer")
	}

	// numXSteps / numYSteps: must be >= 2
	if req.NumXSteps < 2 {
		result.addError("numXSteps", "must be at least 2 for a sweep")
	}
	if req.NumYSteps < 2 {
		result.addError("numYSteps", "must be at least 2 for a sweep")
	}

	return result
}

// ValidateGetSetRequest validates a measure_get_set request against
// the falcon-measurement-lib/schemas/scripts/measure_get_set.json schema.
func ValidateGetSetRequest(req FalconMeasureGetSetRequest) *SchemaValidationResult {
	result := &SchemaValidationResult{}

	// Must have at least one setter or getter
	if len(req.Setters) == 0 && len(req.Getters) == 0 {
		result.addError("setters/getters", "must have at least one setter or getter")
	}

	for i, s := range req.Setters {
		for _, e := range ValidateInstrumentTarget(s, fmt.Sprintf("setters[%d]", i)) {
			result.Errors = append(result.Errors, e)
		}
	}
	for i, g := range req.Getters {
		for _, e := range ValidateInstrumentTarget(g, fmt.Sprintf("getters[%d]", i)) {
			result.Errors = append(result.Errors, e)
		}
	}

	// Every setter should have a corresponding setVoltage
	for _, setter := range req.Setters {
		key := setter.Serialize()
		if _, ok := req.SetVoltages[key]; !ok {
			result.addError("setVoltages",
				fmt.Sprintf("missing voltage for setter %s", key))
		}
	}

	return result
}

// ValidateEnvelope validates the measurement envelope metadata.
func ValidateEnvelope(env FalconMeasurementEnvelope) *SchemaValidationResult {
	result := &SchemaValidationResult{}

	if env.MeasurementID == "" {
		result.addError("measurementId", "must not be empty")
	}

	if env.MeasurementType == "" {
		result.addError("measurementType", "must not be empty")
	}

	validTypes := map[string]bool{
		"measure_1D_buffered": true,
		"measure_2D_buffered": true,
		"measure_get_set":     true,
	}
	if !validTypes[env.MeasurementType] {
		result.addError("measurementType",
			fmt.Sprintf("unknown type %q; expected one of: measure_1D_buffered, measure_2D_buffered, measure_get_set",
				env.MeasurementType))
	}

	if len(env.Request) == 0 {
		result.addError("request", "must not be empty")
	} else {
		// Verify it's valid JSON
		var raw json.RawMessage
		if err := json.Unmarshal(env.Request, &raw); err != nil {
			result.addError("request", "must be valid JSON")
		}
	}

	return result
}

// ValidateRequest performs full validation of a measurement envelope and its
// payload. It first validates the envelope, then dispatches to the
// type-specific validator.
func ValidateRequest(env FalconMeasurementEnvelope) *SchemaValidationResult {
	// Validate envelope first
	result := ValidateEnvelope(env)
	if !result.OK() {
		return result
	}

	// Dispatch to type-specific validator
	switch env.MeasurementType {
	case "measure_1D_buffered":
		var req FalconMeasure1DBufferedRequest
		if err := json.Unmarshal(env.Request, &req); err != nil {
			result.addError("request", fmt.Sprintf("failed to parse 1D buffered payload: %v", err))
			return result
		}
		payloadResult := Validate1DBufferedRequest(req)
		result.Errors = append(result.Errors, payloadResult.Errors...)

	case "measure_2D_buffered":
		var req FalconMeasure2DBufferedRequest
		if err := json.Unmarshal(env.Request, &req); err != nil {
			result.addError("request", fmt.Sprintf("failed to parse 2D buffered payload: %v", err))
			return result
		}
		payloadResult := Validate2DBufferedRequest(req)
		result.Errors = append(result.Errors, payloadResult.Errors...)

	case "measure_get_set":
		var req FalconMeasureGetSetRequest
		if err := json.Unmarshal(env.Request, &req); err != nil {
			result.addError("request", fmt.Sprintf("failed to parse get/set payload: %v", err))
			return result
		}
		payloadResult := ValidateGetSetRequest(req)
		result.Errors = append(result.Errors, payloadResult.Errors...)
	}

	return result
}
