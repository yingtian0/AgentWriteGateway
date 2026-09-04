package transport

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"themisy/internal/application"
	"themisy/internal/domain"
	"themisy/internal/grant"
	"themisy/internal/identity"
	"themisy/internal/policy"
	runnercore "themisy/internal/runner"
	"themisy/internal/store"
	"themisy/pkg/credentials"
	"themisy/pkg/protocol"
)

func TestStaticRunnerAuthenticatorRequiresBearerScheme(t *testing.T) {
	authenticator := StaticRunnerAuthenticator{"runner-1": {TenantID: "tenant-1", RunnerGroup: "prod-jp", Token: "secret"}}
	request := httptest.NewRequest(http.MethodGet, "/v1/runner/grants:next", nil)
	request.Header.Set(runnerIDHeader, "runner-1")
	request.Header.Set("Authorization", "secret")
	if _, err := authenticator.Authenticate(request); err == nil {
		t.Fatal("expected non-Bearer authorization to be rejected")
	}
}

func TestSignedGrantReachesSeparateRunnerProcess(t *testing.T) {
	if os.Getenv("THEMISY_RUNNER_HELPER") == "1" {
		runRunnerHelper(t)
		return
	}
	now := time.Now().UTC()
	signer, publicKey, err := grant.NewDevelopmentSigner("transport-key")
	if err != nil {
		t.Fatal(err)
	}
	memory := store.NewMemory()
	run := approvedTransportRun(now)
	if _, _, err := memory.CreateRun(run); err != nil {
		t.Fatal(err)
	}
	issuer, err := application.NewGrants(memory, signer, "https://control.example", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	issuer.Now = func() time.Time { return now }
	dispatch, created, err := issuer.Issue(context.Background(), application.GrantIssueRequest{RunID: run.ID, StepID: "payment-api", Capability: protocol.CapabilityDeploy})
	if err != nil || !created {
		t.Fatalf("issue dispatch=%v err=%v", created, err)
	}
	actionGrant := dispatch.Grant
	server := httptest.NewServer((&RunnerServer{Store: memory, Auth: StaticRunnerAuthenticator{"runner-1": {TenantID: actionGrant.TenantID, RunnerGroup: actionGrant.RunnerGroup, Token: "test-runner-token"}}, MaxWait: time.Second, PollInterval: 10 * time.Millisecond}).Handler())
	defer server.Close()
	marker := t.TempDir() + "/adapter-called"
	command := exec.Command(os.Args[0], "-test.run=TestSignedGrantReachesSeparateRunnerProcess")
	command.Env = append(os.Environ(),
		"THEMISY_RUNNER_HELPER=1",
		"THEMISY_TEST_CONTROL_PLANE="+server.URL,
		"THEMISY_TEST_GRANT_KEY="+base64.RawURLEncoding.EncodeToString(publicKey),
		"THEMISY_TEST_MARKER="+marker,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("runner subprocess: %v\n%s", err, output)
	}
	record, err := memory.GetGrantDispatch(context.Background(), actionGrant.GrantID)
	if err != nil || record.Status != store.GrantDispatchSucceeded || record.AcknowledgedAt.IsZero() || record.Result.ExternalExecutionID != "separate-process-execution" {
		t.Fatalf("dispatch=%#v err=%v", record, err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("Runner.Execute did not reach typed adapter: %v", err)
	}
}

func runRunnerHelper(t *testing.T) {
	t.Helper()
	publicKeyBytes, err := base64.RawURLEncoding.DecodeString(os.Getenv("THEMISY_TEST_GRANT_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	digest := testTransportDigest()
	verifier := &grant.Verifier{Issuer: "https://control.example", RunnerGroup: "prod-jp", TenantID: "tenant-1", Keys: grant.StaticKeys{"https://control.example\x00transport-key": ed25519.PublicKey(publicKeyBytes)}, ClockSkew: time.Minute}
	contexts := runnercore.NewReceivedContexts()
	connected := &runnercore.AtomicConnectionState{}
	broker, err := credentials.NewStaticDevelopmentBroker(map[string][]byte{"token": []byte("local-only")}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	executionRunner := &runnercore.Runner{
		TenantID: "tenant-1", RunnerGroup: "prod-jp", Grants: verifier,
		Subjects: helperSubject{}, Delegations: &identity.DelegationVerifier{Resolver: identity.StaticDelegations{"delegation-1": {ID: "delegation-1", Issuer: "https://idp.example", UserSubject: "user-1", AgentID: "agent-1", Actions: []protocol.Capability{protocol.CapabilityDeploy}, ServiceSelectors: []string{"payment-api"}, Environments: []string{"production"}, MaximumRisk: "high", ExpiresAt: now.Add(time.Hour)}}},
		Contexts: contexts, Approvals: contexts, Capabilities: runnercore.NewCapabilitySet(protocol.CapabilityDeploy),
		Policy: policy.NewEngine(helperPolicy{policyHash: digest}), Journal: store.NewMemory(), Credentials: broker,
		Adapter: helperAdapter{marker: os.Getenv("THEMISY_TEST_MARKER")}, Connectivity: connected,
	}
	client := &RunnerClient{BaseURL: os.Getenv("THEMISY_TEST_CONTROL_PLANE"), RunnerID: "runner-1", Token: "test-runner-token", Wait: time.Second, Connectivity: connected, BeforeExecute: func(ctx context.Context, actionGrant protocol.ActionGrant) error {
		if err := verifier.Verify(ctx, actionGrant); err != nil {
			return err
		}
		contexts.BindVerified(actionGrant)
		return nil
	}}
	delivered, err := client.RunOnce(context.Background(), executionRunner)
	if err != nil || !delivered {
		t.Fatalf("delivered=%v err=%v", delivered, err)
	}
}

type helperSubject struct{}

func (helperSubject) Verify(_ context.Context, _ string) (identity.Subject, error) {
	return identity.Subject{Issuer: "https://idp.example", ID: "user-1", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

type helperPolicy struct{ policyHash string }

func (p helperPolicy) Evaluate(_ context.Context, input policy.Input) (policy.Evaluation, error) {
	hash, err := policy.InputHash(input)
	return policy.Evaluation{Decision: domain.DecisionAllow, InputHash: hash, PolicyHash: p.policyHash, PolicyVersion: "test"}, err
}

type helperAdapter struct{ marker string }

func (a helperAdapter) Execute(_ context.Context, _ runnercore.AdapterRequest, _ credentials.Credential) (runnercore.AdapterResult, error) {
	if err := os.WriteFile(a.marker, []byte("called"), 0o600); err != nil {
		return runnercore.AdapterResult{}, err
	}
	return runnercore.AdapterResult{ExternalExecutionID: "separate-process-execution", CompletedAt: time.Now().UTC()}, nil
}

func approvedTransportRun(now time.Time) *domain.ReleaseRun {
	digest := testTransportDigest()
	run := &domain.ReleaseRun{ID: "run-transport", RequestID: "request-transport", WorkflowID: "run-transport", TenantID: "tenant-1", Environment: domain.EnvironmentProduction, RequestedBy: "user-1", SubjectType: "user", SubjectIssuer: "https://idp.example", UserIdentityProof: "signed-oidc", DelegationRef: "delegation-1", Agent: domain.AgentIdentity{ID: "agent-1"}, Status: domain.RunRunning, StateVersion: 1, CreatedAt: now, UpdatedAt: now,
		Plan:  domain.ReleasePlan{Hash: digest, PlanHash: digest, PolicyHash: digest, EvidenceHash: digest, ExpiresAt: now.Add(time.Hour), Phases: []domain.PlanPhase{{Steps: []domain.PlanStep{{Service: "payment-api", ContractHash: digest, ProfileHash: digest, Scheduling: domain.SchedulingContext{RunnerGroup: "prod-jp", RiskTier: "medium"}}}}}},
		Steps: []domain.ReleaseStep{{Service: "payment-api", Status: domain.StepExecuting, Change: domain.Change{Service: "payment-api", DesiredVersion: digest, Risk: "medium", CISuccess: true, DependenciesHealthy: true}}},
	}
	inputHash, _ := policy.InputHash(policy.InputForRelease(*run, run.Steps[0]))
	run.Steps[0].Policy = &domain.PolicyDecision{Decision: domain.DecisionAllow, InputHash: inputHash}
	return run
}

func testTransportDigest() string { return "sha256:" + strings.Repeat("a", 64) }
