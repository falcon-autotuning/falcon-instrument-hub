package measure

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
)

// createBoolMap creates a map with all values set to false
func createBoolMap(names []instrument.Name) map[instrument.Name]bool {
	result := make(map[instrument.Name]bool, len(names))
	for _, name := range names {
		result[name] = false
	}
	return result
}

// collectAllSetInstructions collects and validates all setter instructions
func collectAllRequirements(reqs []string) ([]Instructions, error) {
	var out []Instructions
	var errorMsgs []string

	for _, setter := range reqs {
		var instructions Instructions
		err := instructions.fromJson(setter)
		if err == nil {
			out = append(out, instructions)
			continue
		}
		errorMsgs = append(errorMsgs, fmt.Sprintf("setter %q: %v", setter, err))
	}

	if len(errorMsgs) > 0 {
		return out, fmt.Errorf("failed to process some setters: %s",
			strings.Join(errorMsgs, "; "),
		)
	}

	return out, nil
}

// convertToJsonPorts converts string array to JsonPort array
func convertToJsonPorts(strs []string) ([]instrument.JsonPort, error) {
	result := make([]instrument.JsonPort, len(strs))
	var errorMsgs []string

	for i, s := range strs {
		// we need to fix json issue by cycling inputs through Go json to remove
		// spaces
		port := instrument.PortObject{}
		err1 := json.Unmarshal([]byte(s), &port)
		fixed_bytes, err2 := json.Marshal(port)
		result[i] = instrument.JsonPort(string(fixed_bytes))
		if err1 != nil {
			errorMsgs = append(
				errorMsgs,
				fmt.Sprintf(
					"unmarshal error for string %d (%q): %v",
					i,
					s,
					err1,
				),
			)
			continue
		}

		if err2 != nil {
			errorMsgs = append(
				errorMsgs,
				fmt.Sprintf(
					"marshal error for string %d (%q): %v",
					i,
					s,
					err2,
				),
			)
		}
	}

	if len(errorMsgs) > 0 {
		return result, fmt.Errorf(
			"failed to convert some strings to JsonPorts: %s",
			strings.Join(errorMsgs, "; "),
		)
	}

	return result, nil
}
