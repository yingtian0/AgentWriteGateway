package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"agentwritegateway/internal/domain"
)

const Version = "builtin-v1"

type Input struct {
	UserID               string             `json:"user_id"`
	AgentID              string             `json:"agent_id"`
	AgentScopes          []string           `json:"agent_scopes"`
	Environment          domain.Environment `json:"environment"`
	Service              string             `json:"service"`
	CISuccess            bool               `json:"ci_success"`
	DependenciesHealthy  bool               `json:"dependencies_healthy"`
	DestructiveMigration bool               `json:"destructive_migration"`
	Risk                 string             `json:"risk"`
}

type Engine struct {
	now func() time.Time
}

func New() *Engine { return &Engine{now: time.Now} }

func (e *Engine) Evaluate(input Input) domain.PolicyDecision {
	canonical, _ := json.Marshal(input)
	sum := sha256.Sum256(canonical)
	decision := domain.PolicyDecision{
		PolicyVersion: Version,
		InputHash:     "sha256:" + hex.EncodeToString(sum[:]),
		CreatedAt:     e.now().UTC(),
	}

	scopes := make(map[string]bool, len(input.AgentScopes))
	for _, scope := range input.AgentScopes {
		scopes[scope] = true
	}
	switch {
	case input.UserID == "" || input.AgentID == "":
		decision.Decision = domain.DecisionDeny
		decision.ReasonCode = "MISSING_SUBJECT"
		decision.ReasonDetail = "both user and delegated agent identities are required"
	case !scopes["release:deploy"]:
		decision.Decision = domain.DecisionDeny
		decision.ReasonCode = "MISSING_DELEGATED_SCOPE"
		decision.ReasonDetail = "agent delegation does not include release:deploy"
	case input.Environment == domain.EnvironmentProduction && !scopes["release:production"]:
		decision.Decision = domain.DecisionDeny
		decision.ReasonCode = "MISSING_PRODUCTION_SCOPE"
		decision.ReasonDetail = "production releases require release:production"
	case !input.CISuccess:
		decision.Decision = domain.DecisionDeny
		decision.ReasonCode = "CI_NOT_SUCCESSFUL"
		decision.ReasonDetail = "required CI checks have not succeeded"
	case !input.DependenciesHealthy:
		decision.Decision = domain.DecisionDeny
		decision.ReasonCode = "DEPENDENCIES_UNHEALTHY"
		decision.ReasonDetail = "one or more dependencies are unhealthy"
	case input.DestructiveMigration:
		decision.Decision = domain.DecisionRequireApproval
		decision.ReasonCode = "DESTRUCTIVE_DB_MIGRATION"
		decision.ReasonDetail = "destructive database migrations require explicit risk acceptance"
		decision.RequiredRoles = []string{"service-owner", "sre"}
	case input.Risk == "high":
		decision.Decision = domain.DecisionRequireApproval
		decision.ReasonCode = "HIGH_BLAST_RADIUS"
		decision.ReasonDetail = "high-risk changes require service owner approval"
		decision.RequiredRoles = []string{"service-owner"}
	default:
		decision.Decision = domain.DecisionAllow
		decision.ReasonCode = "POLICY_ALLOW"
		decision.ReasonDetail = "all deterministic preconditions are satisfied"
	}
	return decision
}
