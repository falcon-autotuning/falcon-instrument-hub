package main

import (
	"encoding/json"
	"os"

	"github.com/invopop/jsonschema"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
)

func main() {
	reflector := jsonschema.Reflector{}

	if err := reflector.AddGoComments(
		"github.com/falcon-autotuning/instrument-server/runtime",
		"./internal/config",
	); err != nil {
		panic(err)
	}

	schema := reflector.Reflect(&config.HubConfig{})

	data, err := json.MarshalIndent(
		schema,
		"",
		"  ",
	)
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(
		"../schemas/hub.schema.json",
		data,
		0644,
	); err != nil {
		panic(err)
	}
}
