package store

import (
	"context"
	"errors"
	"time"

	"agentwritegateway/internal/domain"
	"agentwritegateway/pkg/protocol"
)

var (
	ErrNotFound             = errors.New("resource not found")
	ErrConflict             = errors.New("release run state version conflict")
	ErrDuplicateExecution   = errors.New("duplicate adapter idempotency key")
	ErrUnknownExternalState = errors.New("external execution state is unknown")
	ErrJournalUnavailable   = errors.New("runner journal is unavailable")
)

// Store is retained as the compatibility contract used by the Packet 00/01
// engine. Production composition uses DurableStore so state, audit, and
// outbox changes can be committed atomically.
type Store interface {
	CreateRun(*domain.ReleaseRun) (*domain.ReleaseRun, bool, error)
	GetRun(string) (*domain.ReleaseRun, error)
	ListRuns() ([]*domain.ReleaseRun, error)
	UpdateRun(*domain.ReleaseRun, int64) error
	AppendAudit(domain.AuditEvent) error
	AuditEvents(string) ([]domain.AuditEvent, error)
}

type ExecutionStatus string

const (
	ExecutionReserved  ExecutionStatus = "RESERVED"
	ExecutionSucceeded ExecutionStatus = "SUCCEEDED"
	ExecutionFailed    ExecutionStatus = "FAILED"
	ExecutionUnknown   ExecutionStatus = "UNKNOWN"
)

type ExecutionRecord struct {
	ID                  string
	RunID               string
	Service             string
	Adapter             string
	IdempotencyKey      string
	Status              ExecutionStatus
	ExternalExecutionID string
	Payload             map[string]any
	StateVersion        int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type RunnerActionStatus string

const (
	RunnerActionReserved  RunnerActionStatus = "RESERVED"
	RunnerActionSucceeded RunnerActionStatus = "SUCCEEDED"
	RunnerActionUnknown   RunnerActionStatus = "UNKNOWN"
	RunnerActionRejected  RunnerActionStatus = "REJECTED"
)

type RunnerActionRecord struct {
	GrantID        string
	RunID          string
	StepID         string
	TenantID       string
	RunnerGroup    string
	Nonce          string
	IdempotencyKey string
	RequestHash    string
	Target         protocol.Target
	Action         protocol.Action
	Status         RunnerActionStatus
	Result         protocol.Result
	StateVersion   int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RunnerJournal interface {
	GetRunnerAction(context.Context, string, string, string) (RunnerActionRecord, error)
	ReserveRunnerAction(context.Context, RunnerActionRecord, domain.AuditEvent) (RunnerActionRecord, bool, error)
	CompleteRunnerAction(context.Context, RunnerActionRecord, int64, domain.AuditEvent) error
	PendingRunnerActions(context.Context, string, string, int) ([]RunnerActionRecord, error)
}

type DurableStore interface {
	Store
	RunnerJournal
	SavePlan(domain.ReleasePlan) error
	GetPlan(string) (domain.ReleasePlan, error)
	CreateRunAtomic(*domain.ReleaseRun, []domain.AuditEvent, []domain.OutboxEvent) (*domain.ReleaseRun, bool, error)
	UpdateRunAtomic(*domain.ReleaseRun, int64, []domain.AuditEvent, []domain.OutboxEvent) error
	ReserveExecution(ExecutionRecord, domain.AuditEvent, domain.OutboxEvent) (ExecutionRecord, bool, error)
	CompleteExecution(ExecutionRecord, int64, domain.AuditEvent, domain.OutboxEvent) error
	GetExecution(adapter, idempotencyKey string) (ExecutionRecord, error)
	PendingOutbox(int) ([]domain.OutboxEvent, error)
	PendingOutboxByType(string, int) ([]domain.OutboxEvent, error)
	MarkOutboxPublished(string, time.Time) error
	RebuildProjection(string) error
}
