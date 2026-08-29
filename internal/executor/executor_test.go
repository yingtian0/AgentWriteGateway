package executor

import (
	"context"
	"testing"

	"agentwritegateway/pkg/adapter"
)

func TestMockModelsIdempotentDeployAndVerifiedRollback(t *testing.T) {
	service := NewMock(map[string]MockBehavior{"identity": {VerifyStatus: adapter.VerificationFail}})
	request := DeployRequest{Service: "identity", Environment: "staging", DesiredVersion: "v1", IdempotencyKey: "key-1"}
	first, err := service.Deploy(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Deploy(context.Background(), request)
	if err != nil || first.ExternalID != second.ExternalID || service.DeployCalls(request.IdempotencyKey) != 1 {
		t.Fatalf("first=%#v second=%#v calls=%d err=%v", first, second, service.DeployCalls(request.IdempotencyKey), err)
	}
	verification, err := service.Verify(context.Background(), first)
	if err != nil || verification.Outcome() != adapter.VerificationFail {
		t.Fatalf("verification=%#v err=%v", verification, err)
	}
	rolledBack, err := service.Rollback(context.Background(), first)
	if err != nil || rolledBack.ExternalID == "" {
		t.Fatalf("rollback=%#v err=%v", rolledBack, err)
	}
	rollbackVerification, err := service.Verify(context.Background(), rolledBack)
	if err != nil || rollbackVerification.Outcome() != adapter.VerificationPass {
		t.Fatalf("verification=%#v err=%v", rollbackVerification, err)
	}
}

func TestMockClassifiesDeployVerifyAndRollbackFailures(t *testing.T) {
	service := NewMock(map[string]MockBehavior{"deploy": {DeployError: true}, "verify": {VerifyError: true}, "rollback": {RollbackError: true}})
	if _, err := service.Deploy(context.Background(), DeployRequest{Service: "deploy", IdempotencyKey: "deploy"}); err == nil {
		t.Fatal("deploy failure missing")
	}
	verified, _ := service.Deploy(context.Background(), DeployRequest{Service: "verify", IdempotencyKey: "verify"})
	if _, err := service.Verify(context.Background(), verified); err == nil {
		t.Fatal("verify failure missing")
	}
	rollback, _ := service.Deploy(context.Background(), DeployRequest{Service: "rollback", IdempotencyKey: "rollback"})
	if _, err := service.Rollback(context.Background(), rollback); err == nil {
		t.Fatal("rollback failure missing")
	}
}
