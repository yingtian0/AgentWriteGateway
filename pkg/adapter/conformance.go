package adapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"agentwritegateway/pkg/credentials"
)

// ConformanceInput is intended for an explicitly isolated staging account.
// RunConformance performs exactly one deploy and one rollback write.
type ConformanceInput struct {
	Adapter    DeployAdapter
	Credential credentials.Credential
	Deploy     DeployRequest
	Now        func() time.Time
}

type ConformanceReport struct {
	Deployment Deployment
	Rollback   Deployment
	Reconciled ReconcileResult
}

func RunConformance(ctx context.Context, input ConformanceInput) (ConformanceReport, error) {
	if input.Now == nil {
		input.Now = time.Now
	}
	if input.Adapter == nil || !input.Credential.ValidAt(input.Now()) {
		return ConformanceReport{}, errors.New("adapter and valid staging credential are required")
	}
	if err := ValidateDeployRequest(input.Deploy); err != nil {
		return ConformanceReport{}, err
	}
	deployed, err := input.Adapter.Deploy(ctx, input.Deploy, input.Credential)
	if err != nil {
		return ConformanceReport{}, fmt.Errorf("conformance deploy: %w", err)
	}
	if deployed.ExternalExecutionID == "" {
		return ConformanceReport{}, errors.New("deploy adapter returned no external execution ID")
	}
	reconciled, err := input.Adapter.Reconcile(ctx, ReconcileRequest{IdempotencyKey: input.Deploy.IdempotencyKey, DispatchedAt: input.Now().Add(-time.Minute)}, input.Credential)
	if err != nil || reconciled.Status == ReconcileNotFound {
		return ConformanceReport{}, fmt.Errorf("conformance reconcile: status=%s err=%w", reconciled.Status, err)
	}
	rollbackRequest := RollbackRequest{Target: input.Deploy.Target, OriginalDeployment: deployed, IdempotencyKey: input.Deploy.IdempotencyKey + "/rollback"}
	rolledBack, err := input.Adapter.Rollback(ctx, rollbackRequest, input.Credential)
	if err != nil {
		return ConformanceReport{}, fmt.Errorf("conformance rollback: %w", err)
	}
	if rolledBack.ExternalExecutionID == "" {
		return ConformanceReport{}, errors.New("rollback adapter returned no external execution ID")
	}
	return ConformanceReport{Deployment: deployed, Rollback: rolledBack, Reconciled: reconciled}, nil
}
