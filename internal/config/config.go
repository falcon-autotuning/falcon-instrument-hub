package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Port       string `json:"port"`
	DBHost     string `json:"db_host"`
	DBPort     string `json:"db_port"`
	DBUser     string `json:"db_user"`
	DBPassword string `json:"db_password"`
	DBName     string `json:"db_name"`
	SchemaFile string `json:"schema_file"`
}

func WriteConfig() error {
	config := Config{Port: "4222", DBHost: "localhost", DBPort: "8080", DBUser: "postgres", DBName: "spec", DBPassword: "falcon_123", SchemaFile: "specschema.sql"}

	// return &config, nil
	jsonConfig, err := json.Marshal(config)
	if err != nil {
		return err
	}

	err = os.WriteFile("config.json", jsonConfig, 0644)
	if err != nil {
		return err
	}
	return nil
}

func LoadConfig() (*Config, error) {
	file, err := os.Open("config.json")
	if err != nil {
		err := WriteConfig()
		if err != nil {
			return nil, err
		}

	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}
	return &config, nil
}
