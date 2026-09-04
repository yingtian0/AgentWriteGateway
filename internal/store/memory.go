package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"themisy/internal/domain"
)

type Memory struct {
	mu             sync.RWMutex
	runs           map[string]*domain.ReleaseRun
	runByRequestID map[string]string
	audit          map[string][]domain.AuditEvent
	executions     map[string]ExecutionRecord
	outbox         []domain.OutboxEvent
	projections    map[string]*domain.ReleaseRun
	runnerActions  map[string]RunnerActionRecord
	runnerByKey    map[string]string
	journalError   error
}

func NewMemory() *Memory {
	return &Memory{
		runs: make(map[string]*domain.ReleaseRun), runByRequestID: make(map[string]string),
		audit: make(map[string][]domain.AuditEvent), executions: make(map[string]ExecutionRecord),
		projections:   make(map[string]*domain.ReleaseRun),
		runnerActions: make(map[string]RunnerActionRecord), runnerByKey: make(map[string]string),
	}
}

func (m *Memory) SetRunnerJournalError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.journalError = err
}

func (m *Memory) CreateRun(run *domain.ReleaseRun) (*domain.ReleaseRun, bool, error) {
	return m.CreateRunAtomic(run, nil, nil)
}

func (m *Memory) CreateRunAtomic(run *domain.ReleaseRun, audit []domain.AuditEvent, outbox []domain.OutboxEvent) (*domain.ReleaseRun, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existingID, ok := m.runByRequestID[run.RequestID]; ok {
		return cloneRun(m.runs[existingID]), false, nil
	}
	if _, exists := m.runs[run.ID]; exists {
		return nil, false, fmt.Errorf("duplicate run id %q", run.ID)
	}
	stored := cloneRun(run)
	m.runs[run.ID] = stored
	m.projections[run.ID] = cloneRun(run)
	m.runByRequestID[run.RequestID] = run.ID
	m.appendAtomic(run.ID, audit, appendProjection(outbox, run))
	return cloneRun(stored), true, nil
}

func (m *Memory) GetRun(id string) (*domain.ReleaseRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, ok := m.runs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneRun(run), nil
}

func (m *Memory) UpdateRun(run *domain.ReleaseRun, expectedVersion int64) error {
	return m.UpdateRunAtomic(run, expectedVersion, nil, nil)
}

func (m *Memory) UpdateRunAtomic(run *domain.ReleaseRun, expectedVersion int64, audit []domain.AuditEvent, outbox []domain.OutboxEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.runs[run.ID]
	if !ok {
		return ErrNotFound
	}
	if current.StateVersion != expectedVersion {
		return ErrConflict
	}
	updated := cloneRun(run)
	updated.StateVersion = expectedVersion + 1
	m.runs[run.ID] = updated
	m.projections[run.ID] = cloneRun(updated)
	run.StateVersion = updated.StateVersion
	m.appendAtomic(run.ID, audit, appendProjection(outbox, updated))
	return nil
}

func (m *Memory) AppendAudit(event domain.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit[event.CorrelationID] = append(m.audit[event.CorrelationID], event)
	return nil
}

func (m *Memory) AuditEvents(correlationID string) ([]domain.AuditEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.AuditEvent(nil), m.audit[correlationID]...), nil
}

func (m *Memory) ReserveExecution(record ExecutionRecord, audit domain.AuditEvent, outbox domain.OutboxEvent) (ExecutionRecord, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := executionKey(record.Adapter, record.IdempotencyKey)
	if existing, ok := m.executions[key]; ok {
		return cloneExecution(existing), false, nil
	}
	record.StateVersion = 1
	record.Status = ExecutionReserved
	m.executions[key] = cloneExecution(record)
	m.appendAtomic(record.RunID, []domain.AuditEvent{audit}, []domain.OutboxEvent{outbox})
	return cloneExecution(record), true, nil
}

func (m *Memory) CompleteExecution(record ExecutionRecord, expectedVersion int64, audit domain.AuditEvent, outbox domain.OutboxEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := executionKey(record.Adapter, record.IdempotencyKey)
	current, ok := m.executions[key]
	if !ok {
		return ErrNotFound
	}
	if current.StateVersion != expectedVersion {
		return ErrConflict
	}
	record.StateVersion = expectedVersion + 1
	m.executions[key] = cloneExecution(record)
	m.appendAtomic(record.RunID, []domain.AuditEvent{audit}, []domain.OutboxEvent{outbox})
	return nil
}

func (m *Memory) GetExecution(adapter, idempotencyKey string) (ExecutionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.executions[executionKey(adapter, idempotencyKey)]
	if !ok {
		return ExecutionRecord{}, ErrNotFound
	}
	return cloneExecution(record), nil
}

func (m *Memory) PendingOutbox(limit int) ([]domain.OutboxEvent, error) {
	return m.pendingOutbox("", limit), nil
}

func (m *Memory) PendingOutboxByType(eventType string, limit int) ([]domain.OutboxEvent, error) {
	return m.pendingOutbox(eventType, limit), nil
}

func (m *Memory) pendingOutbox(eventType string, limit int) []domain.OutboxEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.outbox) {
		limit = len(m.outbox)
	}
	result := make([]domain.OutboxEvent, 0, limit)
	for _, event := range m.outbox {
		if event.PublishedAt == nil && (eventType == "" || event.EventType == eventType) {
			result = append(result, event)
			if len(result) == limit {
				break
			}
		}
	}
	return result
}

func (m *Memory) MarkOutboxPublished(id string, publishedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.outbox {
		if m.outbox[i].ID == id {
			m.outbox[i].PublishedAt = &publishedAt
			return nil
		}
	}
	return ErrNotFound
}

func (m *Memory) RebuildProjection(runID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	projection, ok := m.projections[runID]
	if !ok {
		return ErrNotFound
	}
	m.runs[runID] = cloneRun(projection)
	return nil
}

func (m *Memory) GetRunnerAction(_ context.Context, tenantID, runnerGroup, nonce string) (RunnerActionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.journalError != nil {
		return RunnerActionRecord{}, m.journalError
	}
	record, ok := m.runnerActions[runnerNonceKey(tenantID, runnerGroup, nonce)]
	if !ok {
		return RunnerActionRecord{}, ErrNotFound
	}
	return record, nil
}

func (m *Memory) ReserveRunnerAction(_ context.Context, record RunnerActionRecord, audit domain.AuditEvent) (RunnerActionRecord, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.journalError != nil {
		return RunnerActionRecord{}, false, m.journalError
	}
	nonceKey := runnerNonceKey(record.TenantID, record.RunnerGroup, record.Nonce)
	if existing, ok := m.runnerActions[nonceKey]; ok {
		return existing, false, nil
	}
	idempotencyKey := runnerIdempotencyKey(record.TenantID, record.RunnerGroup, record.IdempotencyKey)
	if existingNonce, ok := m.runnerByKey[idempotencyKey]; ok {
		return m.runnerActions[existingNonce], false, nil
	}
	record.Status, record.StateVersion = RunnerActionReserved, 1
	m.runnerActions[nonceKey] = record
	m.runnerByKey[idempotencyKey] = nonceKey
	m.audit[audit.CorrelationID] = append(m.audit[audit.CorrelationID], audit)
	return record, true, nil
}

func (m *Memory) CompleteRunnerAction(_ context.Context, record RunnerActionRecord, expected int64, audit domain.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.journalError != nil {
		return m.journalError
	}
	key := runnerNonceKey(record.TenantID, record.RunnerGroup, record.Nonce)
	current, ok := m.runnerActions[key]
	if !ok {
		return ErrNotFound
	}
	if current.StateVersion != expected {
		return ErrConflict
	}
	record.StateVersion = expected + 1
	m.runnerActions[key] = record
	m.audit[audit.CorrelationID] = append(m.audit[audit.CorrelationID], audit)
	return nil
}

func (m *Memory) PendingRunnerActions(_ context.Context, tenantID, runnerGroup string, limit int) ([]RunnerActionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.journalError != nil {
		return nil, m.journalError
	}
	result := []RunnerActionRecord{}
	for _, record := range m.runnerActions {
		if record.TenantID == tenantID && record.RunnerGroup == runnerGroup && (record.Status == RunnerActionReserved || record.Status == RunnerActionUnknown) {
			result = append(result, record)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *Memory) appendAtomic(runID string, audit []domain.AuditEvent, outbox []domain.OutboxEvent) {
	for _, event := range audit {
		m.audit[event.CorrelationID] = append(m.audit[event.CorrelationID], event)
	}
	m.outbox = append(m.outbox, outbox...)
}

func appendProjection(events []domain.OutboxEvent, run *domain.ReleaseRun) []domain.OutboxEvent {
	payload, _ := json.Marshal(run)
	event := domain.OutboxEvent{
		ID: fmt.Sprintf("projection/%s/%d", run.ID, run.StateVersion), AggregateType: "release_run",
		AggregateID: run.ID, EventType: "release.run.project", CreatedAt: run.UpdatedAt,
		AvailableAt: run.UpdatedAt, Payload: map[string]any{"run": json.RawMessage(payload)},
	}
	return append(append([]domain.OutboxEvent(nil), events...), event)
}

func executionKey(adapter, idempotencyKey string) string { return adapter + "\x00" + idempotencyKey }
func runnerNonceKey(tenant, runnerGroup, nonce string) string {
	return tenant + "\x00" + runnerGroup + "\x00" + nonce
}
func runnerIdempotencyKey(tenant, runnerGroup, key string) string {
	return tenant + "\x00" + runnerGroup + "\x00" + key
}

func cloneRun(run *domain.ReleaseRun) *domain.ReleaseRun {
	data, err := json.Marshal(run)
	if err != nil {
		panic(err)
	}
	var cloned domain.ReleaseRun
	if err := json.Unmarshal(data, &cloned); err != nil {
		panic(err)
	}
	return &cloned
}

func cloneExecution(record ExecutionRecord) ExecutionRecord {
	result := record
	if record.Payload != nil {
		result.Payload = make(map[string]any, len(record.Payload))
		for key, value := range record.Payload {
			result.Payload[key] = value
		}
	}
	return result
}
