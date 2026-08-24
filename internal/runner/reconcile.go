package runner

import (
	"context"
	"fmt"

	"agentwritegateway/internal/store"
	"agentwritegateway/pkg/protocol"
)

// Reconcile resolves already-reserved writes. It may run while disconnected,
// but it never starts a new adapter write.
func (r *Runner) Reconcile(ctx context.Context, reconciler Reconciler, limit int) error {
	if r.Journal == nil || reconciler == nil {
		return fmt.Errorf("reconciliation unavailable")
	}
	if r.TenantID == "" || r.RunnerGroup == "" {
		return fmt.Errorf("runner identity unavailable")
	}
	records, err := r.Journal.PendingRunnerActions(ctx, r.TenantID, r.RunnerGroup, limit)
	if err != nil {
		return err
	}
	for _, record := range records {
		result, found, err := reconciler.Reconcile(ctx, record.IdempotencyKey)
		if err != nil || !found {
			continue
		}
		now := r.now()
		record.Status = store.RunnerActionSucceeded
		record.Result = protocol.Result{ProtocolVersion: protocol.VersionV1Alpha1, GrantID: record.GrantID, RunID: record.RunID, StepID: record.StepID, Status: protocol.ResultSucceeded, ExternalExecutionID: result.ExternalExecutionID, CompletedAt: result.CompletedAt}
		record.UpdatedAt = now
		if err := r.Journal.CompleteRunnerAction(ctx, record, record.StateVersion, journalAudit(auditID(record.GrantID, "reconcile", record.StateVersion+1), record.GrantID, "runner.action.reconcile", "SUCCEEDED", now, nil)); err != nil {
			return err
		}
	}
	return nil
}
