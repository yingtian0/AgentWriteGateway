package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentwritegateway/internal/api"
	"agentwritegateway/internal/application"
	"agentwritegateway/internal/domain"
	"agentwritegateway/internal/planner"
	"agentwritegateway/internal/store"
	workflowcore "agentwritegateway/internal/workflow"
)

func TestMCPAndRESTReturnSamePlanHash(t *testing.T) {
	app := testApplication(t)
	principal := Principal{Subject: "operator", TenantID: "tenant-a"}
	server := New(app, principal, nil)
	intent := testIntent()
	arguments, _ := json.Marshal(map[string]any{"intent": intent})
	result, err := server.Call(context.Background(), "plan_release", arguments)
	if err != nil {
		t.Fatal(err)
	}
	mcpPlan := decodePlan(t, result["plan"])

	rest := httptest.NewServer(api.New(app, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer rest.Close()
	body, _ := json.Marshal(intent)
	request, _ := http.NewRequest(http.MethodPost, rest.URL+"/v1/plans", bytes.NewReader(body))
	request.Header.Set("X-Tenant-ID", principal.TenantID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var restPlan domain.ReleasePlan
	if err := json.NewDecoder(response.Body).Decode(&restPlan); err != nil {
		t.Fatal(err)
	}
	if mcpPlan.Hash != restPlan.Hash {
		t.Fatalf("MCP hash=%s REST hash=%s", mcpPlan.Hash, restPlan.Hash)
	}
}

func TestMCPRejectsInvalidSchemaAndProhibitedTools(t *testing.T) {
	server := New(testApplication(t), Principal{TenantID: "tenant-a"}, nil)
	if _, err := server.Call(context.Background(), "get_release_status", json.RawMessage(`{"run_id":"run","shell":"rm"}`)); err == nil {
		t.Fatal("unknown schema field was accepted")
	}
	for _, name := range []string{"shell", "http", "generic_cloud", "mutate_policy"} {
		if _, err := server.Call(context.Background(), name, json.RawMessage(`{}`)); err == nil {
			t.Fatalf("prohibited tool %q was accepted", name)
		}
	}
}

func TestMCPHTTPListsOnlyBoundedToolsAndReturnsStructuredContent(t *testing.T) {
	server := New(testApplication(t), Principal{TenantID: "tenant-a"}, nil)
	list := rpcRequest(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	result := list["result"].(map[string]any)
	listed := result["tools"].([]any)
	if len(listed) != 10 {
		t.Fatalf("tools=%d want=10", len(listed))
	}
	call := rpcRequest(t, server, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_services","arguments":{}}}`)
	callResult := call["result"].(map[string]any)
	if _, ok := callResult["structuredContent"].(map[string]any); !ok {
		t.Fatalf("missing structured content: %#v", callResult)
	}
}

func TestExplainSeparatesSystemReasonsFromAIText(t *testing.T) {
	app := testApplication(t)
	server := New(app, Principal{TenantID: "tenant-a"}, nil)
	intent := testIntent()
	plan, err := app.PlanIntentForTenant("tenant-a", intent)
	if err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(map[string]string{"plan_id": plan.ID})
	result, err := server.Call(context.Background(), "explain_release_plan", arguments)
	if err != nil {
		t.Fatal(err)
	}
	if result["ai_explanation"] != nil {
		t.Fatalf("AI explanation should be absent without explainer: %#v", result)
	}
	if reasons, ok := result["system_reasons"].([]map[string]string); !ok || len(reasons) == 0 || reasons[0]["source"] != "system" {
		t.Fatalf("system reasons=%#v", result["system_reasons"])
	}
}

func rpcRequest(t *testing.T, handler http.Handler, body string) map[string]any {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var value map[string]any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func decodePlan(t *testing.T, value any) domain.ReleasePlan {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var plan domain.ReleasePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func testIntent() domain.ReleaseIntent {
	return domain.ReleaseIntent{APIVersion: domain.ReleaseIntentAPIVersion, Kind: domain.ReleaseIntentKind, RequestID: "mcp-plan", ReleaseVersion: "release-1", TenantID: "tenant-a", Environment: domain.EnvironmentStaging, RequestedBy: "user", Agent: domain.AgentIdentity{ID: "agent", Scopes: []string{"release:deploy"}}, Changes: []domain.Change{{Service: "identity", DesiredVersion: "sha-1", CISuccess: true, DependenciesHealthy: true}}}
}

func testApplication(t *testing.T) *application.Releases {
	t.Helper()
	p, err := planner.New([]domain.Service{{Name: "identity", OwnerTeam: "identity", RiskTier: "high"}})
	if err != nil {
		t.Fatal(err)
	}
	return application.NewReleases(p, store.NewMemory(), noopController{})
}

type noopController struct{}

func (noopController) StartRelease(context.Context, workflowcore.ReleaseInput) (workflowcore.Execution, error) {
	return workflowcore.Execution{}, nil
}
func (noopController) SignalApproval(context.Context, string, workflowcore.ApprovalSignal) error {
	return nil
}
func (noopController) Pause(context.Context, string, workflowcore.ControlSignal) error  { return nil }
func (noopController) Resume(context.Context, string, workflowcore.ControlSignal) error { return nil }
func (noopController) Cancel(context.Context, string, workflowcore.ControlSignal) error { return nil }
