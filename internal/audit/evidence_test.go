package audit

import (
	"testing"
	"time"

	"agentwritegateway/pkg/adapter"
)

func TestEvidenceDetailsOmitRawQueryAndSecrets(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	details := EvidenceDetails(adapter.Evidence{Status: adapter.VerificationPass, ReasonCode: "PASS", Source: "datadog", QueryHash: "sha256:abc", Window: adapter.ObservationWindow{From: now.Add(-time.Minute), To: now}, ObservedAt: now, EvidenceHash: "sha256:def"})
	for _, forbidden := range []string{"query", "credential", "token", "logs"} {
		if _, exists := details[forbidden]; exists {
			t.Fatalf("unsafe evidence key %q", forbidden)
		}
	}
	if details["query_hash"] != "sha256:abc" || details["status"] != adapter.VerificationPass {
		t.Fatalf("details=%#v", details)
	}
}
