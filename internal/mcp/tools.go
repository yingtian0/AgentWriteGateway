package mcp

import "encoding/json"

type Tool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
}

var tools = []Tool{
	tool("list_services", "List services in the trusted contract catalog.", objectSchema(nil, nil)),
	tool("get_service_context", "Get contract-derived context for one service.", objectSchema(map[string]any{"service": stringSchema()}, []string{"service"})),
	tool("plan_release", "Create a canonical plan from a structured ReleaseIntent; does not execute.", objectSchema(map[string]any{"intent": map[string]any{"type": "object"}}, []string{"intent"})),
	tool("explain_release_plan", "Return system reason codes separately from an optional AI-generated explanation.", objectSchema(map[string]any{"plan_id": stringSchema()}, []string{"plan_id"})),
	tool("start_release", "Start a durable release from a structured ReleaseIntent.", objectSchema(map[string]any{"intent": map[string]any{"type": "object"}}, []string{"intent"})),
	tool("get_release_status", "Get the current durable release projection.", objectSchema(map[string]any{"run_id": stringSchema()}, []string{"run_id"})),
	tool("pause_release", "Pause a release at the durable workflow boundary.", objectSchema(map[string]any{"run_id": stringSchema()}, []string{"run_id"})),
	tool("cancel_release", "Cancel a release and prevent pending downstream dispatch.", objectSchema(map[string]any{"run_id": stringSchema()}, []string{"run_id"})),
	tool("list_pending_approvals", "List pending approvals for the authenticated tenant.", objectSchema(nil, nil)),
	tool("get_incident_context", "Get a release and its append-only audit events for incident response.", objectSchema(map[string]any{"run_id": stringSchema()}, []string{"run_id"})),
}

func tool(name, description string, input map[string]any) Tool {
	return Tool{Name: name, Description: description, InputSchema: input, OutputSchema: map[string]any{"type": "object"}}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	result := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

func stringSchema() map[string]any { return map[string]any{"type": "string", "minLength": 1} }

type serviceArgument struct {
	Service string `json:"service"`
}

type intentArgument struct {
	Intent json.RawMessage `json:"intent"`
}

type planArgument struct {
	PlanID string `json:"plan_id"`
}

type runArgument struct {
	RunID string `json:"run_id"`
}
