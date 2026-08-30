package scenario

import (
	"testing"

	"agentwritegateway/internal/domain"
	"agentwritegateway/internal/executor"
	workflowcore "agentwritegateway/internal/workflow"
	"agentwritegateway/pkg/adapter"
)

func TestRollbackAPIFailureEscalatesWithDownstreamStopped(t *testing.T) {
	run, memory, releaseExecutor := scenarioWorkflowFixture(t, map[string]executor.MockBehavior{"identity": {VerifyStatus: adapter.VerificationFail, RollbackError: true}})
	environment := scenarioEnvironment(memory, releaseExecutor)
	environment.ExecuteWorkflow(workflowcore.ReleaseWorkflow, workflowcore.ReleaseInput{Run: run})
	result := scenarioResult(t, environment)
	if result.Status != domain.RunEscalated || result.Steps[0].Status != domain.StepEscalated || result.Steps[1].Status != domain.StepCancelled {
		t.Fatalf("run=%s first=%s downstream=%s", result.Status, result.Steps[0].Status, result.Steps[1].Status)
	}
}

func TestRollbackVerificationFailureEscalates(t *testing.T) {
	run, memory, releaseExecutor := scenarioWorkflowFixture(t, map[string]executor.MockBehavior{"identity": {VerifyStatus: adapter.VerificationFail, RollbackVerifyStatus: adapter.VerificationFail}})
	environment := scenarioEnvironment(memory, releaseExecutor)
	environment.ExecuteWorkflow(workflowcore.ReleaseWorkflow, workflowcore.ReleaseInput{Run: run})
	result := scenarioResult(t, environment)
	if result.Status != domain.RunEscalated || result.Steps[0].RollbackVerification == nil || result.Steps[0].RollbackVerification.Status != domain.VerificationFail {
		t.Fatalf("result=%#v", result)
	}
}
