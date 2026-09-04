package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"themisy/internal/contract"
	"themisy/internal/domain"
	"themisy/internal/executor"
	"themisy/internal/planner"
	"themisy/internal/policy"
	"themisy/internal/profile"
	"themisy/internal/store"
)

func TestNormalReleaseAndDuplicateRequestAreIdempotent(t *testing.T) {
	ex := executor.NewMock(nil)
	e := testEngine(t, ex)
	request := testRequest()
	run, created, err := e.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !created || run.Status != domain.RunSucceeded {
		t.Fatalf("created=%v status=%s, want true/SUCCEEDED", created, run.Status)
	}
	if len(run.Steps) != 2 || run.Steps[0].Status != domain.StepSucceeded || run.Steps[1].Status != domain.StepSucceeded {
		t.Fatalf("unexpected steps: %#v", run.Steps)
	}

	duplicate, created, err := e.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if created || duplicate.ID != run.ID {
		t.Fatalf("duplicate created a new run: created=%v id=%s original=%s", created, duplicate.ID, run.ID)
	}
	for _, step := range run.Steps {
		if calls := ex.DeployCalls(step.Execution.IdempotencyKey); calls != 1 {
			t.Fatalf("deploy calls for %s = %d, want 1", step.Service, calls)
		}
	}
}

func TestDestructiveChangeWaitsForAuthorizedApproval(t *testing.T) {
	ex := executor.NewMock(nil)
	e := testEngine(t, ex)
	request := testRequest()
	request.Changes[0].DestructiveMigration = true
	run, _, err := e.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var approvalStep *domain.ReleaseStep
	for i := range run.Steps {
		if run.Steps[i].Approval != nil {
			approvalStep = &run.Steps[i]
			break
		}
	}
	if run.Status != domain.RunWaitingApproval || approvalStep == nil || approvalStep.Execution != nil {
		t.Fatalf("run crossed approval boundary: status=%s step=%#v", run.Status, approvalStep)
	}
	approvalID := approvalStep.Approval.ID
	_, err = e.DecideApproval(context.Background(), run.ID, approvalID, "user-1", []string{"service-owner", "sre"}, true)
	if !errors.Is(err, ErrApproval) {
		t.Fatalf("got %v, want ErrApproval for self approval", err)
	}
	_, err = e.DecideApproval(context.Background(), run.ID, approvalID, "owner", []string{"service-owner"}, true)
	if !errors.Is(err, ErrApproval) {
		t.Fatalf("got %v, want ErrApproval for missing sre role", err)
	}
	run, err = e.DecideApproval(context.Background(), run.ID, approvalID, "sre-user", []string{"service-owner", "sre"}, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range run.Steps {
		if run.Steps[i].Approval != nil {
			approvalStep = &run.Steps[i]
		}
	}
	if run.Status != domain.RunSucceeded || approvalStep.Approval.Status != domain.ApprovalApproved {
		t.Fatalf("unexpected approved run: status=%s approval=%s", run.Status, approvalStep.Approval.Status)
	}
}

func TestCompatibilityPauseResumeDoesNotBypassApproval(t *testing.T) {
	e := testEngine(t, executor.NewMock(nil))
	request := testRequest()
	request.Changes[0].DestructiveMigration = true
	run, _, err := e.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.RunWaitingApproval {
		t.Fatalf("status=%s", run.Status)
	}
	run, err = e.Pause(run.ID, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.RunPaused || run.PausedFrom != domain.RunWaitingApproval {
		t.Fatalf("paused run=%#v", run)
	}
	run, err = e.Resume(context.Background(), run.ID, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.RunWaitingApproval {
		t.Fatalf("status=%s", run.Status)
	}
	for _, step := range run.Steps {
		if step.Approval != nil && step.Execution != nil {
			t.Fatalf("resume crossed approval: %#v", step)
		}
	}
}

func TestPolicyDenyCreatesNoExecutionAndStopsDownstream(t *testing.T) {
	ex := executor.NewMock(nil)
	e := testEngine(t, ex)
	request := testRequest()
	request.Agent.Scopes = []string{"release:deploy"}
	run, _, err := e.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.RunBlocked || run.Steps[0].Status != domain.StepBlocked || run.Steps[0].Execution != nil {
		t.Fatalf("deny invariant violated: %#v", run)
	}
	if run.Steps[1].Status != domain.StepCancelled {
		t.Fatalf("downstream status=%s, want CANCELLED", run.Steps[1].Status)
	}
}

func TestVerificationFailureRollsBackAndStopsDownstream(t *testing.T) {
	ex := executor.NewMock(map[string]executor.MockBehavior{
		"identity": {VerifyHealthy: false},
	})
	e := testEngine(t, ex)
	run, _, err := e.Start(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.RunFailed || run.Steps[0].Status != domain.StepRolledBack {
		t.Fatalf("got run=%s step=%s, want FAILED/ROLLED_BACK", run.Status, run.Steps[0].Status)
	}
	if run.Steps[1].Status != domain.StepCancelled {
		t.Fatalf("downstream status=%s, want CANCELLED", run.Steps[1].Status)
	}
}

func TestEveryExecutionHasPolicyAndAuditTrail(t *testing.T) {
	e := testEngine(t, executor.NewMock(nil))
	run, _, err := e.Start(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range run.Steps {
		if step.Execution != nil && (step.Policy == nil || step.Policy.Decision != domain.DecisionAllow || step.Policy.InputHash == "") {
			t.Fatalf("execution lacks reproducible policy decision: %#v", step)
		}
	}
	events, err := e.Events(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 5 {
		t.Fatalf("got %d audit events, want at least 5", len(events))
	}
}

func TestAuditFailurePreventsExternalWrite(t *testing.T) {
	ex := executor.NewMock(nil)
	p, err := planner.New([]domain.Service{{Name: "identity"}})
	if err != nil {
		t.Fatal(err)
	}
	st := &failingAuditStore{Memory: store.NewMemory(), failAction: "deployment.start"}
	e := New(p, policy.New(), ex, st)
	request := testRequest()
	request.Changes = request.Changes[1:]
	run, _, err := e.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.RunBlocked || run.Steps[0].Execution != nil {
		t.Fatalf("audit failure did not fail closed: %#v", run)
	}
	key := run.ID + "/production/identity/sha-identity"
	if calls := ex.DeployCalls(key); calls != 0 {
		t.Fatalf("external deploy calls=%d, want 0", calls)
	}
}

func TestExpiredPlanCannotExecute(t *testing.T) {
	plannedAt := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	contracts, err := contract.LoadDir("../../examples/contracts")
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := profile.LoadDir("../../examples/profiles")
	if err != nil {
		t.Fatal(err)
	}
	p, err := planner.NewFromContracts(contracts, profiles, planner.Options{
		Now:     func() time.Time { return plannedAt },
		PlanTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	e := New(p, policy.New(), executor.NewMock(nil), store.NewMemory())
	e.now = func() time.Time { return plannedAt.Add(time.Minute) }
	request := domain.ReleaseRequest{
		RequestID: "expired-request", ReleaseVersion: "release-1",
		Environment: domain.EnvironmentStaging, RequestedBy: "user-1",
		Agent: domain.AgentIdentity{ID: "agent-1", Scopes: []string{"release:deploy"}},
		Changes: []domain.Change{{
			Service: "identity-api", DesiredVersion: "sha-identity",
			CISuccess: true, DependenciesHealthy: true,
		}},
	}
	run, created, err := e.Start(context.Background(), request)
	if run != nil || created {
		t.Fatalf("expired plan created a run: run=%#v created=%v", run, created)
	}
	if code, ok := domain.ReasonOf(err); !ok || code != domain.ReasonPlanExpired {
		t.Fatalf("got %v, want %s", err, domain.ReasonPlanExpired)
	}
}

type failingAuditStore struct {
	*store.Memory
	failAction string
}

func (s *failingAuditStore) AppendAudit(event domain.AuditEvent) error {
	if event.Action == s.failAction {
		return errors.New("simulated audit outage")
	}
	return s.Memory.AppendAudit(event)
}

func testEngine(t *testing.T, ex executor.ReleaseExecutor) *Engine {
	t.Helper()
	p, err := planner.New([]domain.Service{
		{Name: "identity", ReleasePhase: 0},
		{Name: "payments", ReleasePhase: 0, Dependencies: []string{"identity"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return New(p, policy.New(), ex, store.NewMemory())
}

func testRequest() domain.ReleaseRequest {
	return domain.ReleaseRequest{
		RequestID: "request-1", ReleaseVersion: "release-1",
		Environment: domain.EnvironmentProduction, RequestedBy: "user-1",
		Agent: domain.AgentIdentity{ID: "agent-1", Scopes: []string{"release:deploy", "release:production"}},
		Changes: []domain.Change{
			{Service: "payments", DesiredVersion: "sha-payments", CISuccess: true, DependenciesHealthy: true},
			{Service: "identity", DesiredVersion: "sha-identity", CISuccess: true, DependenciesHealthy: true},
		},
	}
}
