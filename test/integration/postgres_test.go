//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"themisy/internal/domain"
	"themisy/internal/store"
	postgresstore "themisy/internal/store/postgres"

	"github.com/jackc/pgx/v5"
)

func TestPostgresMigrationAndStoreContract(t *testing.T) {
	requireIntegration(t)
	databaseURL := integrationDatabaseURL()
	eventually(t, 30*time.Second, func() error {
		connection, err := pgx.Connect(context.Background(), databaseURL)
		if err != nil {
			return err
		}
		connection.Close(context.Background())
		return nil
	})
	if err := postgresstore.Migrate(databaseURL, true); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	var table *string
	if err := connection.QueryRow(context.Background(), `SELECT to_regclass('public.release_runs')::text`).Scan(&table); err != nil {
		t.Fatal(err)
	}
	connection.Close(context.Background())
	if table != nil {
		t.Fatalf("release_runs remains after down migration: %v", *table)
	}
	if err := postgresstore.Migrate(databaseURL, false); err != nil {
		t.Fatal(err)
	}
	persistent, err := postgresstore.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer persistent.Close()
	now := time.Now().UTC()
	plan := domain.ReleasePlan{ID: "integration-plan-" + now.Format("150405.000000000"), Hash: "sha256:integration", PlanHash: "sha256:integration", Environment: domain.EnvironmentStaging, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := persistent.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	storedPlan, err := persistent.GetPlan(plan.ID)
	if err != nil || storedPlan.PlanHash != plan.PlanHash {
		t.Fatalf("stored plan=%#v err=%v", storedPlan, err)
	}
	run := &domain.ReleaseRun{ID: "integration-run-" + now.Format("150405.000000000"), RequestID: "integration-request-" + now.Format("150405.000000000"), WorkflowID: "integration-workflow-" + now.Format("150405.000000000"), ReleaseVersion: "release-1", Environment: domain.EnvironmentStaging, RequestedBy: "integration", Status: domain.RunPending, StateVersion: 1, CreatedAt: now, UpdatedAt: now}
	audit := integrationAudit("audit-create-"+run.ID, run.ID, now)
	outbox := domain.OutboxEvent{ID: "outbox-create-" + run.ID, AggregateType: "release_run", AggregateID: run.ID, EventType: "release.created", CreatedAt: now, AvailableAt: now}
	created, wasCreated, err := persistent.CreateRunAtomic(run, []domain.AuditEvent{audit}, []domain.OutboxEvent{outbox})
	if err != nil || !wasCreated {
		t.Fatalf("create: created=%v err=%v", wasCreated, err)
	}
	duplicate, wasCreated, err := persistent.CreateRunAtomic(run, nil, nil)
	if err != nil || wasCreated || duplicate.ID != created.ID {
		t.Fatalf("duplicate request created another run: %#v %v %v", duplicate, wasCreated, err)
	}
	stale := *created
	created.Status = domain.RunRunning
	created.UpdatedAt = now.Add(time.Second)
	if err := persistent.UpdateRunAtomic(created, 1, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := persistent.UpdateRun(&stale, 1); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("got %v, want conflict", err)
	}
	record := store.ExecutionRecord{ID: "execution-" + run.ID, RunID: run.ID, Service: "identity", Adapter: "mock", IdempotencyKey: "key-" + run.ID, CreatedAt: now, UpdatedAt: now}
	if _, reserved, err := persistent.ReserveExecution(record, integrationAudit("audit-reserve-"+run.ID, run.ID, now), domain.OutboxEvent{ID: "outbox-reserve-" + run.ID, AggregateType: "execution", AggregateID: record.ID, EventType: "execution.reserved", CreatedAt: now, AvailableAt: now}); err != nil || !reserved {
		t.Fatalf("reserve=%v err=%v", reserved, err)
	}
	if _, reserved, err := persistent.ReserveExecution(record, integrationAudit("ignored", run.ID, now), domain.OutboxEvent{ID: "ignored"}); err != nil || reserved {
		t.Fatalf("duplicate reserve=%v err=%v", reserved, err)
	}
	atomicAudit := integrationAudit("audit-atomic-"+run.ID, run.ID, now)
	if err := persistent.AppendAudit(atomicAudit); err != nil {
		t.Fatal(err)
	}
	before, err := persistent.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	before.Status = domain.RunFailed
	before.UpdatedAt = now.Add(2 * time.Second)
	if err := persistent.UpdateRunAtomic(before, before.StateVersion, []domain.AuditEvent{atomicAudit}, []domain.OutboxEvent{{ID: "outbox-must-rollback-" + run.ID, AggregateType: "release_run", AggregateID: run.ID, EventType: "should.rollback", CreatedAt: now, AvailableAt: now}}); err == nil {
		t.Fatal("duplicate audit should abort transaction")
	}
	after, err := persistent.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != domain.RunRunning || after.StateVersion != 2 {
		t.Fatalf("atomic rollback failed: status=%s version=%d", after.Status, after.StateVersion)
	}
	connection, err = pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(context.Background(), `UPDATE release_runs SET status='FAILED', payload=jsonb_set(payload,'{status}','"FAILED"') WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	connection.Close(context.Background())
	if err := persistent.RebuildProjection(run.ID); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := persistent.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Status != domain.RunRunning {
		t.Fatalf("rebuilt status=%s", rebuilt.Status)
	}
	if err := postgresstore.Migrate(databaseURL, true); err != nil {
		t.Fatal(err)
	}
	if err := postgresstore.Migrate(databaseURL, false); err != nil {
		t.Fatal(err)
	}
}

func integrationAudit(id, runID string, at time.Time) domain.AuditEvent {
	return domain.AuditEvent{ID: id, CorrelationID: runID, ActorType: "system", ActorID: "integration", Action: "integration", ResourceType: "release_run", ResourceID: runID, Result: "OK", Timestamp: at}
}
func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("THEMISY_INTEGRATION") != "1" {
		t.Fatal("set THEMISY_INTEGRATION=1 and start deploy/compose dependencies")
	}
}
func integrationDatabaseURL() string {
	if value := os.Getenv("THEMISY_DATABASE_URL"); value != "" {
		return value
	}
	return "postgres://themisy:themisy@localhost:5432/themisy?sslmode=disable"
}
func eventually(t *testing.T, timeout time.Duration, check func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return
		} else {
			last = err
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-time.After(time.Until(deadline)):
		}
	}
	t.Fatalf("condition not met: %v", last)
}
