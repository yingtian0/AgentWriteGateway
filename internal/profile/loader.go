package profile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agentwritegateway/internal/domain"
	"go.yaml.in/yaml/v3"
)

func LoadFile(path string) (ReleaseProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ReleaseProfile{}, fmt.Errorf("read release profile %q: %w", path, err)
	}
	profile, err := Decode(data)
	if err != nil {
		return ReleaseProfile{}, fmt.Errorf("decode release profile %q: %w", path, err)
	}
	profile.Source = path
	return profile, nil
}

func Decode(data []byte) (ReleaseProfile, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var profile ReleaseProfile
	if err := decoder.Decode(&profile); err != nil {
		return ReleaseProfile{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return ReleaseProfile{}, errors.New("multiple YAML documents are not allowed")
		}
		return ReleaseProfile{}, err
	}
	if err := Validate(profile); err != nil {
		return ReleaseProfile{}, err
	}
	hash, err := Hash(profile)
	if err != nil {
		return ReleaseProfile{}, err
	}
	profile.ContentHash = hash
	return profile, nil
}

func LoadDir(path string) ([]ReleaseProfile, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read profile directory %q: %w", path, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension == ".yaml" || extension == ".yml" || extension == ".json" {
			paths = append(paths, filepath.Join(path, entry.Name()))
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("profile directory %q contains no YAML or JSON profiles", path)
	}
	profiles := make([]ReleaseProfile, 0, len(paths))
	for _, path := range paths {
		profile, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func Validate(profile ReleaseProfile) error {
	if profile.APIVersion != APIVersion {
		return domain.NewReasonError(domain.ReasonUnsupportedSchemaVersion, "apiVersion", fmt.Sprintf("got %q, want %q", profile.APIVersion, APIVersion), nil)
	}
	if profile.Kind != Kind {
		return domain.NewReasonError(domain.ReasonInvalidProfile, "kind", fmt.Sprintf("got %q, want %q", profile.Kind, Kind), nil)
	}
	if strings.TrimSpace(profile.Metadata.Name) == "" {
		return domain.NewReasonError(domain.ReasonInvalidProfile, "metadata.name", "name is required", nil)
	}
	if len(profile.Spec.RequiredCapabilities) == 0 {
		return domain.NewReasonError(domain.ReasonMissingCapability, "spec.requiredCapabilities", "at least one capability is required", nil)
	}
	seen := make(map[domain.Capability]struct{}, len(profile.Spec.RequiredCapabilities))
	for _, capability := range profile.Spec.RequiredCapabilities {
		if !capability.Valid() {
			return domain.NewReasonError(domain.ReasonInvalidProfile, "spec.requiredCapabilities", fmt.Sprintf("unsupported capability %q", capability), nil)
		}
		if _, duplicate := seen[capability]; duplicate {
			return domain.NewReasonError(domain.ReasonInvalidProfile, "spec.requiredCapabilities", fmt.Sprintf("duplicate capability %q", capability), nil)
		}
		seen[capability] = struct{}{}
	}
	if profile.Spec.Deployment.Strategy == "" {
		return domain.NewReasonError(domain.ReasonInvalidProfile, "spec.deployment.strategy", "strategy is required", nil)
	}
	if duration, err := time.ParseDuration(profile.Spec.Verification.ObservationWindow); err != nil || duration <= 0 {
		return domain.NewReasonError(domain.ReasonInvalidProfile, "spec.verification.observationWindow", "must be a positive Go duration", err)
	}
	switch profile.Spec.Rollback.Mode {
	case "automatic", "approval-required", "manual-only", "unsupported":
	default:
		return domain.NewReasonError(domain.ReasonInvalidProfile, "spec.rollback.mode", "unsupported rollback mode", nil)
	}
	return nil
}

func Canonical(profile ReleaseProfile) ([]byte, error) {
	profile.ContentHash = ""
	profile.Source = ""
	profile.Spec.RequiredCapabilities = append([]domain.Capability(nil), profile.Spec.RequiredCapabilities...)
	sort.Slice(profile.Spec.RequiredCapabilities, func(i, j int) bool {
		return profile.Spec.RequiredCapabilities[i] < profile.Spec.RequiredCapabilities[j]
	})
	data, err := json.Marshal(profile)
	if err != nil {
		return nil, fmt.Errorf("encode canonical release profile: %w", err)
	}
	return data, nil
}

func Hash(profile ReleaseProfile) (string, error) {
	canonical, err := Canonical(profile)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
