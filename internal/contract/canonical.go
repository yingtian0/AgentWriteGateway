package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"themisy/internal/domain"
)

func Canonical(contract ServiceContract) ([]byte, error) {
	contract.ContentHash = ""
	contract.Source = ""
	contract.Dependencies = append([]domain.Dependency(nil), contract.Dependencies...)
	contract.Verification.Checks = append([]VerificationCheck(nil), contract.Verification.Checks...)
	environments := make(map[string]EnvironmentContract, len(contract.Environments))
	for name, environment := range contract.Environments {
		environment.Capabilities = append([]domain.Capability(nil), environment.Capabilities...)
		sort.Slice(environment.Capabilities, func(i, j int) bool {
			return environment.Capabilities[i] < environment.Capabilities[j]
		})
		environments[name] = environment
	}
	contract.Environments = environments
	sort.Slice(contract.Dependencies, func(i, j int) bool {
		left, right := contract.Dependencies[i], contract.Dependencies[j]
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.Service != right.Service {
			return left.Service < right.Service
		}
		if left.Resource != right.Resource {
			return left.Resource < right.Resource
		}
		if left.Condition != right.Condition {
			return left.Condition < right.Condition
		}
		return left.Constraint < right.Constraint
	})
	sort.Slice(contract.Verification.Checks, func(i, j int) bool {
		left, right := contract.Verification.Checks[i], contract.Verification.Checks[j]
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.Check != right.Check {
			return left.Check < right.Check
		}
		return left.Threshold < right.Threshold
	})
	data, err := json.Marshal(contract)
	if err != nil {
		return nil, fmt.Errorf("encode canonical service contract: %w", err)
	}
	return data, nil
}

func Hash(contract ServiceContract) (string, error) {
	canonical, err := Canonical(contract)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
