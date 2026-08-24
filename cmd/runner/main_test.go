package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerConfigRejectsProductionWithoutDurableTrustBoundary(t *testing.T) {
	path := writeConfig(t, `
mode: production
runner_id: runner-1
runner_group: prod
tenant_id: tenant-1
control_plane:
  address: http://control.example
  issuer: https://control.example
identity:
  issuer: https://idp.example
  audience: awg
policy:
  bundle_file: bundle.json
capabilities: [release.deploy]
`)
	if _, err := load(path); err == nil || !strings.Contains(err.Error(), "durable journal") {
		t.Fatalf("got %v", err)
	}
}

func TestRunnerConfigRejectsGenericCapability(t *testing.T) {
	path := writeConfig(t, `
mode: development
runner_id: runner-1
runner_group: dev
tenant_id: tenant-1
control_plane:
  address: https://control.example
  issuer: https://control.example
identity:
  issuer: https://idp.example
  audience: awg
policy:
  bundle_file: bundle.json
capabilities: [cloud.generic]
`)
	if _, err := load(path); err == nil || !strings.Contains(err.Error(), "unsupported capability") {
		t.Fatalf("got %v", err)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runner.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
