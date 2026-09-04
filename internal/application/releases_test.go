package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"themisy/internal/domain"
	"themisy/internal/planner"
	"themisy/internal/store"
	workflowcore "themisy/internal/workflow"
)

func TestStartPersistsWorkflowRequestAndDeduplicatesCommand(t *testing.T) {
	p, err := planner.New([]domain.Service{{Name: "identity"}})
	if err != nil {
		t.Fatal(err)
	}
	memory := store.NewMemory()
	controller := &fakeWorkflowController{}
	service := NewReleases(p, memory, controller)
	request := applicationRequest()
	run, created, err := service.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !created || run.Status != domain.RunPending || run.WorkflowID == "" {
		t.Fatalf("created=%v run=%#v", created, run)
	}
	if controller.starts != 1 {
		t.Fatalf("workflow starts=%d", controller.starts)
	}
	duplicate, created, err := service.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if created || duplicate.ID != run.ID || controller.starts != 1 {
		t.Fatalf("duplicate=%#v created=%v starts=%d", duplicate, created, controller.starts)
	}
	events, err := memory.AuditEvents(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events=%d", len(events))
	}
	outbox, err := memory.PendingOutbox(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(outbox) != 1 || outbox[0].EventType != "release.run.project" {
		t.Fatalf("pending outbox=%#v, want only rebuildable projection", outbox)
	}
}

func TestApprovalValidationHappensBeforeTemporalSignal(t *testing.T) {
	p, _ := planner.New([]domain.Service{{Name: "identity"}})
	memory := store.NewMemory()
	controller := &fakeWorkflowController{}
	service := NewReleases(p, memory, controller)
	now := time.Now().UTC()
	run := &domain.ReleaseRun{ID: "approval-run", WorkflowID: "approval-run", RequestID: "approval-request", RequestedBy: "requester", Plan: domain.ReleasePlan{Hash: "plan"}, Status: domain.RunWaitingApproval, StateVersion: 1, CreatedAt: now, UpdatedAt: now, Steps: []domain.ReleaseStep{{Service: "identity", Status: domain.StepWaitingApproval, Approval: &domain.Approval{ID: "approval", Status: domain.ApprovalPending, PlanHash: "plan", RequiredRoles: []string{"sre"}, ExpiresAt: now.Add(time.Hour)}}}}
	if _, _, err := memory.CreateRun(run); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DecideApproval(context.Background(), run.ID, "approval", "requester", []string{"sre"}, true); !errors.Is(err, ErrApproval) {
		t.Fatalf("self approval error=%v", err)
	}
	if _, err := service.DecideApproval(context.Background(), run.ID, "approval", "other", nil, true); !errors.Is(err, ErrApproval) {
		t.Fatalf("role error=%v", err)
	}
	if controller.approvals != 0 {
		t.Fatalf("invalid signals=%d", controller.approvals)
	}
	if _, err := service.DecideApproval(context.Background(), run.ID, "approval", "other", []string{"sre"}, true); err != nil {
		t.Fatal(err)
	}
	if controller.approvals != 1 {
		t.Fatalf("valid signals=%d", controller.approvals)
	}
}

func TestWorkflowStartOutboxRecoversAfterControlPlaneFailure(t *testing.T) {
	p, err := planner.New([]domain.Service{{Name: "identity"}})
	if err != nil {
		t.Fatal(err)
	}
	memory := store.NewMemory()
	controller := &fakeWorkflowController{err: errors.New("temporal unavailable")}
	service := NewReleases(p, memory, controller)
	run, created, err := service.Start(context.Background(), applicationRequest())
	if err == nil || !created || run == nil {
		t.Fatalf("run=%#v created=%v err=%v", run, created, err)
	}
	controller.err = nil
	published, err := service.RecoverPendingWorkflows(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if published != 1 || controller.starts != 2 {
		t.Fatalf("published=%d starts=%d", published, controller.starts)
	}
	pending, err := memory.PendingOutboxByType(workflowRequestedEvent, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending workflow requests=%d", len(pending))
	}
}

func applicationRequest() domain.ReleaseRequest {
	return domain.ReleaseRequest{RequestID: "request-application", ReleaseVersion: "release-1", Environment: domain.EnvironmentStaging, RequestedBy: "user", Agent: domain.AgentIdentity{ID: "agent", Scopes: []string{"release:deploy"}}, Changes: []domain.Change{{Service: "identity", DesiredVersion: "sha-1", CISuccess: true, DependenciesHealthy: true}}}
}

type fakeWorkflowController struct {
	starts    int
	err       error
	approvals int
}

func (f *fakeWorkflowController) StartRelease(context.Context, workflowcore.ReleaseInput) (workflowcore.Execution, error) {
	f.starts++
	return workflowcore.Execution{WorkflowID: "workflow", RunID: "temporal-run"}, f.err
}

func (f *fakeWorkflowController) SignalApproval(context.Context, string, workflowcore.ApprovalSignal) error {
	f.approvals++
	return nil
}
func (*fakeWorkflowController) Pause(context.Context, string, workflowcore.ControlSignal) error {
	return nil
}
func (*fakeWorkflowController) Resume(context.Context, string, workflowcore.ControlSignal) error {
	return nil
}
func (*fakeWorkflowController) Cancel(context.Context, string, workflowcore.ControlSignal) error {
	return nil
}
