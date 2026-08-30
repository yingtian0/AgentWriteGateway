package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCredentialFilesRequirePrivatePermissionsAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	directory := t.TempDir()
	githubPath := filepath.Join(directory, "github.json")
	if err := os.WriteFile(githubPath, []byte(`{"token":"github-secret","expires_at":"`+now.Add(time.Hour).Format(time.RFC3339)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	githubCredential, err := (GitHubTokenFileSource{Path: githubPath}).Token(context.Background())
	if err != nil || string(githubCredential.Value("token")) != "github-secret" {
		t.Fatalf("credential=%#v err=%v", githubCredential, err)
	}
	datadogPath := filepath.Join(directory, "datadog.json")
	if err := os.WriteFile(datadogPath, []byte(`{"api_key":"api-secret","app_key":"app-secret","expires_at":"`+now.Add(time.Hour).Format(time.RFC3339)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	datadogCredential, err := (DatadogCredentialFileSource{Path: datadogPath}).Credential(context.Background())
	if err != nil || string(datadogCredential.Value("api_key")) != "api-secret" || string(datadogCredential.Value("app_key")) != "app-secret" {
		t.Fatalf("credential=%#v err=%v", datadogCredential, err)
	}
	if err := os.Chmod(githubPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (GitHubTokenFileSource{Path: githubPath}).Token(context.Background()); err == nil {
		t.Fatal("world-readable credential file accepted")
	}
}

func TestDatadogBrokerEnforcesPurposeAndTarget(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	target := DatadogTarget{TenantID: "tenant-1", Service: "identity", Environment: "production"}
	broker := &DatadogBroker{Source: staticDatadogSource{credential: New(map[string][]byte{"api_key": []byte("api"), "app_key": []byte("app")}, now.Add(time.Hour))}, Allowed: map[DatadogTarget]struct{}{target: {}}, Now: func() time.Time { return now }}
	credential, err := broker.Acquire(context.Background(), Request{Provider: DatadogProvider, TenantID: target.TenantID, Service: target.Service, Environment: target.Environment, Purpose: PurposeVerify})
	if err != nil || !credential.ValidAt(now) {
		t.Fatalf("credential=%#v err=%v", credential, err)
	}
	if _, err := broker.Acquire(context.Background(), Request{Provider: DatadogProvider, TenantID: target.TenantID, Service: target.Service, Environment: target.Environment, Purpose: PurposeDeploy}); err == nil {
		t.Fatal("Datadog credential issued for deploy")
	}
}

func TestStaticDevelopmentBrokerIsExplicitAndExpires(t *testing.T) {
	broker, err := NewStaticDevelopmentBroker(map[string][]byte{"token": []byte("development")}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Acquire(context.Background(), Request{Provider: "development", TenantID: "tenant", Service: "service", Environment: "staging", Purpose: PurposeDeploy}); err != nil {
		t.Fatal(err)
	}
	broker.now = func() time.Time { return time.Now().Add(time.Hour) }
	if _, err := broker.Acquire(context.Background(), Request{Provider: "development", TenantID: "tenant", Service: "service", Environment: "staging", Purpose: PurposeDeploy}); err == nil {
		t.Fatal("expired development credential issued")
	}
}

type staticDatadogSource struct{ credential Credential }

func (s staticDatadogSource) Credential(context.Context) (Credential, error) {
	return s.credential.Clone(), nil
}
