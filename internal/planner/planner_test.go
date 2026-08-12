package planner

import (
	"errors"
	"testing"

	"agentwritegateway/internal/domain"
)

func TestPlanOrdersDependenciesAndIsDeterministic(t *testing.T) {
	p, err := New([]domain.Service{
		{Name: "gateway", ReleasePhase: 0, Dependencies: []string{"payments"}},
		{Name: "identity", ReleasePhase: 0},
		{Name: "payments", ReleasePhase: 0, Dependencies: []string{"identity"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := domain.ReleaseRequest{
		ReleaseVersion: "v1", Environment: domain.EnvironmentProduction,
		Changes: []domain.Change{
			{Service: "gateway", DesiredVersion: "c"},
			{Service: "identity", DesiredVersion: "a"},
			{Service: "payments", DesiredVersion: "b"},
		},
	}
	plan, err := p.Plan(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Phases) != 3 {
		t.Fatalf("got %d phases, want 3", len(plan.Phases))
	}
	for index, want := range []string{"identity", "payments", "gateway"} {
		if got := plan.Phases[index].Steps[0].Service; got != want {
			t.Fatalf("phase %d service = %q, want %q", index, got, want)
		}
	}

	request.Changes[0], request.Changes[2] = request.Changes[2], request.Changes[0]
	second, err := p.Plan(request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Hash != second.Hash {
		t.Fatalf("plan hash changed with input order: %s != %s", plan.Hash, second.Hash)
	}
}

func TestPlanDetectsSelectedDependencyCycle(t *testing.T) {
	p, err := New([]domain.Service{
		{Name: "a", Dependencies: []string{"b"}},
		{Name: "b", Dependencies: []string{"a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Plan(domain.ReleaseRequest{
		Environment: domain.EnvironmentStaging,
		Changes:     []domain.Change{{Service: "a", DesiredVersion: "1"}, {Service: "b", DesiredVersion: "1"}},
	})
	if !errors.Is(err, ErrDependencyCycle) {
		t.Fatalf("got %v, want ErrDependencyCycle", err)
	}
}

func TestNewRejectsUnknownDependency(t *testing.T) {
	_, err := New([]domain.Service{{Name: "a", Dependencies: []string{"missing"}}})
	if err == nil {
		t.Fatal("expected unknown dependency error")
	}
}
