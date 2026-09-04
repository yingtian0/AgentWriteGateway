package grant

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"themisy/pkg/protocol"
)

const AlgorithmEd25519 = "Ed25519"

// Signer is the boundary implemented by production KMS/HSM integrations.
// Implementations receive only canonical bytes and never expose private key material.
type Signer interface {
	Sign(context.Context, []byte) (protocol.Signature, error)
}

type KMSClient interface {
	Sign(context.Context, string, []byte) ([]byte, error)
}

type KMSSigner struct {
	Client    KMSClient
	KeyID     string
	Algorithm string
}

func (s KMSSigner) Sign(ctx context.Context, payload []byte) (protocol.Signature, error) {
	if s.Client == nil || s.KeyID == "" || s.Algorithm == "" {
		return protocol.Signature{}, fmt.Errorf("KMS signer is not configured")
	}
	value, err := s.Client.Sign(ctx, s.KeyID, payload)
	if err != nil {
		return protocol.Signature{}, fmt.Errorf("KMS sign: %w", err)
	}
	return protocol.Signature{Algorithm: s.Algorithm, KeyID: s.KeyID, Value: base64.RawURLEncoding.EncodeToString(value)}, nil
}

// DevelopmentSigner is intentionally explicit and must not be selected as a
// production default. Its private key exists only in the local process.
type DevelopmentSigner struct {
	keyID string
	key   ed25519.PrivateKey
}

func NewDevelopmentSigner(keyID string) (*DevelopmentSigner, ed25519.PublicKey, error) {
	if keyID == "" {
		return nil, nil, fmt.Errorf("development key id is required")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return &DevelopmentSigner{keyID: keyID, key: private}, public, nil
}

func (s *DevelopmentSigner) Sign(_ context.Context, payload []byte) (protocol.Signature, error) {
	if len(s.key) != ed25519.PrivateKeySize {
		return protocol.Signature{}, fmt.Errorf("development signer is not initialized")
	}
	value := ed25519.Sign(s.key, payload)
	return protocol.Signature{Algorithm: AlgorithmEd25519, KeyID: s.keyID, Value: base64.RawURLEncoding.EncodeToString(value)}, nil
}

func SignActionGrant(ctx context.Context, signer Signer, actionGrant protocol.ActionGrant) (protocol.ActionGrant, error) {
	payload, err := protocol.CanonicalGrantPayload(actionGrant)
	if err != nil {
		return protocol.ActionGrant{}, err
	}
	signature, err := signer.Sign(ctx, payload)
	if err != nil {
		return protocol.ActionGrant{}, err
	}
	actionGrant.Signature = signature
	return actionGrant, nil
}
