package application

import (
	"context"
	"fmt"
	"time"

	workflowcore "themisy/internal/workflow"
)

const workflowRequestedEvent = "release.workflow.requested"

// RecoverPendingWorkflows publishes workflow-start outbox records. Starting a
// Temporal workflow uses the run ID as Workflow ID, so replaying this publisher
// is idempotent across control-plane crashes.
func (r *Releases) RecoverPendingWorkflows(ctx context.Context, limit int) (int, error) {
	events, err := r.store.PendingOutboxByType(workflowRequestedEvent, limit)
	if err != nil {
		return 0, err
	}
	published := 0
	for _, event := range events {
		run, err := r.store.GetRun(event.AggregateID)
		if err != nil {
			return published, fmt.Errorf("load outbox run %s: %w", event.AggregateID, err)
		}
		if _, err := r.workflows.StartRelease(ctx, workflowcore.ReleaseInput{Run: *run}); err != nil {
			return published, fmt.Errorf("publish workflow request %s: %w", event.ID, err)
		}
		if err := r.store.MarkOutboxPublished(event.ID, r.now().UTC()); err != nil {
			return published, fmt.Errorf("mark workflow request %s published: %w", event.ID, err)
		}
		published++
	}
	return published, nil
}

func (r *Releases) RunWorkflowRecovery(ctx context.Context, interval time.Duration, report func(error)) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := r.RecoverPendingWorkflows(ctx, 100); err != nil && report != nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
