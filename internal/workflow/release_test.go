package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"themisy/internal/domain"
	"themisy/internal/executor"
	"themisy/internal/policy"
	"themisy/internal/store"

	"go.temporal.io/sdk/testsuite"
)

func TestReleaseWorkflowCompletesThroughActivities(t *testing.T) {
	run, memory, exec := workflowFixture(t, false, nil)
	env := newWorkflowEnvironment(memory, exec)
	env.ExecuteWorkflow(ReleaseWorkflow, ReleaseInput{Run: run})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	var result domain.ReleaseRun
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.RunSucceeded || result.Steps[0].Status != domain.StepSucceeded {
		t.Fatalf("run=%s step=%s", result.Status, result.Steps[0].Status)
	}
	if calls := exec.DeployCalls(result.Steps[0].Execution.IdempotencyKey); calls != 1 {
		t.Fatalf("deploy calls=%d, want 1", calls)
	}
	persisted, err := memory.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != domain.RunSucceeded {
		t.Fatalf("persisted status=%s", persisted.Status)
	}
}

func TestApprovalPauseResumeAndCancelSignalsSurviveWorkflowWait(t *testing.T) {
	t.Run("approval after pause and resume", func(t *testing.T) {
		run, memory, exec := workflowFixture(t, true, nil)
		env := newWorkflowEnvironment(memory, exec)
		approvalID := deterministicID("approval", run.ID, run.Steps[0].Service)
		env.RegisterDelayedCallback(func() { env.SignalWorkflow(SignalPause, ControlSignal{Actor: "operator"}) }, time.Minute)
		env.RegisterDelayedCallback(func() { env.SignalWorkflow(SignalResume, ControlSignal{Actor: "operator"}) }, 2*time.Minute)
		env.RegisterDelayedCallback(func() {
			env.SignalWorkflow(SignalApproval, ApprovalSignal{ApprovalID: approvalID, Actor: "sre-user", Roles: []string{"service-owner", "sre"}, Approve: true})
		}, 3*time.Minute)
		env.ExecuteWorkflow(ReleaseWorkflow, ReleaseInput{Run: run})
		if err := env.GetWorkflowError(); err != nil {
			t.Fatal(err)
		}
		var result domain.ReleaseRun
		if err := env.GetWorkflowResult(&result); err != nil {
			t.Fatal(err)
		}
		if result.Status != domain.RunSucceeded || result.Steps[0].Approval.Status != domain.ApprovalApproved {
			t.Fatalf("status=%s approval=%s", result.Status, result.Steps[0].Approval.Status)
		}
	})
	t.Run("cancel while waiting", func(t *testing.T) {
		run, memory, exec := workflowFixture(t, true, nil)
		env := newWorkflowEnvironment(memory, exec)
		env.RegisterDelayedCallback(func() { env.SignalWorkflow(SignalCancel, ControlSignal{Actor: "operator"}) }, time.Minute)
		env.ExecuteWorkflow(ReleaseWorkflow, ReleaseInput{Run: run})
		if err := env.GetWorkflowError(); err != nil {
			t.Fatal(err)
		}
		var result domain.ReleaseRun
		if err := env.GetWorkflowResult(&result); err != nil {
			t.Fatal(err)
		}
		if result.Status != domain.RunCancelled || exec.DeployCalls(run.ID+"/production/identity/sha-1") != 0 {
			t.Fatalf("status=%s deploy should not run", result.Status)
		}
	})
}

func TestPersistActivityFailuresRecoverWithoutDuplicateWrite(t *testing.T) {
	for failAt := 1; failAt <= 5; failAt++ {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			run, memory, exec := workflowFixture(t, false, nil)
			flaky := &flakyUpdateStore{Memory: memory, failAt: failAt}
			env := newWorkflowEnvironment(flaky, exec)
			env.ExecuteWorkflow(ReleaseWorkflow, ReleaseInput{Run: run})
			if err := env.GetWorkflowError(); err != nil {
				t.Fatal(err)
			}
			var result domain.ReleaseRun
			if err := env.GetWorkflowResult(&result); err != nil {
				t.Fatal(err)
			}
			if result.Status != domain.RunSucceeded {
				t.Fatalf("status=%s", result.Status)
			}
			if calls := exec.DeployCalls(run.ID + "/staging/identity/sha-1"); calls != 1 {
				t.Fatalf("deploy calls=%d", calls)
			}
		})
	}
}

func TestUnknownDeployStateNeverCreatesSecondWrite(t *testing.T) {
	run, memory, exec := workflowFixture(t, false, nil)
	failing := &failCompleteStore{Memory: memory, fail: true}
	activities := NewActivities(failing, policy.New(), exec)
	input := DeployInput{RunID: run.ID, RequestedBy: run.RequestedBy, AgentID: run.Agent.ID, Service: "identity", Environment: "staging", DesiredVersion: "sha-1", IdempotencyKey: "fixed-key"}
	if _, err := activities.Deploy(context.Background(), input); err == nil {
		t.Fatal("first deploy should report unknown persistence state")
	}
	if _, err := activities.Deploy(context.Background(), input); err == nil {
		t.Fatal("second deploy should require reconciliation")
	}
	if calls := exec.DeployCalls("fixed-key"); calls != 1 {
		t.Fatalf("deploy calls=%d, want 1", calls)
	}
}

func newWorkflowEnvironment(st store.DurableStore, exec executor.ReleaseExecutor) *testsuite.TestWorkflowEnvironment {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	activities := NewActivities(st, policy.New(), exec)
	env.RegisterActivity(activities.PersistRun)
	env.RegisterActivity(activities.EvaluateStep)
	env.RegisterActivity(activities.Deploy)
	env.RegisterActivity(activities.Verify)
	env.RegisterActivity(activities.Rollback)
	return env
}

func workflowFixture(t *testing.T, approval bool, behaviors map[string]executor.MockBehavior) (domain.ReleaseRun, *store.Memory, *executor.Mock) {
	t.Helper()
	now := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	environment := domain.EnvironmentStaging
	scopes := []string{"release:deploy"}
	if approval {
		environment = domain.EnvironmentProduction
		scopes = append(scopes, "release:production")
	}
	change := domain.Change{Service: "identity", DesiredVersion: "sha-1", CISuccess: true, DependenciesHealthy: true, DestructiveMigration: approval}
	run := domain.ReleaseRun{ID: "run-workflow", WorkflowID: "run-workflow", RequestID: "request-workflow", ReleaseVersion: "release-1", Environment: environment, RequestedBy: "requester", Agent: domain.AgentIdentity{ID: "agent", Scopes: scopes}, Plan: domain.ReleasePlan{Hash: "plan-hash", PlanHash: "plan-hash", Phases: []domain.PlanPhase{{Number: 0, Steps: []domain.PlanStep{{Service: "identity", DesiredVersion: "sha-1", Phase: 0, VerificationRequired: true, ObservationWindow: "1s", RollbackMode: domain.RollbackAutomatic}}}}}, Status: domain.RunPending, Steps: []domain.ReleaseStep{{Service: "identity", Phase: 0, Status: domain.StepPending, Change: change, VerificationRequired: true, ObservationWindow: "1s", RollbackMode: domain.RollbackAutomatic}}, StateVersion: 1, CreatedAt: now, UpdatedAt: now}
	memory := store.NewMemory()
	if _, created, err := memory.CreateRun(&run); err != nil || !created {
		t.Fatalf("create fixture: %v", err)
	}
	return run, memory, executor.NewMock(behaviors)
}

type flakyUpdateStore struct {
	*store.Memory
	calls  int
	failAt int
}

func (s *flakyUpdateStore) UpdateRunAtomic(run *domain.ReleaseRun, expected int64, audit []domain.AuditEvent, outbox []domain.OutboxEvent) error {
	s.calls++
	if s.calls == s.failAt {
		return errors.New("simulated worker crash before transaction")
	}
	return s.Memory.UpdateRunAtomic(run, expected, audit, outbox)
}

type failCompleteStore struct {
	*store.Memory
	fail bool
}

func (s *failCompleteStore) CompleteExecution(record store.ExecutionRecord, expected int64, audit domain.AuditEvent, outbox domain.OutboxEvent) error {
	if s.fail {
		s.fail = false
		return errors.New("simulated commit response loss")
	}
	return s.Memory.CompleteExecution(record, expected, audit, outbox)
}
