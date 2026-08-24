package runner

import (
	"context"
	"fmt"
	"time"

	"agentwritegateway/internal/identity"
	"agentwritegateway/pkg/protocol"
)

type ConnectionState bool

func (s ConnectionState) Connected() bool { return bool(s) }

type StaticContexts map[string]PinnedContext

func (s StaticContexts) ResolveContext(_ context.Context, runID, stepID string) (PinnedContext, error) {
	value, ok := s[runID+"\x00"+stepID]
	if !ok {
		return PinnedContext{}, fmt.Errorf("pinned context not found")
	}
	return value, nil
}

type ProofReferences map[string]struct{}

func (p ProofReferences) VerifyApprovals(_ context.Context, grant protocol.ActionGrant, _ identity.Subject) error {
	for _, reference := range grant.ApprovalProofs {
		if _, ok := p[reference]; !ok {
			return fmt.Errorf("untrusted approval proof")
		}
	}
	return nil
}

type Credential struct {
	Value     []byte
	ExpiresAt time.Time
}

type CredentialRequest struct {
	TenantID    string
	Service     string
	Environment string
	Capability  protocol.Capability
}

type CredentialBroker interface {
	Acquire(context.Context, CredentialRequest) (Credential, error)
}

type AdapterRequest struct {
	GrantID        string
	RunID          string
	StepID         string
	Target         protocol.Target
	Action         protocol.Action
	IdempotencyKey string
}

type AdapterResult struct {
	ExternalExecutionID string
	CompletedAt         time.Time
}

// TypedAdapter intentionally exposes no command, URL, shell, or generic cloud operation.
type TypedAdapter interface {
	Execute(context.Context, AdapterRequest, Credential) (AdapterResult, error)
}

type Reconciler interface {
	Reconcile(context.Context, string) (AdapterResult, bool, error)
}
