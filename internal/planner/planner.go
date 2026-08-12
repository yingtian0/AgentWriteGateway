package planner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"agentwritegateway/internal/domain"
)

var (
	ErrNoChanges       = errors.New("at least one change is required")
	ErrDependencyCycle = errors.New("dependency cycle detected")
)

type Planner struct {
	services map[string]domain.Service
	now      func() time.Time
}

func New(services []domain.Service) (*Planner, error) {
	byName := make(map[string]domain.Service, len(services))
	for _, service := range services {
		if service.Name == "" {
			return nil, errors.New("service name is required")
		}
		if _, exists := byName[service.Name]; exists {
			return nil, fmt.Errorf("duplicate service %q", service.Name)
		}
		byName[service.Name] = service
	}
	for _, service := range services {
		for _, dependency := range service.Dependencies {
			if _, exists := byName[dependency]; !exists {
				return nil, fmt.Errorf("service %q references unknown dependency %q", service.Name, dependency)
			}
		}
	}
	return &Planner{services: byName, now: time.Now}, nil
}

func (p *Planner) Services() []domain.Service {
	services := make([]domain.Service, 0, len(p.services))
	for _, service := range p.services {
		services = append(services, service)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return services
}

func (p *Planner) Plan(request domain.ReleaseRequest) (domain.ReleasePlan, error) {
	if len(request.Changes) == 0 {
		return domain.ReleasePlan{}, ErrNoChanges
	}
	if request.Environment != domain.EnvironmentStaging && request.Environment != domain.EnvironmentProduction {
		return domain.ReleasePlan{}, fmt.Errorf("unsupported environment %q", request.Environment)
	}

	changes := make(map[string]domain.Change, len(request.Changes))
	for _, change := range request.Changes {
		if _, exists := p.services[change.Service]; !exists {
			return domain.ReleasePlan{}, fmt.Errorf("unknown service %q", change.Service)
		}
		if change.DesiredVersion == "" {
			return domain.ReleasePlan{}, fmt.Errorf("desired version is required for %q", change.Service)
		}
		if _, duplicate := changes[change.Service]; duplicate {
			return domain.ReleasePlan{}, fmt.Errorf("duplicate change for %q", change.Service)
		}
		changes[change.Service] = change
	}

	indegree := make(map[string]int, len(changes))
	children := make(map[string][]string, len(changes))
	phase := make(map[string]int, len(changes))
	for name := range changes {
		service := p.services[name]
		phase[name] = service.ReleasePhase
		for _, dependency := range service.Dependencies {
			if _, selected := changes[dependency]; selected {
				indegree[name]++
				children[dependency] = append(children[dependency], name)
			}
		}
	}

	ready := make([]string, 0)
	for name := range changes {
		if indegree[name] == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)
	processed := 0
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		processed++
		sort.Strings(children[name])
		for _, child := range children[name] {
			if phase[child] <= phase[name] {
				phase[child] = phase[name] + 1
			}
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
				sort.Strings(ready)
			}
		}
	}
	if processed != len(changes) {
		return domain.ReleasePlan{}, ErrDependencyCycle
	}

	byPhase := make(map[int][]domain.PlanStep)
	phaseNumbers := make([]int, 0)
	for name, change := range changes {
		if _, exists := byPhase[phase[name]]; !exists {
			phaseNumbers = append(phaseNumbers, phase[name])
		}
		byPhase[phase[name]] = append(byPhase[phase[name]], domain.PlanStep{
			Service: name, DesiredVersion: change.DesiredVersion, Phase: phase[name],
		})
	}
	sort.Ints(phaseNumbers)
	plan := domain.ReleasePlan{
		ReleaseVersion: request.ReleaseVersion,
		Environment:    request.Environment,
		CreatedAt:      p.now().UTC(),
	}
	for _, number := range phaseNumbers {
		steps := byPhase[number]
		sort.Slice(steps, func(i, j int) bool { return steps[i].Service < steps[j].Service })
		plan.Phases = append(plan.Phases, domain.PlanPhase{Number: number, Steps: steps})
	}
	canonical, err := json.Marshal(struct {
		ReleaseVersion string             `json:"release_version"`
		Environment    domain.Environment `json:"environment"`
		Phases         []domain.PlanPhase `json:"phases"`
	}{plan.ReleaseVersion, plan.Environment, plan.Phases})
	if err != nil {
		return domain.ReleasePlan{}, fmt.Errorf("encode plan: %w", err)
	}
	sum := sha256.Sum256(canonical)
	plan.Hash = "sha256:" + hex.EncodeToString(sum[:])
	return plan, nil
}
