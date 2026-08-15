package contract

import (
	"strings"
	"testing"

	"agentwritegateway/internal/domain"
)

func TestLoadDirLoadsValidContractsWithContentHash(t *testing.T) {
	contracts, err := LoadDir("../../test/fixtures/contracts/valid")
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 1 {
		t.Fatalf("contracts=%d, want 1", len(contracts))
	}
	if !strings.HasPrefix(contracts[0].ContentHash, "sha256:") || contracts[0].Source == "" {
		t.Fatalf("contract lacks source metadata: %#v", contracts[0])
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	data := []byte(`
apiVersion: execution.agentwritegateway.io/v1alpha1
kind: ServiceContract
unexpected: true
`)
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "field unexpected not found") {
		t.Fatalf("got %v, want unknown field error", err)
	}
}

func TestCanonicalHashIgnoresCollectionOrderAndSource(t *testing.T) {
	contract, err := LoadFile("../../examples/contracts/payment-api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	changed := contract
	changed.Source = "/different/path.yaml"
	changed.ContentHash = "stale"
	changed.Dependencies = append([]domain.Dependency(nil), contract.Dependencies...)
	for left, right := 0, len(changed.Dependencies)-1; left < right; left, right = left+1, right-1 {
		changed.Dependencies[left], changed.Dependencies[right] = changed.Dependencies[right], changed.Dependencies[left]
	}
	production := changed.Environments["production"]
	for left, right := 0, len(production.Capabilities)-1; left < right; left, right = left+1, right-1 {
		production.Capabilities[left], production.Capabilities[right] = production.Capabilities[right], production.Capabilities[left]
	}
	changed.Environments["production"] = production
	hash, err := Hash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if hash != contract.ContentHash {
		t.Fatalf("hash changed with non-semantic order/source: %s != %s", hash, contract.ContentHash)
	}
}
