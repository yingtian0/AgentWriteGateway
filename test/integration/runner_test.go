//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"agentwritegateway/internal/store"
	postgresstore "agentwritegateway/internal/store/postgres"
)

func TestRunnerJournalSurvivesReconnectAndDeduplicatesNonce(t *testing.T) {
	requireIntegration(t)
	ctx := context.Background()
	databaseURL := integrationDatabaseURL()
	if err := postgresstore.Migrate(databaseURL, false); err != nil {
		t.Fatal(err)
	}
	persistent, err := postgresstore.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	suffix := now.Format("150405.000000000")
	record := store.RunnerActionRecord{GrantID: "runner-grant-" + suffix, RunID: "runner-run-" + suffix, StepID: "step-1", TenantID: "tenant-integration", RunnerGroup: "runner-integration", Nonce: "nonce-" + suffix, IdempotencyKey: "key-" + suffix, RequestHash: "sha256:integration", CreatedAt: now, UpdatedAt: now}
	audit := integrationAudit("runner-audit-"+suffix, record.GrantID, now)
	reserved, created, err := persistent.ReserveRunnerAction(ctx, record, audit)
	if err != nil || !created {
		t.Fatalf("reserve created=%v err=%v", created, err)
	}
	persistent.Close()

	reconnected, err := postgresstore.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	loaded, err := reconnected.GetRunnerAction(ctx, record.TenantID, record.RunnerGroup, record.Nonce)
	if err != nil || loaded.RequestHash != record.RequestHash {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
	if _, created, err := reconnected.ReserveRunnerAction(ctx, record, integrationAudit("ignored-"+suffix, record.GrantID, now)); err != nil || created {
		t.Fatalf("replay created=%v err=%v", created, err)
	}
	reserved.Status, reserved.UpdatedAt = store.RunnerActionSucceeded, now.Add(time.Second)
	if err := reconnected.CompleteRunnerAction(ctx, reserved, reserved.StateVersion, integrationAudit("runner-complete-"+suffix, record.GrantID, now)); err != nil {
		t.Fatal(err)
	}
	completed, err := reconnected.GetRunnerAction(ctx, record.TenantID, record.RunnerGroup, record.Nonce)
	if err != nil || completed.Status != store.RunnerActionSucceeded {
		t.Fatalf("completed=%#v err=%v", completed, err)
	}
}
