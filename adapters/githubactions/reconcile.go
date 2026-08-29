package githubactions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"agentwritegateway/pkg/adapter"
	"agentwritegateway/pkg/credentials"
)

func (a *Adapter) Reconcile(ctx context.Context, request adapter.ReconcileRequest, credential credentials.Credential) (adapter.ReconcileResult, error) {
	if request.IdempotencyKey == "" || request.DispatchedAt.IsZero() {
		return adapter.ReconcileResult{}, &adapter.Error{Class: adapter.ErrorTerminal, Operation: "reconcile.validate", Err: errors.New("idempotency key and dispatch time are required")}
	}
	if !credential.ValidAt(a.now()) || len(credential.Value("token")) == 0 {
		return adapter.ReconcileResult{}, &adapter.Error{Class: adapter.ErrorTerminal, Operation: "reconcile.credential", Err: errors.New("valid GitHub credential required")}
	}
	correlation := adapter.CorrelationID(request.IdempotencyKey)
	keys := make([]TargetKey, 0, len(a.config.Targets))
	for key := range a.config.Targets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Service != keys[j].Service {
			return keys[i].Service < keys[j].Service
		}
		return keys[i].Environment < keys[j].Environment
	})
	seen := map[string]struct{}{}
	for _, key := range keys {
		target := a.config.Targets[key]
		for _, workflow := range []string{target.DeployWorkflow, target.RollbackWorkflow} {
			identity := target.Owner + "\x00" + target.Repository + "\x00" + workflow
			if _, ok := seen[identity]; ok {
				continue
			}
			seen[identity] = struct{}{}
			result, found, err := a.findRun(ctx, target, workflow, request.DispatchedAt, correlation, credential)
			if err != nil {
				return adapter.ReconcileResult{}, err
			}
			if found {
				return result, nil
			}
		}
	}
	return adapter.ReconcileResult{Status: adapter.ReconcileNotFound}, nil
}

func (a *Adapter) findRun(ctx context.Context, target Target, workflow string, dispatchedAt time.Time, correlation string, credential credentials.Credential) (adapter.ReconcileResult, bool, error) {
	query := url.Values{}
	query.Set("event", "workflow_dispatch")
	query.Set("per_page", "100")
	query.Set("created", ">="+dispatchedAt.UTC().Add(-time.Minute).Format(time.RFC3339))
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?%s", apiBaseURL, url.PathEscape(target.Owner), url.PathEscape(target.Repository), url.PathEscape(workflow), query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return adapter.ReconcileResult{}, false, err
	}
	authorize(req, credential)
	response, err := a.client.Do(req)
	if err != nil {
		return adapter.ReconcileResult{}, false, &adapter.Error{Class: adapter.ErrorRetryable, Operation: "reconcile.list", CorrelationID: correlation, Err: errors.New("GitHub workflow runs unavailable")}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		class := adapter.ErrorTerminal
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			class = adapter.ErrorRetryable
		}
		return adapter.ReconcileResult{}, false, &adapter.Error{Class: class, Operation: "reconcile.list", CorrelationID: correlation, Err: fmt.Errorf("GitHub returned HTTP %d", response.StatusCode)}
	}
	var payload struct {
		WorkflowRuns []struct {
			ID           int64     `json:"id"`
			DisplayTitle string    `json:"display_title"`
			Status       string    `json:"status"`
			Conclusion   string    `json:"conclusion"`
			CreatedAt    time.Time `json:"created_at"`
			UpdatedAt    time.Time `json:"updated_at"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return adapter.ReconcileResult{}, false, &adapter.Error{Class: adapter.ErrorRetryable, Operation: "reconcile.decode", CorrelationID: correlation, Err: errors.New("invalid GitHub workflow run response")}
	}
	for _, run := range payload.WorkflowRuns {
		if run.ID > 0 && validRunTitle(run.DisplayTitle, correlation) {
			return adapter.ReconcileResult{Status: mapRunStatus(run.Status, run.Conclusion), Deployment: adapter.Deployment{ExternalExecutionID: strconv.FormatInt(run.ID, 10), CorrelationID: correlation, StartedAt: run.CreatedAt.UTC(), FinishedAt: run.UpdatedAt.UTC()}}, true, nil
		}
	}
	return adapter.ReconcileResult{}, false, nil
}
