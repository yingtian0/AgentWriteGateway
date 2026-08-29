package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const DatadogProvider = "datadog"

type DatadogTarget struct {
	TenantID    string
	Service     string
	Environment string
}

type DatadogCredentialFileSource struct {
	Path string
}

func (s DatadogCredentialFileSource) Credential(_ context.Context) (Credential, error) {
	if strings.TrimSpace(s.Path) == "" {
		return Credential{}, errors.New("Datadog credential file is required")
	}
	file, err := os.Open(s.Path)
	if err != nil {
		return Credential{}, fmt.Errorf("open Datadog credential file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Credential{}, fmt.Errorf("stat Datadog credential file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Credential{}, errors.New("Datadog credential file must not be group or world accessible")
	}
	var document struct {
		APIKey    string    `json:"api_key"`
		AppKey    string    `json:"app_key"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Credential{}, fmt.Errorf("decode Datadog credential file: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Credential{}, errors.New("Datadog credential file contains trailing JSON")
	}
	if strings.TrimSpace(document.APIKey) == "" || strings.TrimSpace(document.AppKey) == "" || document.ExpiresAt.IsZero() {
		return Credential{}, errors.New("Datadog API key, application key, and expiry are required")
	}
	return New(map[string][]byte{"api_key": []byte(document.APIKey), "app_key": []byte(document.AppKey)}, document.ExpiresAt), nil
}

type DatadogCredentialSource interface {
	Credential(context.Context) (Credential, error)
}

type DatadogBroker struct {
	Source  DatadogCredentialSource
	Allowed map[DatadogTarget]struct{}
	Now     func() time.Time
}

func (b *DatadogBroker) Acquire(ctx context.Context, request Request) (Credential, error) {
	if err := request.Validate(); err != nil {
		return Credential{}, err
	}
	if request.Provider != DatadogProvider || request.Purpose != PurposeVerify {
		return Credential{}, errors.New("Datadog broker only serves verification")
	}
	if _, ok := b.Allowed[DatadogTarget{TenantID: request.TenantID, Service: request.Service, Environment: request.Environment}]; !ok {
		return Credential{}, errors.New("Datadog credential target is not allow-listed")
	}
	if b.Source == nil {
		return Credential{}, errors.New("Datadog credential source unavailable")
	}
	credential, err := b.Source.Credential(ctx)
	if err != nil {
		return Credential{}, err
	}
	now := time.Now().UTC()
	if b.Now != nil {
		now = b.Now().UTC()
	}
	if len(credential.Value("api_key")) == 0 || len(credential.Value("app_key")) == 0 || !credential.ValidAt(now.Add(30*time.Second)) {
		return Credential{}, errors.New("Datadog credential is missing or too close to expiry")
	}
	return credential.Clone(), nil
}
