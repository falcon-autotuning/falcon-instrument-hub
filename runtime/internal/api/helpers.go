package api

import "reflect"

// GetCommandName extracts command name from CommandRegistry
func GetCommandName(cmdType any) string {
	cmdTypeOf := reflect.TypeOf(cmdType)
	for name, cmdInstance := range CommandRegistry {
		if cmdTypeOf == reflect.TypeOf(cmdInstance) {
			return name
		}
	}
	return ""
}
