package workflow

import (
	"context"
	"errors"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

type Execution struct {
	WorkflowID string
	RunID      string
}

type Controller struct {
	client    client.Client
	taskQueue string
}

func NewController(temporalClient client.Client, taskQueue string) *Controller {
	if taskQueue == "" {
		taskQueue = DefaultTaskQueue
	}
	return &Controller{client: temporalClient, taskQueue: taskQueue}
}

func (c *Controller) StartRelease(ctx context.Context, input ReleaseInput) (Execution, error) {
	handle, err := c.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: input.Run.WorkflowID, TaskQueue: c.taskQueue,
		WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowExecutionErrorWhenAlreadyStarted: false}, ReleaseWorkflow, input)
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			return Execution{WorkflowID: input.Run.WorkflowID, RunID: alreadyStarted.RunId}, nil
		}
		return Execution{}, err
	}
	return Execution{WorkflowID: handle.GetID(), RunID: handle.GetRunID()}, nil
}

func (c *Controller) SignalApproval(ctx context.Context, workflowID string, signal ApprovalSignal) error {
	return c.client.SignalWorkflow(ctx, workflowID, "", SignalApproval, signal)
}
func (c *Controller) Pause(ctx context.Context, workflowID string, signal ControlSignal) error {
	return c.client.SignalWorkflow(ctx, workflowID, "", SignalPause, signal)
}
func (c *Controller) Resume(ctx context.Context, workflowID string, signal ControlSignal) error {
	return c.client.SignalWorkflow(ctx, workflowID, "", SignalResume, signal)
}
func (c *Controller) Cancel(ctx context.Context, workflowID string, signal ControlSignal) error {
	return c.client.SignalWorkflow(ctx, workflowID, "", SignalCancel, signal)
}
