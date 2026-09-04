package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"themisy/internal/domain"
	"themisy/pkg/protocol"
)

func TestMemoryOptimisticLockAndProjectionRebuild(t *testing.T) {
	now := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	memory := NewMemory()
	run := &domain.ReleaseRun{ID: "run-1", RequestID: "request-1", WorkflowID: "run-1", Status: domain.RunPending, StateVersion: 1, CreatedAt: now, UpdatedAt: now}
	if _, created, err := memory.CreateRunAtomic(run, []domain.AuditEvent{testAudit("audit-1", now)}, nil); err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	stale := cloneRun(run)
	run.Status = domain.RunRunning
	run.UpdatedAt = now.Add(time.Minute)
	if err := memory.UpdateRunAtomic(run, 1, []domain.AuditEvent{testAudit("audit-2", now)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := memory.UpdateRun(stale, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
	memory.mu.Lock()
	memory.runs[run.ID].Status = domain.RunFailed
	memory.mu.Unlock()
	if err := memory.RebuildProjection(run.ID); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := memory.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Status != domain.RunRunning {
		t.Fatalf("status=%s, want RUNNING", rebuilt.Status)
	}
}

func TestMemoryGrantDispatchLeaseAckResultAndOutbox(t *testing.T) {
	now := time.Date(2026, 9, 4, 1, 0, 0, 0, time.UTC)
	memory := NewMemory()
	run := &domain.ReleaseRun{ID: "run-grant", RequestID: "request-grant", WorkflowID: "run-grant", StateVersion: 1, CreatedAt: now, UpdatedAt: now}
	if _, _, err := memory.CreateRun(run); err != nil {
		t.Fatal(err)
	}
	grant := protocol.ActionGrant{GrantID: "grant-1", RunID: run.ID, StepID: "payments", TenantID: "tenant-1", RunnerGroup: "prod", IdempotencyKey: "deploy-1", ExpiresAt: now.Add(time.Minute)}
	record := GrantDispatchRecord{Grant: grant, CreatedAt: now, UpdatedAt: now}
	audit := testAudit("grant-audit", now)
	outbox := domain.OutboxEvent{ID: "grant-outbox", AggregateType: "action_grant", AggregateID: grant.GrantID, EventType: "grant.dispatch.requested", CreatedAt: now, AvailableAt: now}
	created, fresh, err := memory.CreateGrantDispatch(context.Background(), record, audit, outbox)
	if err != nil || !fresh || created.Status != GrantDispatchPending {
		t.Fatalf("create=%#v fresh=%v err=%v", created, fresh, err)
	}
	if _, fresh, err := memory.CreateGrantDispatch(context.Background(), record, audit, outbox); err != nil || fresh {
		t.Fatalf("duplicate fresh=%v err=%v", fresh, err)
	}
	claimed, err := memory.ClaimGrantDispatch(context.Background(), "tenant-1", "prod", "runner-1", "secret-token", now, now.Add(30*time.Second))
	if err != nil || claimed.DeliveryToken != "secret-token" || claimed.Status != GrantDispatchLeased {
		t.Fatalf("claim=%#v err=%v", claimed, err)
	}
	if _, err := memory.ClaimGrantDispatch(context.Background(), "tenant-1", "prod", "runner-2", "other-token", now, now.Add(30*time.Second)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second runner claim=%v", err)
	}
	acked, err := memory.AcknowledgeGrantDispatch(context.Background(), grant.GrantID, "runner-1", "secret-token", now.Add(time.Second), now.Add(time.Minute))
	if err != nil || acked.Status != GrantDispatchAcked {
		t.Fatalf("ack=%#v err=%v", acked, err)
	}
	result := protocol.Result{ProtocolVersion: protocol.VersionV1Alpha1, GrantID: grant.GrantID, RunID: run.ID, StepID: grant.StepID, Status: protocol.ResultSucceeded, CompletedAt: now.Add(2 * time.Second)}
	completed, err := memory.CompleteGrantDispatch(context.Background(), grant.GrantID, "runner-1", "secret-token", result, now.Add(2*time.Second), testAudit("grant-result", now))
	if err != nil || completed.Status != GrantDispatchSucceeded || completed.Result != result {
		t.Fatalf("complete=%#v err=%v", completed, err)
	}
	pending, err := memory.PendingOutboxByType("grant.dispatch.requested", 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending dispatch outbox=%d err=%v", len(pending), err)
	}
}

func TestMemoryExecutionIdempotencyAndOutboxAtomicity(t *testing.T) {
	now := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	memory := NewMemory()
	run := &domain.ReleaseRun{ID: "run-1", RequestID: "request-1", WorkflowID: "run-1", StateVersion: 1, CreatedAt: now, UpdatedAt: now}
	if _, _, err := memory.CreateRun(run); err != nil {
		t.Fatal(err)
	}
	record := ExecutionRecord{ID: "execution-1", RunID: run.ID, Service: "identity", Adapter: "mock", IdempotencyKey: "same-key", CreatedAt: now, UpdatedAt: now}
	audit := testAudit("audit-execution", now)
	outbox := domain.OutboxEvent{ID: "outbox-execution", AggregateType: "execution", AggregateID: record.ID, EventType: "execution.reserved", CreatedAt: now, AvailableAt: now}
	if _, created, err := memory.ReserveExecution(record, audit, outbox); err != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, err)
	}
	if existing, created, err := memory.ReserveExecution(record, audit, outbox); err != nil || created || existing.ID != record.ID {
		t.Fatalf("duplicate: existing=%#v created=%v err=%v", existing, created, err)
	}
	events, err := memory.AuditEvents(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events=%d, want 1", len(events))
	}
	pending, err := memory.PendingOutbox(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("outbox events=%d, want projection+execution", len(pending))
	}
}

func testAudit(id string, now time.Time) domain.AuditEvent {
	return domain.AuditEvent{ID: id, CorrelationID: "run-1", ActorType: "system", ActorID: "test", Action: "test", ResourceType: "release_run", ResourceID: "run-1", Result: "OK", Timestamp: now}
}
