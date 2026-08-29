package adapter

import (
	"errors"
	"testing"
	"time"
)

func TestTypedValidationAndErrorClassification(t *testing.T) {
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := DeployRequest{Target: Target{Service: "identity", Environment: "staging"}, ArtifactDigest: digest, IdempotencyKey: "key"}
	if err := ValidateDeployRequest(request); err != nil {
		t.Fatal(err)
	}
	request.ArtifactDigest = "latest"
	if err := ValidateDeployRequest(request); err == nil {
		t.Fatal("untyped artifact accepted")
	}
	classified := &Error{Class: ErrorUnknown, Operation: "deploy", Err: errors.New("timeout")}
	if !IsClass(classified, ErrorUnknown) || IsClass(classified, ErrorTerminal) {
		t.Fatal("error classification failed")
	}
}

func TestEvidenceHashDetectsMutation(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	evidence := Evidence{Status: VerificationPass, ReasonCode: "PASS", Source: "datadog", QueryHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Window: ObservationWindow{From: now.Add(-time.Minute), To: now}, ObservedAt: now, ObservedValue: 0.2, Threshold: 1, AdapterVersion: "datadog/v1"}
	evidence.EvidenceHash, _ = EvidenceHash(evidence)
	if err := ValidateEvidence(evidence); err != nil {
		t.Fatal(err)
	}
	original := evidence.EvidenceHash
	evidence.ObservedValue = 2
	mutated, _ := EvidenceHash(evidence)
	if original == mutated {
		t.Fatal("evidence mutation did not change hash")
	}
}
