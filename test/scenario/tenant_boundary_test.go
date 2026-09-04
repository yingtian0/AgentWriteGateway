package scenario

import (
	"context"
	"testing"

	"themisy/internal/application"
	"themisy/internal/domain"
	"themisy/internal/planner"
	"themisy/internal/store"
	workflowcore "themisy/internal/workflow"
)

func TestTenantBoundaryCoversPlanRunAndApprovalQueries(t *testing.T) {
	p, err := planner.New([]domain.Service{{Name: "identity"}})
	if err != nil {
		t.Fatal(err)
	}
	app := application.NewReleases(p, store.NewMemory(), scenarioController{})
	intent := domain.ReleaseIntent{APIVersion: domain.ReleaseIntentAPIVersion, Kind: domain.ReleaseIntentKind, RequestID: "tenant-boundary", ReleaseVersion: "release-1", TenantID: "tenant-a", Environment: domain.EnvironmentStaging, RequestedBy: "user", Agent: domain.AgentIdentity{ID: "agent", Scopes: []string{"release:deploy"}}, Changes: []domain.Change{{Service: "identity", DesiredVersion: "sha-1", CISuccess: true, DependenciesHealthy: true}}}
	plan, err := app.PlanIntentForTenant("tenant-a", intent)
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := app.StartIntentForTenant(context.Background(), "tenant-a", intent)
	if err != nil {
		t.Fatal(err)
	}
	for name, operation := range map[string]func() error{
		"plan":   func() error { _, err := app.GetPlan("tenant-b", plan.ID); return err },
		"run":    func() error { _, err := app.GetForTenant("tenant-b", run.ID); return err },
		"events": func() error { _, err := app.EventsForTenant("tenant-b", run.ID); return err },
	} {
		if code := scenarioReason(operation()); code != domain.ReasonTenantBoundary {
			t.Fatalf("%s cross-tenant reason=%s", name, code)
		}
	}
	approvals, err := app.ListPendingApprovals("tenant-b")
	if err != nil || len(approvals) != 0 {
		t.Fatalf("cross-tenant approvals=%#v err=%v", approvals, err)
	}
}

func scenarioReason(err error) domain.ReasonCode {
	reason, _ := domain.ReasonOf(err)
	return reason
}

type scenarioController struct{}

func (scenarioController) StartRelease(context.Context, workflowcore.ReleaseInput) (workflowcore.Execution, error) {
	return workflowcore.Execution{}, nil
}
func (scenarioController) SignalApproval(context.Context, string, workflowcore.ApprovalSignal) error {
	return nil
}
func (scenarioController) Pause(context.Context, string, workflowcore.ControlSignal) error {
	return nil
}
func (scenarioController) Resume(context.Context, string, workflowcore.ControlSignal) error {
	return nil
}
func (scenarioController) Cancel(context.Context, string, workflowcore.ControlSignal) error {
	return nil
}
