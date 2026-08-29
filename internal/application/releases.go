package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"agentwritegateway/internal/domain"
	"agentwritegateway/internal/planner"
	"agentwritegateway/internal/store"
	workflowcore "agentwritegateway/internal/workflow"
)

var ErrInvalidRequest = errors.New("invalid release request")

type Releases struct {
	planner   *planner.Planner
	store     store.DurableStore
	workflows WorkflowController
	now       func() time.Time
}

func NewReleases(releasePlanner *planner.Planner, st store.DurableStore, workflows WorkflowController) *Releases {
	return &Releases{planner: releasePlanner, store: st, workflows: workflows, now: time.Now}
}

func (r *Releases) Services() []domain.Service { return r.planner.Services() }
func (r *Releases) Plan(request domain.ReleaseRequest) (domain.ReleasePlan, error) {
	if err := validateRequest(request); err != nil {
		return domain.ReleasePlan{}, err
	}
	return r.planner.Plan(request)
}
func (r *Releases) PlanIntent(intent domain.ReleaseIntent) (domain.ReleasePlan, error) {
	request := planner.LegacyRequestFromIntent(intent)
	if err := validateRequest(request); err != nil {
		return domain.ReleasePlan{}, err
	}
	return r.planner.PlanIntent(intent)
}
func (r *Releases) Start(ctx context.Context, request domain.ReleaseRequest) (*domain.ReleaseRun, bool, error) {
	plan, err := r.Plan(request)
	if err != nil {
		return nil, false, err
	}
	return r.start(ctx, request, plan)
}
func (r *Releases) StartIntent(ctx context.Context, intent domain.ReleaseIntent) (*domain.ReleaseRun, bool, error) {
	plan, err := r.PlanIntent(intent)
	if err != nil {
		return nil, false, err
	}
	return r.start(ctx, planner.LegacyRequestFromIntent(intent), plan)
}

func (r *Releases) start(ctx context.Context, request domain.ReleaseRequest, plan domain.ReleasePlan) (*domain.ReleaseRun, bool, error) {
	if err := r.planner.ValidatePlan(plan, r.now()); err != nil {
		return nil, false, fmt.Errorf("validate release plan before workflow start: %w", err)
	}
	now := r.now().UTC()
	runID := newID("run")
	run := &domain.ReleaseRun{ID: runID, WorkflowID: runID, RequestID: request.RequestID, ReleaseVersion: request.ReleaseVersion, Environment: request.Environment, RequestedBy: request.RequestedBy, Agent: request.Agent, Plan: plan, Status: domain.RunPending, StateVersion: 1, CreatedAt: now, UpdatedAt: now}
	changes := make(map[string]domain.Change, len(request.Changes))
	for _, change := range request.Changes {
		changes[change.Service] = change
	}
	for _, phase := range plan.Phases {
		for _, planned := range phase.Steps {
			run.Steps = append(run.Steps, domain.ReleaseStep{Service: planned.Service, Phase: phase.Number, Status: domain.StepPending, Change: changes[planned.Service], VerificationRequired: planned.VerificationRequired, ObservationWindow: planned.ObservationWindow, RollbackMode: planned.RollbackMode})
		}
	}
	audit := domain.AuditEvent{ID: newID("audit"), CorrelationID: run.ID, ActorType: "user", ActorID: request.RequestedBy, DelegatedBy: request.RequestedBy, Action: "release.start", ResourceType: "release_run", ResourceID: run.ID, Result: "ACCEPTED", Details: map[string]any{"plan_hash": plan.Hash, "workflow_id": run.WorkflowID}, Timestamp: now}
	outbox := domain.OutboxEvent{ID: newID("workflow"), AggregateType: "release_run", AggregateID: run.ID, EventType: "release.workflow.requested", Payload: map[string]any{"workflow_id": run.WorkflowID}, CreatedAt: now, AvailableAt: now}
	stored, created, err := r.store.CreateRunAtomic(run, []domain.AuditEvent{audit}, []domain.OutboxEvent{outbox})
	if err != nil {
		return nil, false, err
	}
	if !created {
		return stored, false, nil
	}
	if _, err := r.workflows.StartRelease(ctx, workflowcore.ReleaseInput{Run: *stored}); err != nil {
		return stored, created, fmt.Errorf("start temporal release workflow: %w", err)
	}
	if err := r.store.MarkOutboxPublished(outbox.ID, r.now().UTC()); err != nil {
		return stored, created, fmt.Errorf("mark workflow request published: %w", err)
	}
	return stored, created, nil
}

func (r *Releases) Get(id string) (*domain.ReleaseRun, error) { return r.store.GetRun(id) }
func (r *Releases) Events(id string) ([]domain.AuditEvent, error) {
	if _, err := r.store.GetRun(id); err != nil {
		return nil, err
	}
	return r.store.AuditEvents(id)
}

func validateRequest(request domain.ReleaseRequest) error {
	switch {
	case request.RequestID == "":
		return fmt.Errorf("%w: request_id is required", ErrInvalidRequest)
	case request.ReleaseVersion == "":
		return fmt.Errorf("%w: release_version is required", ErrInvalidRequest)
	case request.RequestedBy == "":
		return fmt.Errorf("%w: requested_by is required", ErrInvalidRequest)
	case request.Agent.ID == "":
		return fmt.Errorf("%w: delegated_agent.id is required", ErrInvalidRequest)
	default:
		return nil
	}
}
func newID(prefix string) string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(value)
}

var _ ReleaseService = (*Releases)(nil)
