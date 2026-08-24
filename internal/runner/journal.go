package runner

import (
	"fmt"
	"time"

	"agentwritegateway/internal/domain"
)

func journalAudit(id, correlationID, action, result string, at time.Time, details map[string]any) domain.AuditEvent {
	return domain.AuditEvent{ID: id, CorrelationID: correlationID, ActorType: "runner", ActorID: "customer-runner", Action: action, ResourceType: "action_grant", ResourceID: correlationID, Result: result, Details: details, Timestamp: at.UTC()}
}

func auditID(grantID, action string, version int64) string {
	return fmt.Sprintf("runner/%s/%s/%d", grantID, action, version)
}
