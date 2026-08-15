package application

import (
	"context"

	"agentwritegateway/internal/domain"
	workflowcore "agentwritegateway/internal/workflow"
)

type ReleaseService interface {
	Services() []domain.Service
	Plan(domain.ReleaseRequest) (domain.ReleasePlan, error)
	PlanIntent(domain.ReleaseIntent) (domain.ReleasePlan, error)
	Start(context.Context, domain.ReleaseRequest) (*domain.ReleaseRun, bool, error)
	StartIntent(context.Context, domain.ReleaseIntent) (*domain.ReleaseRun, bool, error)
	Get(string) (*domain.ReleaseRun, error)
	Events(string) ([]domain.AuditEvent, error)
	DecideApproval(context.Context, string, string, string, []string, bool) (*domain.ReleaseRun, error)
	Cancel(string, string) (*domain.ReleaseRun, error)
	Pause(string, string) (*domain.ReleaseRun, error)
	Resume(context.Context, string, string) (*domain.ReleaseRun, error)
}

type WorkflowController interface {
	StartRelease(context.Context, workflowcore.ReleaseInput) (workflowcore.Execution, error)
	SignalApproval(context.Context, string, workflowcore.ApprovalSignal) error
	Pause(context.Context, string, workflowcore.ControlSignal) error
	Resume(context.Context, string, workflowcore.ControlSignal) error
	Cancel(context.Context, string, workflowcore.ControlSignal) error
}
