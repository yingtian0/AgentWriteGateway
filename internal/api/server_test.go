package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentwritegateway/internal/domain"
	"agentwritegateway/internal/engine"
	"agentwritegateway/internal/executor"
	"agentwritegateway/internal/planner"
	"agentwritegateway/internal/policy"
	"agentwritegateway/internal/store"
)

func TestPlanAndStartRoutes(t *testing.T) {
	p, err := planner.New([]domain.Service{{Name: "identity", ReleasePhase: 0}})
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(p, policy.New(), executor.NewMock(nil), store.NewMemory())
	handler := New(e, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	body := []byte(`{
      "request_id":"http-1",
      "release_version":"release-1",
      "environment":"staging",
      "requested_by":"user-1",
      "delegated_agent":{"id":"agent-1","scopes":["release:deploy"]},
      "changes":[{"service":"identity","desired_version":"sha-1","ci_success":true,"dependencies_healthy":true}]
    }`)

	planRequest := httptest.NewRequest(http.MethodPost, "/v1/release-runs:plan", bytes.NewReader(body))
	planRequest.Header.Set("Content-Type", "application/json")
	planResponse := httptest.NewRecorder()
	handler.ServeHTTP(planResponse, planRequest)
	if planResponse.Code != http.StatusOK {
		t.Fatalf("plan status=%d body=%s", planResponse.Code, planResponse.Body.String())
	}
	var plan domain.ReleasePlan
	if err := json.NewDecoder(planResponse.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	if plan.Hash == "" || plan.PlanHash != plan.Hash || plan.ID == "" || plan.ExpiresAt.IsZero() || len(plan.Phases) != 1 {
		t.Fatalf("unexpected plan: %#v", plan)
	}

	startRequest := httptest.NewRequest(http.MethodPost, "/v1/release-runs", bytes.NewReader(body))
	startRequest.Header.Set("Content-Type", "application/json")
	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusCreated {
		t.Fatalf("start status=%d body=%s", startResponse.Code, startResponse.Body.String())
	}
	var run domain.ReleaseRun
	if err := json.NewDecoder(startResponse.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.RunSucceeded {
		t.Fatalf("run status=%s, want SUCCEEDED", run.Status)
	}
}

func TestVersionedIntentUsesExistingPlanAndStartRoutes(t *testing.T) {
	p, err := planner.New([]domain.Service{{Name: "identity", ReleasePhase: 0}})
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(p, policy.New(), executor.NewMock(nil), store.NewMemory())
	handler := New(e, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	body := []byte(`{
      "api_version":"execution.agentwritegateway.io/v1alpha1",
      "kind":"ReleaseIntent",
      "request_id":"versioned-http-1",
      "release_version":"release-1",
      "environment":"staging",
      "requested_by":"user-1",
      "delegated_agent":{"id":"agent-1","scopes":["release:deploy"]},
      "changes":[{"service":"identity","desired_version":"sha-1","ci_success":true,"dependencies_healthy":true}]
    }`)

	for _, path := range []string{"/v1/release-runs:plan", "/v1/release-runs"} {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if path == "/v1/release-runs:plan" && response.Code != http.StatusOK {
			t.Fatalf("plan status=%d body=%s", response.Code, response.Body.String())
		}
		if path == "/v1/release-runs" && response.Code != http.StatusCreated {
			t.Fatalf("start status=%d body=%s", response.Code, response.Body.String())
		}
	}
}

func TestVersionedIntentReturnsTypedReasonCode(t *testing.T) {
	p, err := planner.New([]domain.Service{{Name: "identity"}})
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(p, policy.New(), executor.NewMock(nil), store.NewMemory())
	handler := New(e, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	body := []byte(`{
      "api_version":"execution.agentwritegateway.io/v2",
      "kind":"ReleaseIntent",
      "request_id":"invalid-version",
      "release_version":"release-1",
      "environment":"staging",
      "requested_by":"user-1",
      "delegated_agent":{"id":"agent-1","scopes":["release:deploy"]},
      "changes":[{"service":"identity","desired_version":"sha-1","ci_success":true,"dependencies_healthy":true}]
    }`)
	request := httptest.NewRequest(http.MethodPost, "/v1/release-runs:plan", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded errorResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ReasonCode != domain.ReasonUnsupportedSchemaVersion {
		t.Fatalf("reason=%s, want %s", decoded.ReasonCode, domain.ReasonUnsupportedSchemaVersion)
	}
}

func TestPauseResumeRoutesDoNotBypassApproval(t *testing.T) {
	p, err := planner.New([]domain.Service{{Name: "identity"}})
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(p, policy.New(), executor.NewMock(nil), store.NewMemory())
	handler := New(e, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	body := []byte(`{
      "request_id":"pause-http-1","release_version":"release-1","environment":"production",
      "requested_by":"user-1","delegated_agent":{"id":"agent-1","scopes":["release:deploy","release:production"]},
      "changes":[{"service":"identity","desired_version":"sha-1","ci_success":true,"dependencies_healthy":true,"destructive_migration":true}]
    }`)
	start := httptest.NewRecorder()
	handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/v1/release-runs", bytes.NewReader(body)))
	if start.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", start.Code, start.Body.String())
	}
	var run domain.ReleaseRun
	if err := json.NewDecoder(start.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"pause", "resume"} {
		request := httptest.NewRequest(http.MethodPost, "/v1/release-runs/"+run.ID+"/"+action, nil)
		request.Header.Set("X-Actor-ID", "operator")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", action, response.Code, response.Body.String())
		}
		if err := json.NewDecoder(response.Body).Decode(&run); err != nil {
			t.Fatal(err)
		}
	}
	if run.Status != domain.RunWaitingApproval || run.Steps[0].Execution != nil {
		t.Fatalf("control crossed approval: %#v", run)
	}
}
