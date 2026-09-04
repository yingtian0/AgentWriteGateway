package config

import (
	"os"
	"testing"
)

func TestLoadRejectsUnknownConfigurationFields(t *testing.T) {
	path := t.TempDir() + "/themisy.yaml"
	if err := os.WriteFile(path, []byte("unknown: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestExampleConfigurationLoads(t *testing.T) {
	configuration, err := Load("../../config/themisy.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Mode != "all" || configuration.Database.URL == "" || configuration.Temporal.TaskQueue == "" {
		t.Fatalf("configuration=%#v", configuration)
	}
}
