package executor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type DeployRequest struct {
	Service        string
	Environment    string
	DesiredVersion string
	IdempotencyKey string
}

type Deployment struct {
	ExternalID string
	StartedAt  time.Time
	FinishedAt time.Time
}

type VerificationResult struct {
	Healthy       bool
	Reason        string
	ObservedValue float64
	Threshold     float64
}

type ReleaseExecutor interface {
	Deploy(context.Context, DeployRequest) (Deployment, error)
	Verify(context.Context, Deployment) (VerificationResult, error)
	Rollback(context.Context, Deployment) error
}

type MockBehavior struct {
	DeployError   bool
	VerifyHealthy bool
	VerifyError   bool
	RollbackError bool
}

type Mock struct {
	mu          sync.Mutex
	deployments map[string]Deployment
	serviceByID map[string]string
	behavior    map[string]MockBehavior
	deployCalls map[string]int
}

func NewMock(behavior map[string]MockBehavior) *Mock {
	return &Mock{
		deployments: make(map[string]Deployment), serviceByID: make(map[string]string),
		behavior: behavior, deployCalls: make(map[string]int),
	}
}

func (m *Mock) Deploy(_ context.Context, request DeployRequest) (Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.deployments[request.IdempotencyKey]; ok {
		return existing, nil
	}
	m.deployCalls[request.IdempotencyKey]++
	if m.behavior[request.Service].DeployError {
		return Deployment{}, errors.New("simulated deploy failure")
	}
	now := time.Now().UTC()
	deployment := Deployment{
		ExternalID: fmt.Sprintf("mock-%s-%d", request.Service, len(m.deployments)+1),
		StartedAt:  now, FinishedAt: now,
	}
	m.deployments[request.IdempotencyKey] = deployment
	m.serviceByID[deployment.ExternalID] = request.Service
	return deployment, nil
}

func (m *Mock) Verify(_ context.Context, deployment Deployment) (VerificationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	behavior, configured := m.behavior[m.serviceByID[deployment.ExternalID]]
	if behavior.VerifyError {
		return VerificationResult{}, errors.New("simulated metrics failure")
	}
	if configured && !behavior.VerifyHealthy {
		return VerificationResult{Healthy: false, Reason: "simulated error rate exceeded", ObservedValue: 8.3, Threshold: 1.0}, nil
	}
	return VerificationResult{Healthy: true, Reason: "mock health checks passed", ObservedValue: 0.1, Threshold: 1.0}, nil
}

func (m *Mock) Rollback(_ context.Context, deployment Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.behavior[m.serviceByID[deployment.ExternalID]].RollbackError {
		return errors.New("simulated rollback failure")
	}
	return nil
}

func (m *Mock) DeployCalls(idempotencyKey string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deployCalls[idempotencyKey]
}
