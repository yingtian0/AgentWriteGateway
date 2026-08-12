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
	if plan.Hash == "" || len(plan.Phases) != 1 {
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
