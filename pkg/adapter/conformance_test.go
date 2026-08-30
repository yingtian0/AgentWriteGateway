package adapter

import (
	"context"
	"testing"
	"time"

	"agentwritegateway/pkg/credentials"
)

func TestRunConformanceRequiresExternalIDsAndReconciliation(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	probe := &conformanceAdapter{now: now}
	report, err := RunConformance(context.Background(), ConformanceInput{Adapter: probe, Credential: credentials.New(map[string][]byte{"token": []byte("test")}, now.Add(time.Hour)), Deploy: DeployRequest{Target: Target{Service: "identity", Environment: "staging"}, ArtifactDigest: digest, IdempotencyKey: "staging-key"}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if report.Deployment.ExternalExecutionID == "" || report.Rollback.ExternalExecutionID == "" || probe.deployCalls != 1 || probe.rollbackCalls != 1 || probe.reconcileCalls != 1 {
		t.Fatalf("report=%#v calls=%d/%d/%d", report, probe.deployCalls, probe.rollbackCalls, probe.reconcileCalls)
	}
}

type conformanceAdapter struct {
	now                                        time.Time
	deployCalls, rollbackCalls, reconcileCalls int
}

func (a *conformanceAdapter) Name() string    { return "test" }
func (a *conformanceAdapter) Version() string { return "test/v1" }
func (a *conformanceAdapter) Deploy(context.Context, DeployRequest, credentials.Credential) (Deployment, error) {
	a.deployCalls++
	return Deployment{ExternalExecutionID: "deploy-1", CorrelationID: "correlation", StartedAt: a.now}, nil
}
func (a *conformanceAdapter) Rollback(context.Context, RollbackRequest, credentials.Credential) (Deployment, error) {
	a.rollbackCalls++
	return Deployment{ExternalExecutionID: "rollback-1", CorrelationID: "rollback-correlation", StartedAt: a.now}, nil
}
func (a *conformanceAdapter) Reconcile(context.Context, ReconcileRequest, credentials.Credential) (ReconcileResult, error) {
	a.reconcileCalls++
	return ReconcileResult{Status: ReconcileSucceeded, Deployment: Deployment{ExternalExecutionID: "deploy-1", StartedAt: a.now}}, nil
}
