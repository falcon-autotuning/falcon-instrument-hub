//go:build !cgo || !falcon_core

package config

// Load is the non-CGO config loader.
func Load(deviceConfigPath, wiremapPath string) (*Config, error) {
	return LoadConfig(deviceConfigPath, wiremapPath)
}
