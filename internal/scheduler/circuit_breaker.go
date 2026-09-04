package scheduler

import (
	"errors"
	"sync"
)

var ErrCircuitOpen = errors.New("production circuit breaker is open")

type CircuitPolicy struct {
	MinimumSamples int     `json:"minimum_samples" yaml:"minimum_samples"`
	WindowSize     int     `json:"window_size" yaml:"window_size"`
	ErrorRate      float64 `json:"error_rate" yaml:"error_rate"`
}

type CircuitState struct {
	Open      bool    `json:"open"`
	Samples   int     `json:"samples"`
	Failures  int     `json:"failures"`
	ErrorRate float64 `json:"error_rate"`
}

// CircuitBreaker has no public close operation. Recovery is an explicit
// operator concern outside every REST/CLI/MCP agent-facing interface.
type CircuitBreaker struct {
	mu      sync.RWMutex
	policy  CircuitPolicy
	windows map[string][]bool
	open    map[string]bool
}

func NewCircuitBreaker(policy CircuitPolicy) (*CircuitBreaker, error) {
	if policy.MinimumSamples < 1 || policy.WindowSize < policy.MinimumSamples || policy.ErrorRate <= 0 || policy.ErrorRate > 1 {
		return nil, errors.New("invalid circuit breaker policy")
	}
	return &CircuitBreaker{policy: policy, windows: make(map[string][]bool), open: make(map[string]bool)}, nil
}

func (b *CircuitBreaker) Allow(tenant, environment string) error {
	if environment != "production" {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.open[tenantKey(tenant)] {
		return ErrCircuitOpen
	}
	return nil
}

func (b *CircuitBreaker) Record(tenant, environment string, failed bool) CircuitState {
	if environment != "production" {
		return CircuitState{}
	}
	key := tenantKey(tenant)
	b.mu.Lock()
	defer b.mu.Unlock()
	window := append(b.windows[key], failed)
	if len(window) > b.policy.WindowSize {
		window = window[len(window)-b.policy.WindowSize:]
	}
	b.windows[key] = window
	state := stateOf(window, b.open[key])
	if len(window) >= b.policy.MinimumSamples && state.ErrorRate >= b.policy.ErrorRate {
		b.open[key] = true
		state.Open = true
	}
	return state
}

func (b *CircuitBreaker) State(tenant string) CircuitState {
	key := tenantKey(tenant)
	b.mu.RLock()
	defer b.mu.RUnlock()
	return stateOf(b.windows[key], b.open[key])
}

func stateOf(window []bool, open bool) CircuitState {
	failures := 0
	for _, failed := range window {
		if failed {
			failures++
		}
	}
	rate := 0.0
	if len(window) > 0 {
		rate = float64(failures) / float64(len(window))
	}
	return CircuitState{Open: open, Samples: len(window), Failures: failures, ErrorRate: rate}
}

func tenantKey(tenant string) string {
	if tenant == "" {
		return "default"
	}
	return tenant
}
