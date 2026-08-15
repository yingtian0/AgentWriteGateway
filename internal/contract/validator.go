package contract

import (
	"fmt"
	"strings"
	"time"

	"agentwritegateway/internal/domain"
)

func Validate(contract ServiceContract) error {
	if contract.APIVersion != APIVersion {
		return domain.NewReasonError(domain.ReasonUnsupportedSchemaVersion, "apiVersion", fmt.Sprintf("got %q, want %q", contract.APIVersion, APIVersion), nil)
	}
	if contract.Kind != Kind {
		return domain.NewReasonError(domain.ReasonInvalidContract, "kind", fmt.Sprintf("got %q, want %q", contract.Kind, Kind), nil)
	}
	if strings.TrimSpace(contract.Metadata.Name) == "" {
		return domain.NewReasonError(domain.ReasonInvalidContract, "metadata.name", "name is required", nil)
	}
	if strings.TrimSpace(contract.Metadata.Owner) == "" {
		return domain.NewReasonError(domain.ReasonMissingOwner, "metadata.owner", "owner is required", nil)
	}
	if strings.TrimSpace(contract.Metadata.Repository) == "" {
		return domain.NewReasonError(domain.ReasonInvalidContract, "metadata.repository", "repository is required", nil)
	}
	switch contract.Metadata.RiskTier {
	case "low", "medium", "high", "critical":
	default:
		return domain.NewReasonError(domain.ReasonInvalidContract, "metadata.riskTier", "must be low, medium, high, or critical", nil)
	}
	if len(contract.Environments) == 0 {
		return domain.NewReasonError(domain.ReasonInvalidContract, "environments", "at least one environment is required", nil)
	}
	for name, environment := range contract.Environments {
		field := "environments." + name
		if name != string(domain.EnvironmentStaging) && name != string(domain.EnvironmentProduction) {
			return domain.NewReasonError(domain.ReasonInvalidContract, field, "unsupported environment", nil)
		}
		if environment.ReleaseProfile == "" {
			return domain.NewReasonError(domain.ReasonInvalidContract, field+".releaseProfile", "release profile is required", nil)
		}
		if environment.RunnerGroup == "" {
			return domain.NewReasonError(domain.ReasonInvalidContract, field+".runnerGroup", "runner group is required", nil)
		}
		if len(environment.Capabilities) == 0 {
			return domain.NewReasonError(domain.ReasonMissingCapability, field+".capabilities", "at least one capability is required", nil)
		}
		seen := make(map[domain.Capability]struct{}, len(environment.Capabilities))
		for _, capability := range environment.Capabilities {
			if !capability.Valid() {
				return domain.NewReasonError(domain.ReasonInvalidContract, field+".capabilities", fmt.Sprintf("unsupported capability %q", capability), nil)
			}
			if _, duplicate := seen[capability]; duplicate {
				return domain.NewReasonError(domain.ReasonInvalidContract, field+".capabilities", fmt.Sprintf("duplicate capability %q", capability), nil)
			}
			seen[capability] = struct{}{}
		}
	}
	for index, dependency := range contract.Dependencies {
		field := fmt.Sprintf("dependencies[%d]", index)
		if !dependency.Type.Valid() {
			return domain.NewReasonError(domain.ReasonInvalidContract, field+".type", fmt.Sprintf("unsupported dependency type %q", dependency.Type), nil)
		}
		if (dependency.Service == "") == (dependency.Resource == "") {
			return domain.NewReasonError(domain.ReasonInvalidContract, field, "exactly one of service or resource is required", nil)
		}
	}
	if contract.Verification.Profile == "" || contract.Verification.ObservationWindow == "" || len(contract.Verification.Checks) == 0 {
		return domain.NewReasonError(domain.ReasonInvalidContract, "verification", "profile, observationWindow, and checks are required", nil)
	}
	if duration, err := time.ParseDuration(contract.Verification.ObservationWindow); err != nil || duration <= 0 {
		return domain.NewReasonError(domain.ReasonInvalidContract, "verification.observationWindow", "must be a positive Go duration", err)
	}
	for index, check := range contract.Verification.Checks {
		if check.Type == "" || (check.Threshold == "" && check.Check == "") {
			return domain.NewReasonError(domain.ReasonInvalidContract, fmt.Sprintf("verification.checks[%d]", index), "type and threshold or check are required", nil)
		}
	}
	switch contract.Rollback.Capability {
	case "automatic", "approval-required", "manual-only", "unsupported":
	default:
		return domain.NewReasonError(domain.ReasonInvalidContract, "rollback.capability", "unsupported rollback capability", nil)
	}
	if contract.Rollback.Strategy == "" {
		return domain.NewReasonError(domain.ReasonInvalidContract, "rollback.strategy", "strategy is required", nil)
	}
	if duration, err := time.ParseDuration(contract.Rollback.Deadline); err != nil || duration <= 0 {
		return domain.NewReasonError(domain.ReasonInvalidContract, "rollback.deadline", "must be a positive Go duration", err)
	}
	return nil
}
