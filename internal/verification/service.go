package verification

import (
	"context"
	"errors"

	"themisy/pkg/adapter"
	"themisy/pkg/credentials"
)

type Service struct {
	TenantID string
	Adapter  adapter.VerificationAdapter
	Broker   credentials.Broker
}

func (s *Service) Verify(ctx context.Context, request adapter.VerificationRequest) (adapter.Evidence, error) {
	if s.TenantID == "" || s.Adapter == nil || s.Broker == nil {
		return adapter.Evidence{}, errors.New("verification service is not configured")
	}
	credential, err := s.Broker.Acquire(ctx, credentials.Request{Provider: adapter.ProviderDatadog, TenantID: s.TenantID, Service: request.Target.Service, Environment: request.Target.Environment, Purpose: credentials.PurposeVerify})
	if err != nil {
		return adapter.Evidence{}, err
	}
	evidence, err := s.Adapter.Verify(ctx, request, credential)
	if err != nil {
		return adapter.Evidence{}, err
	}
	if err := ValidateEvidence(evidence); err != nil {
		return adapter.Evidence{}, err
	}
	return evidence, nil
}
