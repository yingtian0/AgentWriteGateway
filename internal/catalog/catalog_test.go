package catalog

import (
	"testing"

	"themisy/internal/domain"
	"themisy/internal/planner"
)

func TestLegacyCatalogConvertsToContracts(t *testing.T) {
	contracts, profiles, err := LoadContracts("../../config/services.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 20 || len(profiles) != 1 {
		t.Fatalf("contracts=%d profiles=%d, want 20/1", len(contracts), len(profiles))
	}
	p, err := planner.NewFromContracts(contracts, profiles, planner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := p.Plan(domain.ReleaseRequest{
		ReleaseVersion: "legacy",
		Environment:    domain.EnvironmentStaging,
		Changes: []domain.Change{
			{Service: "identity-api", DesiredVersion: "identity"},
			{Service: "user-api", DesiredVersion: "user"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Phases) != 2 {
		t.Fatalf("legacy phases=%d, want 2", len(plan.Phases))
	}
}
