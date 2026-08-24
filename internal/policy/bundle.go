package policy

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"agentwritegateway/internal/grant"
	"agentwritegateway/pkg/protocol"
)

type Layer string

const (
	LayerPlatform    Layer = "platform_mandatory"
	LayerEnvironment Layer = "environment"
	LayerTeam        Layer = "team"
	LayerService     Layer = "service"
)

type Module struct {
	Name   string `json:"name"`
	Layer  Layer  `json:"layer"`
	Source string `json:"source"`
}

type Bundle struct {
	Version       string             `json:"version"`
	Issuer        string             `json:"issuer"`
	Compatibility []string           `json:"compatibility"`
	IssuedAt      time.Time          `json:"issued_at"`
	ExpiresAt     time.Time          `json:"expires_at"`
	Modules       []Module           `json:"modules"`
	Hash          string             `json:"hash"`
	Signature     protocol.Signature `json:"signature"`
}

type BundleKeyResolver interface {
	ResolvePolicyKey(context.Context, string, string) (ed25519.PublicKey, error)
}
type StaticBundleKeys map[string]ed25519.PublicKey

func (s StaticBundleKeys) ResolvePolicyKey(_ context.Context, issuer, keyID string) (ed25519.PublicKey, error) {
	key, ok := s[issuer+"\x00"+keyID]
	if !ok {
		return nil, fmt.Errorf("unknown policy bundle key")
	}
	return append(ed25519.PublicKey(nil), key...), nil
}

var ErrInvalidBundle = errors.New("INVALID_POLICY_BUNDLE")

func CalculateBundleHash(bundle Bundle) (string, error) {
	copy := normalizedBundle(bundle)
	copy.Hash, copy.Signature = "", protocol.Signature{}
	data, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func CanonicalBundlePayload(bundle Bundle) ([]byte, error) {
	copy := normalizedBundle(bundle)
	copy.Signature = protocol.Signature{}
	if copy.Version == "" || copy.Issuer == "" || copy.Hash == "" || copy.IssuedAt.IsZero() || !copy.ExpiresAt.After(copy.IssuedAt) || len(copy.Modules) == 0 {
		return nil, ErrInvalidBundle
	}
	return json.Marshal(copy)
}

func SignBundle(ctx context.Context, signer grant.Signer, bundle Bundle) (Bundle, error) {
	hash, err := CalculateBundleHash(bundle)
	if err != nil {
		return Bundle{}, err
	}
	bundle.Hash = hash
	payload, err := CanonicalBundlePayload(bundle)
	if err != nil {
		return Bundle{}, err
	}
	bundle.Signature, err = signer.Sign(ctx, payload)
	return bundle, err
}

func VerifyBundle(ctx context.Context, bundle Bundle, issuer string, keys BundleKeyResolver, now time.Time) error {
	if bundle.Issuer != issuer || bundle.Signature.Algorithm != grant.AlgorithmEd25519 || keys == nil || !containsString(bundle.Compatibility, InputVersionV1Alpha1) || bundle.IssuedAt.After(now.UTC()) || !bundle.ExpiresAt.After(now.UTC()) || !validLayers(bundle.Modules) {
		return ErrInvalidBundle
	}
	expected, err := CalculateBundleHash(bundle)
	if err != nil || expected != bundle.Hash {
		return ErrInvalidBundle
	}
	payload, err := CanonicalBundlePayload(bundle)
	if err != nil {
		return ErrInvalidBundle
	}
	key, err := keys.ResolvePolicyKey(ctx, issuer, bundle.Signature.KeyID)
	if err != nil {
		return ErrInvalidBundle
	}
	signature, err := base64.RawURLEncoding.DecodeString(bundle.Signature.Value)
	if err != nil || !ed25519.Verify(key, payload, signature) {
		return ErrInvalidBundle
	}
	return nil
}

func validLayers(modules []Module) bool {
	hasPlatform := false
	for _, module := range modules {
		switch module.Layer {
		case LayerPlatform:
			hasPlatform = true
		case LayerEnvironment, LayerTeam, LayerService:
		default:
			return false
		}
		if module.Name == "" || module.Source == "" {
			return false
		}
	}
	return hasPlatform
}

func normalizedBundle(bundle Bundle) Bundle {
	bundle.Compatibility = append([]string(nil), bundle.Compatibility...)
	bundle.Modules = append([]Module(nil), bundle.Modules...)
	sort.Strings(bundle.Compatibility)
	sort.Slice(bundle.Modules, func(i, j int) bool {
		if bundle.Modules[i].Layer == bundle.Modules[j].Layer {
			return bundle.Modules[i].Name < bundle.Modules[j].Name
		}
		return bundle.Modules[i].Layer < bundle.Modules[j].Layer
	})
	return bundle
}
