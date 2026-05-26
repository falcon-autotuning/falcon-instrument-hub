package serverinterpreter

// navJSON navigates a JSON structure using a sequence of string keys and int indices.
// Returns nil if any step fails or the path does not exist.
func navJSON(obj interface{}, keys ...interface{}) interface{} {
	for _, k := range keys {
		if obj == nil {
			return nil
		}
		switch key := k.(type) {
		case string:
			m, ok := obj.(map[string]interface{})
			if !ok {
				return nil
			}
			obj = m[key]
		case int:
			arr, ok := obj.([]interface{})
			if !ok || key >= len(arr) {
				return nil
			}
			obj = arr[key]
		}
	}
	return obj
}

// convertToFloat64 converts an interface{} to float64.
func convertToFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

// stubWaveformData returns a minimal placeholder WaveformData for error paths.
func stubWaveformData() *WaveformData {
	return &WaveformData{
		RawTimeTrace: [][]float64{{0.0}},
		AxisDomains:  [][]LabelledDomainInfo{},
		TimeDomain:   DomainBounds{Min: 0, Max: 0.001},
		Shape:        []int{1},
	}
}
