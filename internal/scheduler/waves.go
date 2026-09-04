package scheduler

import (
	"fmt"
	"sort"
)

type Wave struct {
	Number int    `json:"number"`
	Steps  []Step `json:"steps"`
}

// BuildWaves deterministically groups ready steps into the widest safe waves.
// Reservations are released only at a wave boundary, so every limit is an
// upper bound on simultaneous dispatch.
func BuildWaves(steps []Step, limits Limits) ([]Wave, error) {
	budget, err := NewBudget(limits)
	if err != nil {
		return nil, err
	}
	pending := make(map[string]Step, len(steps))
	for _, step := range steps {
		if step.ID == "" {
			return nil, fmt.Errorf("scheduled step id is required")
		}
		if _, duplicate := pending[step.ID]; duplicate {
			return nil, fmt.Errorf("duplicate scheduled step %q", step.ID)
		}
		pending[step.ID] = cloneStep(step)
	}
	completed := make(map[string]bool, len(steps))
	waves := make([]Wave, 0)
	for len(pending) > 0 {
		ready := make([]Step, 0)
		for _, step := range pending {
			if dependenciesComplete(step, completed) {
				ready = append(ready, step)
			}
		}
		sort.Slice(ready, func(i, j int) bool {
			if ready[i].Phase != ready[j].Phase {
				return ready[i].Phase < ready[j].Phase
			}
			return ready[i].ID < ready[j].ID
		})
		if len(ready) == 0 {
			return nil, fmt.Errorf("dependency cycle or unknown dependency in scheduling input")
		}
		minimumPhase := ready[0].Phase
		wave := Wave{Number: len(waves)}
		reservations := make([]Reservation, 0)
		for _, step := range ready {
			if step.Phase != minimumPhase {
				break
			}
			reservation, blocked := budget.TryAcquire(step)
			if len(blocked) > 0 {
				continue
			}
			wave.Steps = append(wave.Steps, step)
			reservations = append(reservations, reservation)
		}
		if len(wave.Steps) == 0 {
			return nil, fmt.Errorf("configured limits prevent every ready step from dispatching")
		}
		for _, reservation := range reservations {
			budget.Release(reservation)
		}
		for _, step := range wave.Steps {
			delete(pending, step.ID)
			completed[step.ID] = true
		}
		waves = append(waves, wave)
	}
	return waves, nil
}

func dependenciesComplete(step Step, completed map[string]bool) bool {
	for _, dependency := range step.Dependencies {
		if !completed[dependency] {
			return false
		}
	}
	return true
}

func cloneStep(step Step) Step {
	step.FailureDomains = append([]string(nil), step.FailureDomains...)
	step.Dependencies = append([]string(nil), step.Dependencies...)
	sort.Strings(step.FailureDomains)
	sort.Strings(step.Dependencies)
	return step
}
