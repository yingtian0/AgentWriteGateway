package workflow

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"themisy/internal/audit"
	"themisy/internal/domain"
	"themisy/internal/executor"
	"themisy/pkg/adapter"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type ReleaseInput struct{ Run domain.ReleaseRun }

func ReleaseWorkflow(ctx workflow.Context, input ReleaseInput) (domain.ReleaseRun, error) {
	run := input.Run
	if err := workflow.SetQueryHandler(ctx, QueryState, func() (domain.ReleaseRun, error) { return run, nil }); err != nil {
		return run, err
	}
	activityContext := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: time.Minute, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, BackoffCoefficient: 2, MaximumInterval: 10 * time.Second, MaximumAttempts: 3}})
	approvalChannel := workflow.GetSignalChannel(ctx, SignalApproval)
	pauseChannel := workflow.GetSignalChannel(ctx, SignalPause)
	resumeChannel := workflow.GetSignalChannel(ctx, SignalResume)
	cancelChannel := workflow.GetSignalChannel(ctx, SignalCancel)
	run.Status = domain.RunRunning
	run.UpdatedAt = workflow.Now(ctx).UTC()
	var err error
	run, err = persist(activityContext, run, []AuditIntent{{ActorType: "system", ActorID: "temporal", Action: "release.workflow.start", ResourceType: "release_run", ResourceID: run.ID, Result: "RUNNING"}}, "release.workflow.started")
	if err != nil {
		return run, err
	}
	succeeded := map[string]bool{}
	selected := map[string]bool{}
	for _, step := range run.Steps {
		selected[step.Service] = true
		if step.Status == domain.StepSucceeded {
			succeeded[step.Service] = true
		}
	}
	for index := range run.Steps {
		if cancelled := cancelChannel.ReceiveAsync(&ControlSignal{}); cancelled {
			return cancelRun(activityContext, ctx, run, index, "signal")
		}
		if pauseChannel.ReceiveAsync(&ControlSignal{}) {
			run, err = waitForResume(activityContext, ctx, run, resumeChannel, cancelChannel, index)
			if err != nil || run.Status == domain.RunCancelled {
				return run, err
			}
		}
		step := &run.Steps[index]
		if step.Status == domain.StepSucceeded {
			continue
		}
		if unmet := unmetDependency(run.Plan, step.Service, selected, succeeded); unmet != "" {
			step.Status = domain.StepBlocked
			step.Failure = "selected dependency did not succeed: " + unmet
			run.Status = domain.RunBlocked
			cancelDownstream(&run, index+1)
			run.UpdatedAt = workflow.Now(ctx).UTC()
			return persist(activityContext, run, []AuditIntent{{ActorType: "system", ActorID: "temporal", Action: "dependency.block", ResourceType: "service", ResourceID: step.Service, Result: "BLOCKED"}}, "release.blocked")
		}
		var decision domain.PolicyDecision
		if err := workflow.ExecuteActivity(activityContext, ActivityEvaluateStep, EvaluateInput{Run: run, Step: *step}).Get(activityContext, &decision); err != nil {
			return run, err
		}
		step.Policy = &decision
		run.UpdatedAt = workflow.Now(ctx).UTC()
		run, err = persist(activityContext, run, []AuditIntent{{ActorType: "system", ActorID: "policy-engine", Action: "policy.evaluate", ResourceType: "release_step", ResourceID: step.Service, Result: string(decision.Decision), Details: map[string]any{"reason_code": decision.ReasonCode, "input_hash": decision.InputHash}}}, "policy.evaluated")
		if err != nil {
			return run, err
		}
		step = &run.Steps[index]
		if decision.Decision == domain.DecisionDeny {
			step.Status = domain.StepBlocked
			step.Failure = decision.ReasonDetail
			run.Status = domain.RunBlocked
			cancelDownstream(&run, index+1)
			run.UpdatedAt = workflow.Now(ctx).UTC()
			return persist(activityContext, run, nil, "release.blocked")
		}
		if decision.Decision == domain.DecisionRequireApproval {
			run, err = awaitApproval(activityContext, ctx, run, index, decision, approvalChannel, pauseChannel, resumeChannel, cancelChannel)
			if err != nil || run.Status != domain.RunRunning {
				return run, err
			}
			step = &run.Steps[index]
		}
		step.Status = domain.StepExecuting
		run.UpdatedAt = workflow.Now(ctx).UTC()
		run, err = persist(activityContext, run, nil, "deployment.executing")
		if err != nil {
			return run, err
		}
		step = &run.Steps[index]
		deployInput := DeployInput{RunID: run.ID, RequestedBy: run.RequestedBy, AgentID: run.Agent.ID, Service: step.Service, Environment: string(run.Environment), DesiredVersion: step.Change.DesiredVersion, IdempotencyKey: fmt.Sprintf("%s/%s/%s/%s", run.ID, run.Environment, step.Service, step.Change.DesiredVersion)}
		var deployed DeployResult
		if err := workflow.ExecuteActivity(activityContext, ActivityDeploy, deployInput).Get(activityContext, &deployed); err != nil {
			step.Status = domain.StepUnknown
			step.Failure = "deployment state unknown; reconciliation required"
			run.Status = domain.RunBlocked
			cancelDownstream(&run, index+1)
			run.UpdatedAt = workflow.Now(ctx).UTC()
			run, persistErr := persist(activityContext, run, nil, "deployment.unknown")
			if persistErr != nil {
				return run, persistErr
			}
			return run, nil
		}
		step.Execution = &deployed.Execution
		if !step.VerificationRequired {
			step.Status = domain.StepSucceeded
			succeeded[step.Service] = true
			run.UpdatedAt = workflow.Now(ctx).UTC()
			run, err = persist(activityContext, run, nil, "step.succeeded")
			if err != nil {
				return run, err
			}
			continue
		}
		step.Status = domain.StepVerifying
		run.UpdatedAt = workflow.Now(ctx).UTC()
		run, err = persist(activityContext, run, nil, "deployment.started")
		if err != nil {
			return run, err
		}
		step = &run.Steps[index]
		if err := workflow.Sleep(ctx, observationDuration(step.ObservationWindow)); err != nil {
			return run, err
		}
		var verification executor.VerificationResult
		if err := workflow.ExecuteActivity(activityContext, ActivityVerify, VerifyInput{Deployment: deployed.Deployment}).Get(activityContext, &verification); err != nil {
			step.Status = domain.StepBlocked
			step.Failure = "verification unavailable: " + err.Error()
			run.Status = domain.RunBlocked
			cancelDownstream(&run, index+1)
			run.UpdatedAt = workflow.Now(ctx).UTC()
			return persist(activityContext, run, nil, "verification.unavailable")
		}
		step.Verification = verificationProjection(verification, workflow.Now(ctx).UTC())
		if verification.Outcome() != adapter.VerificationPass {
			if step.RollbackMode != domain.RollbackAutomatic {
				step.Status = domain.StepEscalated
				step.Failure = "verification " + string(verification.Outcome()) + "; automatic rollback is not authorized: " + string(step.RollbackMode)
				run.Status = domain.RunEscalated
				cancelDownstream(&run, index+1)
				run.UpdatedAt = workflow.Now(ctx).UTC()
				return persist(activityContext, run, []AuditIntent{{ActorType: "system", ActorID: "verifier", Action: "deployment.verify", ResourceType: "service", ResourceID: step.Service, Result: string(verification.Outcome()), Details: audit.EvidenceDetails(verification.Evidence)}}, "release.escalated")
			}
			step.Status = domain.StepRollingBack
			run.UpdatedAt = workflow.Now(ctx).UTC()
			run, err = persist(activityContext, run, nil, "rollback.started")
			if err != nil {
				return run, err
			}
			step = &run.Steps[index]
			var rolledBack RollbackResult
			if err := workflow.ExecuteActivity(activityContext, ActivityRollback, RollbackInput{Deploy: deployInput, Deployment: deployed.Deployment}).Get(activityContext, &rolledBack); err != nil {
				step.Status = domain.StepEscalated
				step.Failure = "rollback failed or unknown: " + err.Error()
				run.Status = domain.RunEscalated
			} else {
				step.RollbackExecution = &rolledBack.Execution
				if err := workflow.Sleep(ctx, observationDuration(step.ObservationWindow)); err != nil {
					return run, err
				}
				var rollbackVerification executor.VerificationResult
				if err := workflow.ExecuteActivity(activityContext, ActivityVerify, VerifyInput{Deployment: rolledBack.Deployment}).Get(activityContext, &rollbackVerification); err != nil {
					step.Status = domain.StepEscalated
					step.Failure = "rollback verification unavailable: " + err.Error()
					run.Status = domain.RunEscalated
				} else {
					step.RollbackVerification = verificationProjection(rollbackVerification, workflow.Now(ctx).UTC())
					if rollbackVerification.Outcome() != adapter.VerificationPass {
						step.Status = domain.StepEscalated
						step.Failure = "rollback verification did not pass: " + string(rollbackVerification.Outcome())
						run.Status = domain.RunEscalated
					} else {
						step.Status = domain.StepRolledBack
						step.Failure = verification.Reason
						run.Status = domain.RunFailed
					}
				}
			}
			cancelDownstream(&run, index+1)
			run.UpdatedAt = workflow.Now(ctx).UTC()
			return persist(activityContext, run, nil, "rollback.finished")
		}
		step.Status = domain.StepSucceeded
		succeeded[step.Service] = true
		run.UpdatedAt = workflow.Now(ctx).UTC()
		run, err = persist(activityContext, run, []AuditIntent{{ActorType: "system", ActorID: "verifier", Action: "deployment.verify", ResourceType: "service", ResourceID: step.Service, Result: "SUCCEEDED", Details: audit.EvidenceDetails(verification.Evidence)}}, "step.succeeded")
		if err != nil {
			return run, err
		}
	}
	run.Status = domain.RunSucceeded
	run.UpdatedAt = workflow.Now(ctx).UTC()
	return persist(activityContext, run, []AuditIntent{{ActorType: "system", ActorID: "temporal", Action: "release.workflow.complete", ResourceType: "release_run", ResourceID: run.ID, Result: "SUCCEEDED"}}, "release.succeeded")
}

func verificationProjection(result executor.VerificationResult, checkedAt time.Time) *domain.Verification {
	outcome := result.Outcome()
	return &domain.Verification{Status: domain.VerificationStatus(outcome), Healthy: outcome == adapter.VerificationPass, Reason: result.Reason, ObservedValue: result.ObservedValue, Threshold: result.Threshold, CheckedAt: checkedAt, Evidence: domain.Evidence{Source: result.Evidence.Source, QueryHash: result.Evidence.QueryHash, WindowFrom: result.Evidence.Window.From, WindowTo: result.Evidence.Window.To, ObservedAt: result.Evidence.ObservedAt, ObservedValue: result.Evidence.ObservedValue, Threshold: result.Evidence.Threshold, AdapterVersion: result.Evidence.AdapterVersion, EvidenceHash: result.Evidence.EvidenceHash}}
}

func observationDuration(value string) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return time.Second
	}
	return duration
}

func persist(ctx workflow.Context, run domain.ReleaseRun, audits []AuditIntent, eventType string) (domain.ReleaseRun, error) {
	var updated domain.ReleaseRun
	err := workflow.ExecuteActivity(ctx, ActivityPersistRun, PersistInput{Run: run, Audits: audits, EventType: eventType}).Get(ctx, &updated)
	return updated, err
}

func awaitApproval(activityContext, ctx workflow.Context, run domain.ReleaseRun, index int, decision domain.PolicyDecision, approvalChannel, pauseChannel, resumeChannel, cancelChannel workflow.ReceiveChannel) (domain.ReleaseRun, error) {
	now := workflow.Now(ctx).UTC()
	approval := &domain.Approval{ID: deterministicID("approval", run.ID, run.Steps[index].Service), RequiredRoles: append([]string(nil), decision.RequiredRoles...), Status: domain.ApprovalPending, PlanHash: run.Plan.Hash, RequestedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
	run.Steps[index].Approval = approval
	run.Steps[index].Status = domain.StepWaitingApproval
	run.Status = domain.RunWaitingApproval
	run.UpdatedAt = now
	var err error
	run, err = persist(activityContext, run, []AuditIntent{{ActorType: "system", ActorID: "policy-engine", Action: "approval.request", ResourceType: "approval", ResourceID: approval.ID, Result: "PENDING"}}, "approval.requested")
	if err != nil {
		return run, err
	}
	for {
		var signal ApprovalSignal
		var pause ControlSignal
		var cancel ControlSignal
		expired := false
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(approvalChannel, func(channel workflow.ReceiveChannel, _ bool) { channel.Receive(ctx, &signal) })
		selector.AddReceive(pauseChannel, func(channel workflow.ReceiveChannel, _ bool) { channel.Receive(ctx, &pause) })
		selector.AddReceive(cancelChannel, func(channel workflow.ReceiveChannel, _ bool) { channel.Receive(ctx, &cancel) })
		timer := workflow.NewTimer(ctx, run.Steps[index].Approval.ExpiresAt.Sub(workflow.Now(ctx)))
		selector.AddFuture(timer, func(workflow.Future) { expired = true })
		selector.Select(ctx)
		if cancel.Actor != "" {
			return cancelRun(activityContext, ctx, run, index, cancel.Actor)
		}
		if pause.Actor != "" {
			run, err = waitForResume(activityContext, ctx, run, resumeChannel, cancelChannel, index)
			if err != nil || run.Status == domain.RunCancelled {
				return run, err
			}
			continue
		}
		if expired {
			run.Steps[index].Approval.Status = domain.ApprovalExpired
			run.Steps[index].Status = domain.StepBlocked
			run.Status = domain.RunBlocked
			cancelDownstream(&run, index+1)
			run.UpdatedAt = workflow.Now(ctx).UTC()
			return persist(activityContext, run, nil, "approval.expired")
		}
		if signal.ApprovalID != run.Steps[index].Approval.ID || signal.Actor == "" ||
			(signal.Approve && signal.Actor == run.RequestedBy) ||
			(signal.Approve && !containsAll(signal.Roles, run.Steps[index].Approval.RequiredRoles)) {
			continue
		}
		decidedAt := workflow.Now(ctx).UTC()
		run.Steps[index].Approval.DecidedBy = signal.Actor
		run.Steps[index].Approval.DecidedAt = &decidedAt
		if !signal.Approve {
			run.Steps[index].Approval.Status = domain.ApprovalDenied
			run.Steps[index].Status = domain.StepBlocked
			run.Status = domain.RunBlocked
			cancelDownstream(&run, index+1)
			run.UpdatedAt = decidedAt
			return persist(activityContext, run, []AuditIntent{{ActorType: "user", ActorID: signal.Actor, Action: "approval.decide", ResourceType: "approval", ResourceID: signal.ApprovalID, Result: "DENIED"}}, "approval.denied")
		}
		run.Steps[index].Approval.Status = domain.ApprovalApproved
		run.Steps[index].Status = domain.StepPending
		run.Status = domain.RunRunning
		run.UpdatedAt = decidedAt
		return persist(activityContext, run, []AuditIntent{{ActorType: "user", ActorID: signal.Actor, Action: "approval.decide", ResourceType: "approval", ResourceID: signal.ApprovalID, Result: "APPROVED"}}, "approval.approved")
	}
}

func waitForResume(activityContext, ctx workflow.Context, run domain.ReleaseRun, resumeChannel, cancelChannel workflow.ReceiveChannel, index int) (domain.ReleaseRun, error) {
	previous := run.Status
	run.PausedFrom = previous
	run.Status = domain.RunPaused
	run.UpdatedAt = workflow.Now(ctx).UTC()
	var err error
	run, err = persist(activityContext, run, nil, "release.paused")
	if err != nil {
		return run, err
	}
	for {
		var resume, cancel ControlSignal
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(resumeChannel, func(c workflow.ReceiveChannel, _ bool) { c.Receive(ctx, &resume) })
		selector.AddReceive(cancelChannel, func(c workflow.ReceiveChannel, _ bool) { c.Receive(ctx, &cancel) })
		selector.Select(ctx)
		if cancel.Actor != "" {
			return cancelRun(activityContext, ctx, run, index, cancel.Actor)
		}
		if resume.Actor != "" {
			run.Status = run.PausedFrom
			if run.Status == "" {
				run.Status = domain.RunRunning
			}
			run.PausedFrom = ""
			run.UpdatedAt = workflow.Now(ctx).UTC()
			return persist(activityContext, run, nil, "release.resumed")
		}
	}
}
func cancelRun(activityContext, ctx workflow.Context, run domain.ReleaseRun, from int, actor string) (domain.ReleaseRun, error) {
	run.Status = domain.RunCancelled
	for i := from; i < len(run.Steps); i++ {
		if run.Steps[i].Status == domain.StepPending || run.Steps[i].Status == domain.StepWaitingApproval {
			run.Steps[i].Status = domain.StepCancelled
		}
	}
	run.UpdatedAt = workflow.Now(ctx).UTC()
	return persist(activityContext, run, []AuditIntent{{ActorType: "user", ActorID: actor, Action: "release.cancel", ResourceType: "release_run", ResourceID: run.ID, Result: "CANCELLED"}}, "release.cancelled")
}
func cancelDownstream(run *domain.ReleaseRun, from int) {
	for i := from; i < len(run.Steps); i++ {
		if run.Steps[i].Status == domain.StepPending {
			run.Steps[i].Status = domain.StepCancelled
		}
	}
}
func deterministicID(prefix string, parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%s_%x", prefix, hash[:12])
}
func containsAll(actual, required []string) bool {
	set := map[string]bool{}
	for _, value := range actual {
		set[value] = true
	}
	for _, value := range required {
		if !set[value] {
			return false
		}
	}
	return true
}
func unmetDependency(plan domain.ReleasePlan, service string, selected, succeeded map[string]bool) string {
	for _, phase := range plan.Phases {
		for _, step := range phase.Steps {
			if step.Service != service {
				continue
			}
			for _, dependency := range step.Dependencies {
				if dependency.Service != "" && dependency.Type.EnforcesRolloutOrder() && selected[dependency.Service] && !succeeded[dependency.Service] {
					return dependency.Service
				}
			}
		}
	}
	return ""
}
