package scheduler

import "errors"

var ErrBackpressure = errors.New("dispatch backpressured")

type Dispatch struct {
	Step        Step               `json:"step"`
	Reservation Reservation        `json:"-"`
	BlockedBy   []Blocked          `json:"blocked_by,omitempty"`
	Pressure    BackpressureReason `json:"pressure,omitempty"`
	Err         error              `json:"-"`
}

type Scheduler struct {
	budget  *Budget
	breaker *CircuitBreaker
}

func New(limits Limits, policy CircuitPolicy) (*Scheduler, error) {
	budget, err := NewBudget(limits)
	if err != nil {
		return nil, err
	}
	breaker, err := NewCircuitBreaker(policy)
	if err != nil {
		return nil, err
	}
	return &Scheduler{budget: budget, breaker: breaker}, nil
}

func (s *Scheduler) Dispatch(steps []Step, capacity Capacity) []Dispatch {
	available, pressure := capacity.Available()
	results := make([]Dispatch, 0, len(steps))
	for _, step := range steps {
		result := Dispatch{Step: cloneStep(step)}
		if available == 0 {
			result.Pressure = pressure
			result.Err = ErrBackpressure
			results = append(results, result)
			continue
		}
		if err := s.breaker.Allow(step.Tenant, step.Environment); err != nil {
			result.Err = err
			results = append(results, result)
			continue
		}
		reservation, blocked := s.budget.TryAcquire(step)
		if len(blocked) > 0 {
			result.BlockedBy = blocked
			results = append(results, result)
			continue
		}
		result.Reservation = reservation
		available--
		results = append(results, result)
	}
	return results
}

func (s *Scheduler) Complete(dispatch Dispatch, failed bool) CircuitState {
	if len(dispatch.Reservation.keys) > 0 {
		s.budget.Release(dispatch.Reservation)
	}
	return s.breaker.Record(dispatch.Step.Tenant, dispatch.Step.Environment, failed)
}

func (s *Scheduler) CircuitState(tenant string) CircuitState {
	return s.breaker.State(tenant)
}
