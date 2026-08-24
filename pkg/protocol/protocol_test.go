package protocol

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestCanonicalGrantPayloadIsStableAndBindsFields(t *testing.T) {
	grant := protocolFixture()
	first, err := CanonicalGrantPayload(grant)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalGrantPayload(grant)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("canonical payload is not stable")
	}
	reordered := grant
	reordered.ApprovalProofs = []string{"approval-2", "approval-1"}
	grant.ApprovalProofs = []string{"approval-1", "approval-2"}
	orderedPayload, _ := CanonicalGrantPayload(grant)
	reorderedPayload, _ := CanonicalGrantPayload(reordered)
	if !bytes.Equal(orderedPayload, reorderedPayload) {
		t.Fatal("approval proof order changed canonical payload")
	}
	originalHash, _ := GrantHash(grant)
	grant.Target.Service = "payments"
	modifiedHash, _ := GrantHash(grant)
	if originalHash == modifiedHash {
		t.Fatal("target mutation did not change hash")
	}
}

func TestDecodeActionGrantRejectsUnknownFields(t *testing.T) {
	encoded, err := EncodeActionGrant(protocolFixture())
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded[:len(encoded)-1], []byte(`,"command":"rm -rf"}`)...)
	if _, err := DecodeActionGrant(bytes.NewReader(encoded)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("got %v, want unknown field rejection", err)
	}
}

func TestCanonicalGrantRejectsNonHexDigest(t *testing.T) {
	grant := protocolFixture()
	grant.PlanHash = "sha256:" + strings.Repeat("z", 64)
	if _, err := CanonicalGrantPayload(grant); err == nil {
		t.Fatal("non-hex digest was accepted")
	}
}

func protocolFixture() ActionGrant {
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	digest := "sha256:" + strings.Repeat("a", 64)
	return ActionGrant{ProtocolVersion: VersionV1Alpha1, GrantID: "grant-1", Issuer: "https://control.example", TenantID: "tenant-1", RunnerGroup: "prod-jp", RunID: "run-1", StepID: "step-1", SubjectType: "user", UserSubject: "user-1", UserIdentityProof: "jwt", AgentID: "agent-1", DelegationRef: "delegation-1", Target: Target{Service: "identity", Environment: "production"}, Action: Action{Capability: CapabilityDeploy, ArtifactDigest: digest}, Risk: "medium", PlanHash: digest, ContractHash: digest, ProfileHash: digest, PolicyHash: digest, PolicyInputHash: digest, EvidenceHash: digest, ApprovalProofs: []string{"approval-1"}, IdempotencyKey: "key-1", Nonce: "nonce-1", IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute)}
}
