package instrument

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// unmarshalAndValidate handles the common unmarshaling and validation logic
func (h *Handler) unmarshalAndValidate(
	data []byte,
	req any,
	commandName string,
) error {
	if err := json.Unmarshal(data, req); err != nil {
		h.logger.Error(
			HandlerName,
			fmt.Sprintf("Failed to unmarshal %s: %v", commandName, err),
		)
		return err
	}

	// Use reflection to get the Name field
	v := reflect.ValueOf(req).Elem()
	nameField := v.FieldByName("Name")
	if !nameField.IsValid() || nameField.String() == "" {
		h.logger.Error(
			HandlerName,
			fmt.Sprintf("%s missing instrument name", commandName),
		)
		return fmt.Errorf("missing instrument name")
	}

	return nil
}
