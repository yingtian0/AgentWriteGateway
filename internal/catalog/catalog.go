package catalog

import (
	"encoding/json"
	"fmt"
	"os"

	"themisy/internal/contract"
	"themisy/internal/domain"
	"themisy/internal/profile"
)

func Load(path string) ([]domain.Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read service catalog: %w", err)
	}
	var services []domain.Service
	if err := json.Unmarshal(data, &services); err != nil {
		return nil, fmt.Errorf("decode service catalog: %w", err)
	}
	return services, nil
}

func LoadContracts(path string) ([]contract.ServiceContract, []profile.ReleaseProfile, error) {
	services, err := Load(path)
	if err != nil {
		return nil, nil, err
	}
	releaseProfile := CompatibilityProfile()
	profileHash, err := profile.Hash(releaseProfile)
	if err != nil {
		return nil, nil, err
	}
	releaseProfile.ContentHash = profileHash
	contracts := make([]contract.ServiceContract, 0, len(services))
	for _, service := range services {
		dependencies := make([]domain.Dependency, 0, len(service.Dependencies))
		for _, dependency := range service.Dependencies {
			dependencies = append(dependencies, domain.Dependency{
				Service:   dependency,
				Type:      domain.DependencyRolloutOrder,
				Condition: "verified",
			})
		}
		serviceContract := contract.ServiceContract{
			APIVersion: contract.APIVersion,
			Kind:       contract.Kind,
			Metadata: contract.Metadata{
				Name:       service.Name,
				Owner:      service.OwnerTeam,
				Repository: service.Repository,
				RiskTier:   "medium",
			},
			Environments: map[string]contract.EnvironmentContract{
				string(domain.EnvironmentStaging): {
					ReleaseProfile: releaseProfile.Metadata.Name,
					RunnerGroup:    "legacy-staging",
					Capabilities:   append([]domain.Capability(nil), releaseProfile.Spec.RequiredCapabilities...),
				},
				string(domain.EnvironmentProduction): {
					ReleaseProfile: releaseProfile.Metadata.Name,
					RunnerGroup:    "legacy-production",
					Capabilities:   append([]domain.Capability(nil), releaseProfile.Spec.RequiredCapabilities...),
				},
			},
			Dependencies: dependencies,
			Verification: contract.Verification{
				Profile:           "legacy-health",
				ObservationWindow: "1m",
				Checks: []contract.VerificationCheck{{
					Type:      "health",
					Threshold: "healthy",
				}},
			},
			Rollback: contract.Rollback{
				Capability: "automatic",
				Strategy:   "mock-previous-version",
				Deadline:   "30m",
			},
		}
		hash, err := contract.Hash(serviceContract)
		if err != nil {
			return nil, nil, err
		}
		serviceContract.ContentHash = hash
		serviceContract.Source = path
		contracts = append(contracts, serviceContract)
	}
	return contracts, []profile.ReleaseProfile{releaseProfile}, nil
}

func CompatibilityProfile() profile.ReleaseProfile {
	return profile.ReleaseProfile{
		APIVersion: profile.APIVersion,
		Kind:       profile.Kind,
		Metadata:   profile.Metadata{Name: "legacy-mock"},
		Spec: profile.Spec{
			RequiredCapabilities: []domain.Capability{
				domain.CapabilityDeploy,
				domain.CapabilityGetStatus,
				domain.CapabilityVerifyRuntime,
				domain.CapabilityRollback,
			},
			Deployment:   profile.Deployment{Strategy: "mock"},
			Verification: profile.Verification{Required: true, ObservationWindow: "1m"},
			Rollback:     profile.Rollback{Mode: "automatic"},
		},
	}
}
