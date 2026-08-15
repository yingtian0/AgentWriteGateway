package policy

import (
	"testing"

	"agentwritegateway/internal/domain"
)

func TestPolicySafetyRules(t *testing.T) {
	base := Input{
		UserID: "user", AgentID: "agent",
		AgentScopes: []string{"release:deploy", "release:production"},
		Environment: domain.EnvironmentProduction, Service: "payments",
		CISuccess: true, DependenciesHealthy: true,
	}
	tests := []struct {
		name   string
		mutate func(*Input)
		want   domain.Decision
		reason string
	}{
		{name: "allow", mutate: func(*Input) {}, want: domain.DecisionAllow, reason: "POLICY_ALLOW"},
		{name: "missing deploy scope", mutate: func(i *Input) { i.AgentScopes = []string{"release:production"} }, want: domain.DecisionDeny, reason: "MISSING_DELEGATED_SCOPE"},
		{name: "missing production scope", mutate: func(i *Input) { i.AgentScopes = []string{"release:deploy"} }, want: domain.DecisionDeny, reason: "MISSING_PRODUCTION_SCOPE"},
		{name: "ci failed", mutate: func(i *Input) { i.CISuccess = false }, want: domain.DecisionDeny, reason: "CI_NOT_SUCCESSFUL"},
		{name: "dependency unhealthy", mutate: func(i *Input) { i.DependenciesHealthy = false }, want: domain.DecisionDeny, reason: "DEPENDENCIES_UNHEALTHY"},
		{name: "destructive migration", mutate: func(i *Input) { i.DestructiveMigration = true }, want: domain.DecisionRequireApproval, reason: "DESTRUCTIVE_DB_MIGRATION"},
		{name: "high risk", mutate: func(i *Input) { i.Risk = "high" }, want: domain.DecisionRequireApproval, reason: "HIGH_BLAST_RADIUS"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			test.mutate(&input)
			got := New().Evaluate(input)
			if got.Decision != test.want || got.ReasonCode != test.reason {
				t.Fatalf("got %s/%s, want %s/%s", got.Decision, got.ReasonCode, test.want, test.reason)
			}
			if got.InputHash == "" || got.PolicyVersion == "" {
				t.Fatal("decision must preserve policy version and input hash")
			}
		})
	}
}
