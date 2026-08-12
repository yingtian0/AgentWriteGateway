package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"agentwritegateway/internal/domain"
)

var (
	ErrNotFound = errors.New("release run not found")
	ErrConflict = errors.New("release run state version conflict")
)

type Store interface {
	CreateRun(*domain.ReleaseRun) (*domain.ReleaseRun, bool, error)
	GetRun(string) (*domain.ReleaseRun, error)
	UpdateRun(*domain.ReleaseRun, int64) error
	AppendAudit(domain.AuditEvent) error
	AuditEvents(string) ([]domain.AuditEvent, error)
}

type Memory struct {
	mu             sync.RWMutex
	runs           map[string]*domain.ReleaseRun
	runByRequestID map[string]string
	audit          map[string][]domain.AuditEvent
}

func NewMemory() *Memory {
	return &Memory{
		runs:           make(map[string]*domain.ReleaseRun),
		runByRequestID: make(map[string]string),
		audit:          make(map[string][]domain.AuditEvent),
	}
}

func (m *Memory) CreateRun(run *domain.ReleaseRun) (*domain.ReleaseRun, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existingID, ok := m.runByRequestID[run.RequestID]; ok {
		return cloneRun(m.runs[existingID]), false, nil
	}
	if _, exists := m.runs[run.ID]; exists {
		return nil, false, fmt.Errorf("duplicate run id %q", run.ID)
	}
	m.runs[run.ID] = cloneRun(run)
	m.runByRequestID[run.RequestID] = run.ID
	return cloneRun(run), true, nil
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
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.runs[run.ID]
	if !ok {
		return ErrNotFound
	}
	if current.StateVersion != expectedVersion {
		return ErrConflict
	}
	run.StateVersion = expectedVersion + 1
	m.runs[run.ID] = cloneRun(run)
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
	events := m.audit[correlationID]
	result := make([]domain.AuditEvent, len(events))
	copy(result, events)
	return result, nil
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
