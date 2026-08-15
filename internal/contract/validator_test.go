package contract

import (
	"errors"
	"testing"

	"agentwritegateway/internal/domain"
)

func TestContractValidInvalidMatrix(t *testing.T) {
	tests := []struct {
		name string
		path string
		code domain.ReasonCode
	}{
		{name: "valid", path: "../../test/fixtures/contracts/valid/service.yaml"},
		{name: "missing owner", path: "../../test/fixtures/contracts/invalid/missing-owner.yaml", code: domain.ReasonMissingOwner},
		{name: "unsupported schema", path: "../../test/fixtures/contracts/invalid/unsupported-schema-version.yaml", code: domain.ReasonUnsupportedSchemaVersion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadFile(test.path)
			if test.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %s", test.code)
			}
			var reason *domain.ReasonError
			if !errors.As(err, &reason) || reason.Code != test.code {
				t.Fatalf("got %v, want reason %s", err, test.code)
			}
		})
	}
}

func TestValidateAcceptsEveryDependencyType(t *testing.T) {
	contract, err := LoadFile("../../test/fixtures/contracts/valid/service.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, dependencyType := range []domain.DependencyType{
		domain.DependencyRolloutOrder,
		domain.DependencySchemaCompatibility,
		domain.DependencyRuntime,
		domain.DependencySharedFailureDomain,
		domain.DependencyDataMigration,
		domain.DependencyTraffic,
	} {
		candidate := contract
		candidate.Dependencies = []domain.Dependency{{Service: "another-service", Type: dependencyType}}
		if dependencyType == domain.DependencySharedFailureDomain || dependencyType == domain.DependencySchemaCompatibility {
			candidate.Dependencies[0].Service = ""
			candidate.Dependencies[0].Resource = "shared-resource"
		}
		if err := Validate(candidate); err != nil {
			t.Fatalf("dependency type %s rejected: %v", dependencyType, err)
		}
	}
}
