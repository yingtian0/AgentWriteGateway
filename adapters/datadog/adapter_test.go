package datadog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"themisy/pkg/adapter"
	"themisy/pkg/credentials"
)

func TestDatadogFourValueClassification(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 5, 0, 0, time.UTC)
	tests := []struct {
		name         string
		body         string
		transportErr error
		want         adapter.VerificationStatus
	}{
		{"pass", metricBody(now, 0.4, 0.5), nil, adapter.VerificationPass},
		{"fail", metricBody(now, 1.2, 1.4), nil, adapter.VerificationFail},
		{"missing", `{"status":"ok","series":[]}`, nil, adapter.VerificationMissing},
		{"inconclusive", "", errors.New("timeout"), adapter.VerificationInconclusive},
		{"partial", metricBody(now, 0.4), nil, adapter.VerificationInconclusive},
		{"malformed", `{"status":"error","series":[]}`, nil, adapter.VerificationInconclusive},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &datadogTransport{body: test.body, err: test.transportErr}
			service := newDatadog(t, transport, now)
			evidence, err := service.Verify(context.Background(), verifyRequest(now), datadogCredential(now))
			if err != nil || evidence.Status != test.want {
				t.Fatalf("status=%s err=%v", evidence.Status, err)
			}
			if evidence.EvidenceHash == "" || evidence.QueryHash == "" {
				t.Fatal("evidence hashes missing")
			}
		})
	}
}

func TestQueryComesOnlyFromTrustedCatalog(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 5, 0, 0, time.UTC)
	transport := &datadogTransport{body: metricBody(now, 0.2, 0.3)}
	service := newDatadog(t, transport, now)
	if _, err := service.Verify(context.Background(), verifyRequest(now), datadogCredential(now)); err != nil {
		t.Fatal(err)
	}
	query := transport.request.URL.Query().Get("query")
	if query != "max:service.error_rate{service:identity,env:staging}" {
		t.Fatalf("query=%q", query)
	}
}

func newDatadog(t *testing.T, transport *datadogTransport, now time.Time) *Adapter {
	t.Helper()
	service, err := New(Config{Site: SiteUS1, Queries: map[QueryKey]Query{{Service: "identity", Environment: "staging"}: {Expression: "max:service.error_rate{service:identity,env:staging}", Comparator: "lte", Threshold: 1, Aggregation: "max", MinimumPoints: 2, MaximumAge: 2 * time.Minute}}}, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	return service
}
func verifyRequest(now time.Time) adapter.VerificationRequest {
	return adapter.VerificationRequest{Target: adapter.Target{Service: "identity", Environment: "staging"}, Deployment: adapter.Deployment{ExternalExecutionID: "run-1", StartedAt: now.Add(-5 * time.Minute)}, Window: adapter.ObservationWindow{From: now.Add(-5 * time.Minute), To: now}}
}
func datadogCredential(now time.Time) credentials.Credential {
	return credentials.New(map[string][]byte{"api_key": []byte("api-secret"), "app_key": []byte("app-secret")}, now.Add(time.Hour))
}
func metricBody(now time.Time, values ...float64) string {
	parts := make([]string, 0, len(values))
	for index, value := range values {
		parts = append(parts, fmtPoint(now.Add(time.Duration(index-len(values)+1)*time.Minute), value))
	}
	return `{"status":"ok","series":[{"pointlist":[` + strings.Join(parts, ",") + `]}]}`
}
func fmtPoint(at time.Time, value float64) string {
	return fmt.Sprintf("[%d,%.4f]", at.UnixMilli(), value)
}

type datadogTransport struct {
	body    string
	err     error
	request *http.Request
}

func (t *datadogTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.request = request.Clone(request.Context())
	if t.err != nil {
		return nil, t.err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(t.body)), Header: make(http.Header), Request: request}, nil
}
