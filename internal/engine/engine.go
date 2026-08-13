package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"agentwritegateway/internal/domain"
	"agentwritegateway/internal/executor"
	"agentwritegateway/internal/planner"
	"agentwritegateway/internal/policy"
	"agentwritegateway/internal/store"
)

var (
	ErrInvalidRequest = errors.New("invalid release request")
	ErrApproval       = errors.New("approval rejected")
)

type Engine struct {
	planner  *planner.Planner
	policy   *policy.Engine
	executor executor.ReleaseExecutor
	store    store.Store
	now      func() time.Time
}

func New(p *planner.Planner, pe *policy.Engine, ex executor.ReleaseExecutor, st store.Store) *Engine {
	return &Engine{planner: p, policy: pe, executor: ex, store: st, now: time.Now}
}

func (e *Engine) Services() []domain.Service { return e.planner.Services() }

func (e *Engine) Plan(request domain.ReleaseRequest) (domain.ReleasePlan, error) {
	if err := validateRequest(request); err != nil {
		return domain.ReleasePlan{}, err
	}
	return e.planner.Plan(request)
}

func (e *Engine) PlanIntent(intent domain.ReleaseIntent) (domain.ReleasePlan, error) {
	request := planner.LegacyRequestFromIntent(intent)
	if err := validateRequest(request); err != nil {
		return domain.ReleasePlan{}, err
	}
	return e.planner.PlanIntent(intent)
}

func (e *Engine) Start(ctx context.Context, request domain.ReleaseRequest) (*domain.ReleaseRun, bool, error) {
	plan, err := e.Plan(request)
	if err != nil {
		return nil, false, err
	}
	return e.startPlanned(ctx, request, plan)
}

func (e *Engine) StartIntent(ctx context.Context, intent domain.ReleaseIntent) (*domain.ReleaseRun, bool, error) {
	plan, err := e.PlanIntent(intent)
	if err != nil {
		return nil, false, err
	}
	return e.startPlanned(ctx, planner.LegacyRequestFromIntent(intent), plan)
}

func (e *Engine) startPlanned(ctx context.Context, request domain.ReleaseRequest, plan domain.ReleasePlan) (*domain.ReleaseRun, bool, error) {
	if err := e.planner.ValidatePlan(plan, e.now()); err != nil {
		return nil, false, fmt.Errorf("validate release plan before execution: %w", err)
	}
	now := e.now().UTC()
	run := &domain.ReleaseRun{
		ID: newID("run"), RequestID: request.RequestID,
		ReleaseVersion: request.ReleaseVersion, Environment: request.Environment,
		RequestedBy: request.RequestedBy, Agent: request.Agent, Plan: plan,
		Status: domain.RunPending, StateVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	changes := make(map[string]domain.Change, len(request.Changes))
	for _, change := range request.Changes {
		changes[change.Service] = change
	}
	for _, phase := range plan.Phases {
		for _, planned := range phase.Steps {
			run.Steps = append(run.Steps, domain.ReleaseStep{
				Service: planned.Service, Phase: phase.Number,
				Status: domain.StepPending, Change: changes[planned.Service],
			})
		}
	}
	createdRun, created, err := e.store.CreateRun(run)
	if err != nil || !created {
		return createdRun, created, err
	}
	if err := e.audit(run, "user", request.RequestedBy, "release.start", "release_run", run.ID, "ACCEPTED", map[string]any{"plan_hash": plan.Hash}); err != nil {
		return nil, true, fmt.Errorf("write release audit event: %w", err)
	}
	if err := e.advance(ctx, run); err != nil {
		return nil, true, err
	}
	updated, err := e.store.GetRun(run.ID)
	return updated, true, err
}

func (e *Engine) Get(id string) (*domain.ReleaseRun, error) { return e.store.GetRun(id) }

func (e *Engine) Events(id string) ([]domain.AuditEvent, error) {
	if _, err := e.store.GetRun(id); err != nil {
		return nil, err
	}
	return e.store.AuditEvents(id)
}

func (e *Engine) DecideApproval(ctx context.Context, runID, approvalID, actor string, roles []string, approve bool) (*domain.ReleaseRun, error) {
	run, err := e.store.GetRun(runID)
	if err != nil {
		return nil, err
	}
	expectedVersion := run.StateVersion
	if actor == "" {
		return nil, fmt.Errorf("%w: approval actor is required", ErrApproval)
	}
	var target *domain.ReleaseStep
	for i := range run.Steps {
		if run.Steps[i].Approval != nil && run.Steps[i].Approval.ID == approvalID {
			target = &run.Steps[i]
			break
		}
	}
	if target == nil || target.Approval.Status != domain.ApprovalPending {
		return nil, fmt.Errorf("%w: pending approval not found", ErrApproval)
	}
	if target.Approval.PlanHash != run.Plan.Hash {
		return nil, fmt.Errorf("%w: plan changed after approval request", ErrApproval)
	}
	if !e.now().Before(target.Approval.ExpiresAt) {
		target.Approval.Status = domain.ApprovalExpired
		target.Status = domain.StepBlocked
		run.Status = domain.RunBlocked
		run.UpdatedAt = e.now().UTC()
		if err := e.store.UpdateRun(run, expectedVersion); err != nil {
			return nil, err
		}
		return e.store.GetRun(runID)
	}
	if approve && !containsAll(roles, target.Approval.RequiredRoles) {
		return nil, fmt.Errorf("%w: approver lacks required roles %v", ErrApproval, target.Approval.RequiredRoles)
	}
	if approve && actor == run.RequestedBy {
		return nil, fmt.Errorf("%w: requester cannot approve their own release", ErrApproval)
	}
	now := e.now().UTC()
	target.Approval.DecidedBy = actor
	target.Approval.DecidedAt = &now
	if approve {
		target.Approval.Status = domain.ApprovalApproved
		target.Status = domain.StepPending
		run.Status = domain.RunRunning
	} else {
		target.Approval.Status = domain.ApprovalDenied
		target.Status = domain.StepBlocked
		target.Failure = "approval denied"
		run.Status = domain.RunBlocked
	}
	run.UpdatedAt = now
	if err := e.audit(run, "user", actor, "approval.decide", "approval", approvalID, string(target.Approval.Status), nil); err != nil {
		return nil, fmt.Errorf("write approval audit event: %w", err)
	}
	if err := e.store.UpdateRun(run, expectedVersion); err != nil {
		return nil, err
	}
	if approve {
		run, err = e.store.GetRun(runID)
		if err != nil {
			return nil, err
		}
		if err := e.advance(ctx, run); err != nil {
			return nil, err
		}
	}
	return e.store.GetRun(runID)
}

func (e *Engine) Cancel(runID, actor string) (*domain.ReleaseRun, error) {
	run, err := e.store.GetRun(runID)
	if err != nil {
		return nil, err
	}
	if run.Status == domain.RunSucceeded || run.Status == domain.RunCancelled {
		return run, nil
	}
	expectedVersion := run.StateVersion
	run.Status = domain.RunCancelled
	for i := range run.Steps {
		if run.Steps[i].Status == domain.StepPending || run.Steps[i].Status == domain.StepWaitingApproval {
			run.Steps[i].Status = domain.StepCancelled
		}
	}
	run.UpdatedAt = e.now().UTC()
	if err := e.audit(run, "user", actor, "release.cancel", "release_run", run.ID, "CANCELLED", nil); err != nil {
		return nil, fmt.Errorf("write cancellation audit event: %w", err)
	}
	if err := e.store.UpdateRun(run, expectedVersion); err != nil {
		return nil, err
	}
	return e.store.GetRun(runID)
}

func (e *Engine) advance(ctx context.Context, run *domain.ReleaseRun) error {
	expectedVersion := run.StateVersion
	run.Status = domain.RunRunning
	succeeded := make(map[string]bool)
	for _, step := range run.Steps {
		if step.Status == domain.StepSucceeded {
			succeeded[step.Service] = true
		}
	}
	selected := make(map[string]bool, len(run.Steps))
	for _, step := range run.Steps {
		selected[step.Service] = true
	}
	services := make(map[string]domain.Service)
	for _, service := range e.planner.Services() {
		services[service.Name] = service
	}

	for i := range run.Steps {
		step := &run.Steps[i]
		if step.Status == domain.StepSucceeded {
			continue
		}
		if step.Status != domain.StepPending {
			continue
		}
		for _, dependency := range services[step.Service].Dependencies {
			if selected[dependency] && !succeeded[dependency] {
				step.Status = domain.StepBlocked
				step.Failure = "selected dependency did not succeed: " + dependency
				run.Status = domain.RunBlocked
				e.cancelDownstream(run, i+1)
				return e.persist(run, expectedVersion)
			}
		}

		decision := e.policy.Evaluate(policy.Input{
			UserID: run.RequestedBy, AgentID: run.Agent.ID, AgentScopes: sorted(run.Agent.Scopes),
			Environment: run.Environment, Service: step.Service,
			CISuccess: step.Change.CISuccess, DependenciesHealthy: step.Change.DependenciesHealthy,
			DestructiveMigration: step.Change.DestructiveMigration, Risk: step.Change.Risk,
		})
		step.Policy = &decision
		if err := e.audit(run, "system", "policy-engine", "policy.evaluate", "release_step", step.Service, string(decision.Decision), map[string]any{"reason_code": decision.ReasonCode, "input_hash": decision.InputHash}); err != nil {
			step.Status = domain.StepBlocked
			step.Failure = "audit unavailable before policy decision"
			run.Status = domain.RunBlocked
			e.cancelDownstream(run, i+1)
			return e.persist(run, expectedVersion)
		}
		if decision.Decision == domain.DecisionDeny {
			step.Status = domain.StepBlocked
			step.Failure = decision.ReasonDetail
			run.Status = domain.RunBlocked
			e.cancelDownstream(run, i+1)
			return e.persist(run, expectedVersion)
		}
		if decision.Decision == domain.DecisionRequireApproval && (step.Approval == nil || step.Approval.Status != domain.ApprovalApproved) {
			if step.Approval == nil {
				now := e.now().UTC()
				step.Approval = &domain.Approval{
					ID: newID("approval"), RequiredRoles: decision.RequiredRoles,
					Status: domain.ApprovalPending, PlanHash: run.Plan.Hash,
					RequestedAt: now, ExpiresAt: now.Add(24 * time.Hour),
				}
			}
			step.Status = domain.StepWaitingApproval
			run.Status = domain.RunWaitingApproval
			if err := e.audit(run, "system", "policy-engine", "approval.request", "approval", step.Approval.ID, "PENDING", map[string]any{"required_roles": step.Approval.RequiredRoles}); err != nil {
				step.Status = domain.StepBlocked
				step.Failure = "audit unavailable before approval request"
				run.Status = domain.RunBlocked
				e.cancelDownstream(run, i+1)
			}
			return e.persist(run, expectedVersion)
		}

		step.Status = domain.StepExecuting
		key := fmt.Sprintf("%s/%s/%s/%s", run.ID, run.Environment, step.Service, step.Change.DesiredVersion)
		if err := e.audit(run, "agent", run.Agent.ID, "deployment.start", "service", step.Service, "AUTHORIZED", map[string]any{"idempotency_key": key, "policy_version": decision.PolicyVersion}); err != nil {
			step.Status = domain.StepBlocked
			step.Failure = "audit unavailable before deployment"
			run.Status = domain.RunBlocked
			e.cancelDownstream(run, i+1)
			return e.persist(run, expectedVersion)
		}
		deployment, err := e.executor.Deploy(ctx, executor.DeployRequest{
			Service: step.Service, Environment: string(run.Environment),
			DesiredVersion: step.Change.DesiredVersion, IdempotencyKey: key,
		})
		if err != nil {
			step.Status = domain.StepBlocked
			step.Failure = "deploy failed: " + err.Error()
			run.Status = domain.RunBlocked
			e.cancelDownstream(run, i+1)
			return e.persist(run, expectedVersion)
		}
		step.Execution = &domain.Execution{
			ID: newID("execution"), IdempotencyKey: key,
			ExternalExecutionID: deployment.ExternalID,
			StartedAt:           deployment.StartedAt, FinishedAt: deployment.FinishedAt,
		}
		step.Status = domain.StepVerifying
		verification, err := e.executor.Verify(ctx, deployment)
		if err != nil {
			step.Status = domain.StepBlocked
			step.Failure = "verification unavailable: " + err.Error()
			run.Status = domain.RunBlocked
			e.cancelDownstream(run, i+1)
			return e.persist(run, expectedVersion)
		}
		step.Verification = &domain.Verification{
			Healthy: verification.Healthy, Reason: verification.Reason,
			ObservedValue: verification.ObservedValue, Threshold: verification.Threshold,
			CheckedAt: e.now().UTC(),
		}
		if !verification.Healthy {
			step.Status = domain.StepRollingBack
			_ = e.audit(run, "system", "verifier", "rollback.start", "service", step.Service, "HEALTH_CHECK_FAILED", nil)
			if err := e.executor.Rollback(ctx, deployment); err != nil {
				step.Status = domain.StepEscalated
				step.Failure = "rollback failed: " + err.Error()
				run.Status = domain.RunEscalated
			} else {
				step.Status = domain.StepRolledBack
				step.Failure = verification.Reason
				run.Status = domain.RunFailed
			}
			e.cancelDownstream(run, i+1)
			return e.persist(run, expectedVersion)
		}
		step.Status = domain.StepSucceeded
		succeeded[step.Service] = true
		_ = e.audit(run, "system", "verifier", "deployment.verify", "service", step.Service, "SUCCEEDED", nil)
	}
	run.Status = domain.RunSucceeded
	return e.persist(run, expectedVersion)
}

func (e *Engine) persist(run *domain.ReleaseRun, expectedVersion int64) error {
	run.UpdatedAt = e.now().UTC()
	return e.store.UpdateRun(run, expectedVersion)
}

func (e *Engine) cancelDownstream(run *domain.ReleaseRun, from int) {
	for i := from; i < len(run.Steps); i++ {
		if run.Steps[i].Status == domain.StepPending {
			run.Steps[i].Status = domain.StepCancelled
		}
	}
}

func (e *Engine) audit(run *domain.ReleaseRun, actorType, actorID, action, resourceType, resourceID, result string, details map[string]any) error {
	return e.store.AppendAudit(domain.AuditEvent{
		ID: newID("audit"), CorrelationID: run.ID, ActorType: actorType,
		ActorID: actorID, DelegatedBy: run.RequestedBy, Action: action,
		ResourceType: resourceType, ResourceID: resourceID, Result: result,
		Details: details, Timestamp: e.now().UTC(),
	})
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

func containsAll(actual, required []string) bool {
	set := make(map[string]bool, len(actual))
	for _, value := range actual {
		set[value] = true
	}
	for _, value := range required {
		if !set[value] {
			return false
		}
	}
	return true
}

func sorted(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func newID(prefix string) string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}
