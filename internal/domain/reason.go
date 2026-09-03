package domain

import (
	"errors"
)

type ReasonCode string

const (
	ReasonUnsupportedSchemaVersion ReasonCode = "UNSUPPORTED_SCHEMA_VERSION"
	ReasonInvalidContract          ReasonCode = "INVALID_CONTRACT"
	ReasonInvalidProfile           ReasonCode = "INVALID_PROFILE"
	ReasonMissingOwner             ReasonCode = "MISSING_OWNER"
	ReasonUnknownProfile           ReasonCode = "UNKNOWN_PROFILE"
	ReasonMissingCapability        ReasonCode = "MISSING_CAPABILITY"
	ReasonUnknownDependency        ReasonCode = "UNKNOWN_DEPENDENCY"
	ReasonDependencyCycle          ReasonCode = "DEPENDENCY_CYCLE"
	ReasonPlanExpired              ReasonCode = "PLAN_EXPIRED"
	ReasonPlanHashMismatch         ReasonCode = "PLAN_HASH_MISMATCH"
	ReasonContractChanged          ReasonCode = "CONTRACT_CHANGED"
	ReasonProfileChanged           ReasonCode = "PROFILE_CHANGED"
	ReasonContextChanged           ReasonCode = "CONTEXT_CHANGED"
	ReasonPolicyChanged            ReasonCode = "POLICY_CHANGED"
	ReasonEvidenceChanged          ReasonCode = "EVIDENCE_CHANGED"
	ReasonTenantBoundary           ReasonCode = "TENANT_BOUNDARY"
	ReasonRunnerFrozen             ReasonCode = "RUNNER_FROZEN"
	ReasonCircuitOpen              ReasonCode = "CIRCUIT_OPEN"
	ReasonBackpressure             ReasonCode = "BACKPRESSURE"
)

type ReasonError struct {
	Code   ReasonCode
	Field  string
	Detail string
	Err    error
}

func (e *ReasonError) Error() string {
	message := string(e.Code)
	if e.Field != "" {
		message += " at " + e.Field
	}
	if e.Detail != "" {
		message += ": " + e.Detail
	}
	if e.Err != nil && e.Detail == "" {
		message += ": " + e.Err.Error()
	}
	return message
}

func (e *ReasonError) Unwrap() error { return e.Err }

func NewReasonError(code ReasonCode, field, detail string, err error) error {
	return &ReasonError{Code: code, Field: field, Detail: detail, Err: err}
}

func ReasonOf(err error) (ReasonCode, bool) {
	var reason *ReasonError
	if !errors.As(err, &reason) {
		return "", false
	}
	return reason.Code, true
}
