package application

import (
	"context"
	"errors"
	"fmt"

	"themisy/internal/domain"
	workflowcore "themisy/internal/workflow"
)

var ErrApproval = errors.New("approval rejected")

func (r *Releases) DecideApproval(ctx context.Context, runID, approvalID, actor string, roles []string, approve bool) (*domain.ReleaseRun, error) {
	if actor == "" {
		return nil, fmt.Errorf("%w: approval actor is required", ErrApproval)
	}
	run, err := r.store.GetRun(runID)
	if err != nil {
		return nil, err
	}
	var target *domain.Approval
	for index := range run.Steps {
		if run.Steps[index].Approval != nil && run.Steps[index].Approval.ID == approvalID {
			target = run.Steps[index].Approval
			break
		}
	}
	if target == nil || target.Status != domain.ApprovalPending {
		return nil, fmt.Errorf("%w: pending approval not found", ErrApproval)
	}
	if target.PlanHash != run.Plan.Hash {
		return nil, fmt.Errorf("%w: plan changed after approval request", ErrApproval)
	}
	if !r.now().Before(target.ExpiresAt) {
		return nil, fmt.Errorf("%w: approval expired", ErrApproval)
	}
	if approve && actor == run.RequestedBy {
		return nil, fmt.Errorf("%w: requester cannot approve their own release", ErrApproval)
	}
	if approve && !containsAll(roles, target.RequiredRoles) {
		return nil, fmt.Errorf("%w: approver lacks required roles %v", ErrApproval, target.RequiredRoles)
	}
	if err := r.workflows.SignalApproval(ctx, run.WorkflowID, workflowcore.ApprovalSignal{ApprovalID: approvalID, Actor: actor, Roles: roles, Approve: approve}); err != nil {
		return nil, err
	}
	return r.store.GetRun(runID)
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

func (r *Releases) Cancel(runID, actor string) (*domain.ReleaseRun, error) {
	return r.control(context.Background(), runID, actor, "cancel")
}
func (r *Releases) Pause(runID, actor string) (*domain.ReleaseRun, error) {
	return r.control(context.Background(), runID, actor, "pause")
}
func (r *Releases) Resume(ctx context.Context, runID, actor string) (*domain.ReleaseRun, error) {
	return r.control(ctx, runID, actor, "resume")
}

func (r *Releases) control(ctx context.Context, runID, actor, action string) (*domain.ReleaseRun, error) {
	if actor == "" {
		return nil, fmt.Errorf("actor is required")
	}
	run, err := r.store.GetRun(runID)
	if err != nil {
		return nil, err
	}
	signal := workflowcore.ControlSignal{Actor: actor}
	switch action {
	case "cancel":
		err = r.workflows.Cancel(ctx, run.WorkflowID, signal)
	case "pause":
		err = r.workflows.Pause(ctx, run.WorkflowID, signal)
	case "resume":
		err = r.workflows.Resume(ctx, run.WorkflowID, signal)
	}
	if err != nil {
		return nil, err
	}
	return r.store.GetRun(runID)
}
