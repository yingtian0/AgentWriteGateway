package githubactions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"agentwritegateway/pkg/adapter"
	"agentwritegateway/pkg/credentials"
)

func TestDeployUsesConfiguredWorkflowAndAllowListedInputs(t *testing.T) {
	now := testNow()
	transport := &recordingTransport{responses: []*http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"workflow_run_id":123,"run_url":"https://api.github.com/runs/123","html_url":"https://github.com/runs/123"}`)), Header: make(http.Header)}}}
	client := &http.Client{Transport: transport}
	service := newTestAdapter(t, client, now)
	request := testDeployRequest()
	deployment, err := service.Deploy(context.Background(), request, testCredential(now))
	if err != nil {
		t.Fatal(err)
	}
	if deployment.ExternalExecutionID != "123" || transport.Calls() != 1 {
		t.Fatalf("deployment=%#v calls=%d", deployment, transport.Calls())
	}
	recorded := transport.Requests()[0]
	if recorded.URL.String() != apiBaseURL+"/repos/acme/releases/actions/workflows/deploy.yml/dispatches" {
		t.Fatalf("url=%s", recorded.URL)
	}
	var body struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs"`
	}
	if err := json.Unmarshal(recorded.body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Inputs) != 4 || body.Inputs["awg_artifact_digest"] != request.ArtifactDigest || body.Inputs["awg_idempotency_key"] != adapter.CorrelationID(request.IdempotencyKey) {
		t.Fatalf("body=%#v", body)
	}
	if strings.Contains(string(recorded.body), "command") || strings.Contains(string(recorded.body), "url") {
		t.Fatal("untyped field escaped into dispatch")
	}
}

func TestTimeoutIsUnknownAndNeverBlindlyRetried(t *testing.T) {
	now := testNow()
	transport := &recordingTransport{err: errors.New("timeout")}
	service := newTestAdapter(t, &http.Client{Transport: transport}, now)
	_, err := service.Deploy(context.Background(), testDeployRequest(), testCredential(now))
	if !adapter.IsClass(err, adapter.ErrorUnknown) || transport.Calls() != 1 {
		t.Fatalf("err=%v calls=%d", err, transport.Calls())
	}
}

func TestReconcileFindsCorrelationRunName(t *testing.T) {
	now := testNow()
	correlation := adapter.CorrelationID(testDeployRequest().IdempotencyKey)
	payload := `{"workflow_runs":[{"id":456,"display_title":"awg:` + correlation + `","status":"completed","conclusion":"success","created_at":"2026-08-29T00:00:00Z","updated_at":"2026-08-29T00:01:00Z"}]}`
	transport := &recordingTransport{responses: []*http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header)}}}
	service := newTestAdapter(t, &http.Client{Transport: transport}, now)
	result, err := service.Reconcile(context.Background(), adapter.ReconcileRequest{IdempotencyKey: testDeployRequest().IdempotencyKey, DispatchedAt: now.Add(-time.Minute)}, testCredential(now))
	if err != nil || result.Status != adapter.ReconcileSucceeded || result.Deployment.ExternalExecutionID != "456" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestConfigRejectsArbitraryWorkflowPathAndRef(t *testing.T) {
	config := testConfig()
	key := TargetKey{Service: "identity", Environment: "staging"}
	target := config.Targets[key]
	target.DeployWorkflow = "../../danger.yml"
	config.Targets[key] = target
	if _, err := New(config, nil); err == nil {
		t.Fatal("arbitrary workflow path accepted")
	}
}

func TestGitHubActionsAdapterPassesPublicConformanceSuite(t *testing.T) {
	now := testNow()
	request := testDeployRequest()
	correlation := adapter.CorrelationID(request.IdempotencyKey)
	transport := &recordingTransport{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"workflow_run_id":123}`)), Header: make(http.Header)},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"workflow_runs":[{"id":123,"display_title":"awg:` + correlation + `","status":"completed","conclusion":"success","created_at":"2026-08-29T00:00:00Z","updated_at":"2026-08-29T00:01:00Z"}]}`)), Header: make(http.Header)},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"workflow_run_id":789}`)), Header: make(http.Header)},
	}}
	service := newTestAdapter(t, &http.Client{Transport: transport}, now)
	report, err := adapter.RunConformance(context.Background(), adapter.ConformanceInput{Adapter: service, Credential: testCredential(now), Deploy: request, Now: func() time.Time { return now }})
	if err != nil || report.Deployment.ExternalExecutionID != "123" || report.Rollback.ExternalExecutionID != "789" || transport.Calls() != 3 {
		t.Fatalf("report=%#v calls=%d err=%v", report, transport.Calls(), err)
	}
}

func TestDeployRejectsUntypedArtifactBeforeHTTP(t *testing.T) {
	now := testNow()
	transport := &recordingTransport{}
	service := newTestAdapter(t, &http.Client{Transport: transport}, now)
	request := testDeployRequest()
	request.ArtifactDigest = "latest"
	if _, err := service.Deploy(context.Background(), request, testCredential(now)); !adapter.IsClass(err, adapter.ErrorTerminal) || transport.Calls() != 0 {
		t.Fatalf("err=%v calls=%d", err, transport.Calls())
	}
}

func testConfig() Config {
	return Config{Targets: map[TargetKey]Target{{Service: "identity", Environment: "staging"}: {Owner: "acme", Repository: "releases", DeployWorkflow: "deploy.yml", RollbackWorkflow: "rollback.yml", Ref: "main"}}}
}
func newTestAdapter(t *testing.T, client HTTPDoer, now time.Time) *Adapter {
	t.Helper()
	service, err := New(testConfig(), client)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	return service
}
func testDeployRequest() adapter.DeployRequest {
	return adapter.DeployRequest{Target: adapter.Target{Service: "identity", Environment: "staging"}, ArtifactDigest: "sha256:" + strings.Repeat("a", 64), IdempotencyKey: "run/identity/v1"}
}
func testCredential(now time.Time) credentials.Credential {
	return credentials.New(map[string][]byte{"token": []byte("secret-test-token")}, now.Add(time.Hour))
}
func testNow() time.Time { return time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }

type recordedRequest struct {
	*http.Request
	body []byte
}
type recordingTransport struct {
	mu        sync.Mutex
	requests  []recordedRequest
	responses []*http.Response
	err       error
}

func (t *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var body []byte
	if request.Body != nil {
		body, _ = io.ReadAll(request.Body)
	}
	copyRequest := request.Clone(request.Context())
	t.requests = append(t.requests, recordedRequest{Request: copyRequest, body: body})
	if t.err != nil {
		return nil, t.err
	}
	if len(t.responses) == 0 {
		return nil, errors.New("no response")
	}
	response := t.responses[0]
	t.responses = t.responses[1:]
	response.Request = request
	return response, nil
}
func (t *recordingTransport) Calls() int { t.mu.Lock(); defer t.mu.Unlock(); return len(t.requests) }
func (t *recordingTransport) Requests() []recordedRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]recordedRequest(nil), t.requests...)
}
