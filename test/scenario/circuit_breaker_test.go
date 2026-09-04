package scenario

import (
	"errors"
	"testing"

	"themisy/internal/scheduler"
)

func TestProductionCircuitBreakerStopsEveryNewStepAfterThreshold(t *testing.T) {
	coordinator, err := scheduler.New(scheduler.DefaultLimits(), scheduler.CircuitPolicy{MinimumSamples: 4, WindowSize: 10, ErrorRate: .5})
	if err != nil {
		t.Fatal(err)
	}
	step := scheduler.Step{ID: "canary", Tenant: "tenant-a", Environment: "production", Region: "ap-northeast-1", Cluster: "production-a", Team: "payments", RiskTier: "critical"}
	for index, failed := range []bool{true, false, true, false} {
		step.ID = string(rune('a' + index))
		results := coordinator.Dispatch([]scheduler.Step{step}, scheduler.Capacity{RunnerAvailable: 1, AdapterRemaining: 1, QueueLimit: 1})
		if results[0].Err != nil {
			t.Fatalf("sample %d was blocked before threshold: %v", index, results[0].Err)
		}
		coordinator.Complete(results[0], failed)
	}
	if state := coordinator.CircuitState("tenant-a"); !state.Open {
		t.Fatalf("state=%#v", state)
	}
	step.ID = "after-open"
	results := coordinator.Dispatch([]scheduler.Step{step}, scheduler.Capacity{RunnerAvailable: 10, AdapterRemaining: 10, QueueLimit: 10})
	newDispatches := 0
	for _, result := range results {
		if result.Err == nil && len(result.BlockedBy) == 0 {
			newDispatches++
		}
		if !errors.Is(result.Err, scheduler.ErrCircuitOpen) {
			t.Fatalf("result=%#v", result)
		}
	}
	if newDispatches != 0 {
		t.Fatalf("new production dispatches after breaker open=%d", newDispatches)
	}
}

func TestCircuitBreakerDoesNotPermitAgentCloseOperation(t *testing.T) {
	breaker, err := scheduler.NewCircuitBreaker(scheduler.CircuitPolicy{MinimumSamples: 1, WindowSize: 1, ErrorRate: 1})
	if err != nil {
		t.Fatal(err)
	}
	breaker.Record("tenant-a", "production", true)
	if err := breaker.Allow("tenant-a", "production"); !errors.Is(err, scheduler.ErrCircuitOpen) {
		t.Fatalf("open breaker became dispatchable: %v", err)
	}
	// There is intentionally no Close/Reset method on CircuitBreaker and no
	// matching application or MCP use case. Recovery is operator-only.
}
