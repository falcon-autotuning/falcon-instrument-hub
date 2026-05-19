//go:build cgo && falcon_core

package config

// Load is the CGO config loader: uses falcon-core bindings to capture cereal JSON
// so the device config response is in the format C++ Config::from_json_string expects.
func Load(deviceConfigPath, wiremapPath string) (*Config, error) {
	return LoadConfigCGO(deviceConfigPath, wiremapPath)
}
