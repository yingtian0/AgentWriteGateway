package scenario

import (
	"context"
	"strings"
	"testing"
	"time"

	"agentwritegateway/internal/domain"
	"agentwritegateway/internal/grant"
	"agentwritegateway/internal/identity"
	"agentwritegateway/internal/policy"
	"agentwritegateway/internal/runner"
	"agentwritegateway/internal/store"
	"agentwritegateway/pkg/protocol"
)

func TestCompromisedControlPlaneCannotEscapeCustomerServiceAllowlist(t *testing.T) {
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	signer, public, err := grant.NewDevelopmentSigner("compromised-control-key")
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	actionGrant := protocol.ActionGrant{ProtocolVersion: protocol.VersionV1Alpha1, GrantID: "malicious-grant", Issuer: "https://control.example", TenantID: "tenant-1", RunnerGroup: "prod", RunID: "run-1", StepID: "step-1", SubjectType: "user", UserSubject: "user-1", UserIdentityProof: "trusted-user-proof", AgentID: "agent-1", DelegationRef: "delegation-1", Target: protocol.Target{Service: "payments", Environment: "production"}, Action: protocol.Action{Capability: protocol.CapabilityDeploy, ArtifactDigest: digest}, Risk: "low", PlanHash: digest, ContractHash: digest, ProfileHash: digest, PolicyHash: digest, PolicyInputHash: digest, EvidenceHash: digest, ApprovalProofs: []string{"approval-1"}, IdempotencyKey: "malicious-key", Nonce: "malicious-nonce", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
	actionGrant, err = grant.SignActionGrant(context.Background(), signer, actionGrant)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &scenarioAdapter{}
	serviceRunner := &runner.Runner{RunnerGroup: "prod", Grants: &grant.Verifier{Issuer: actionGrant.Issuer, RunnerGroup: "prod", TenantID: actionGrant.TenantID, Keys: grant.StaticKeys{actionGrant.Issuer + "\x00compromised-control-key": public}, Now: func() time.Time { return now }}, Subjects: scenarioSubject{subject: identity.Subject{Issuer: "https://idp.example", ID: "user-1"}}, Delegations: &identity.DelegationVerifier{Resolver: identity.StaticDelegations{"delegation-1": {ID: "delegation-1", Issuer: "https://idp.example", UserSubject: "user-1", AgentID: "agent-1", Actions: []protocol.Capability{protocol.CapabilityDeploy}, ServiceSelectors: []string{"*"}, Environments: []string{"production"}, MaximumRisk: "low", ExpiresAt: now.Add(time.Hour)}}, Now: func() time.Time { return now }}, Contexts: runner.StaticContexts{"run-1\x00step-1": {PlanHash: digest, ContractHash: digest, ProfileHash: digest, PolicyHash: digest, EvidenceHash: digest, Target: protocol.Target{Service: "identity", Environment: "production"}, Action: actionGrant.Action}}, Capabilities: runner.NewCapabilitySet(protocol.CapabilityDeploy), Policy: policy.NewEngine(scenarioPolicy{hash: digest}), Journal: store.NewMemory(), Credentials: scenarioCredentials{}, Adapter: adapter, Connectivity: runner.ConnectionState(true), Now: func() time.Time { return now }}
	serviceRunner.TenantID = actionGrant.TenantID
	if _, err := serviceRunner.Execute(context.Background(), actionGrant); err == nil {
		t.Fatal("compromised control plane escaped local service allowlist")
	}
	if adapter.calls != 0 {
		t.Fatalf("adapter calls=%d, want 0", adapter.calls)
	}
}

type scenarioSubject struct{ subject identity.Subject }

func (s scenarioSubject) Verify(context.Context, string) (identity.Subject, error) {
	return s.subject, nil
}

type scenarioPolicy struct{ hash string }

func (p scenarioPolicy) Evaluate(_ context.Context, input policy.Input) (policy.Evaluation, error) {
	hash, _ := policy.InputHash(input)
	return policy.Evaluation{Decision: domain.DecisionAllow, InputHash: hash, PolicyHash: p.hash}, nil
}

type scenarioCredentials struct{}

func (scenarioCredentials) Acquire(context.Context, runner.CredentialRequest) (runner.Credential, error) {
	return runner.Credential{}, nil
}

type scenarioAdapter struct{ calls int }

func (a *scenarioAdapter) Execute(context.Context, runner.AdapterRequest, runner.Credential) (runner.AdapterResult, error) {
	a.calls++
	return runner.AdapterResult{}, nil
}
