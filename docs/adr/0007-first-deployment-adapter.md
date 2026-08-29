# ADR-0007: Use GitHub Actions as the first deployment adapter

- Status: Accepted
- Date: 2026-08-29

## Context

Packet 04 requires one production-quality deployment path, selected from GitHub Actions and AWS ECS. The development environment has an authenticated GitHub account and a repository with a `workflow_dispatch` workflow. It has no usable AWS execution identity or dedicated ECS staging service. Implementing both providers would weaken the typed boundary and leave neither path adequately verified.

GitHub workflow dispatch accepts a configured repository, workflow file, ref, and declared inputs. Those fields are dangerous if copied from agent output because they can select an arbitrary workflow or ref. A timeout can also leave the dispatch result unknown, so blindly repeating the POST can start a second deployment.

## Decision

The first deployment adapter is GitHub Actions. Runner-owned configuration maps each `(service, environment)` pair to one repository, deploy workflow, rollback workflow, and ref. Action Grants contain only a typed service, environment, artifact digest, capability, and idempotency key. They cannot supply a repository, workflow filename, ref, URL, command, or arbitrary input map.

The adapter uses the versioned GitHub REST workflow-dispatch endpoint and requires the returned workflow run ID as the external execution ID. It sends a fixed set of `awg_*` inputs. Target workflows must declare those inputs and set `run-name` to `awg:${{ inputs.awg_idempotency_key }}`. Reconciliation lists configured workflow runs and matches that exact correlation title. A timeout or ambiguous provider response becomes `UNKNOWN_EXTERNAL_STATE`; the Runner journal prevents another write until reconciliation resolves it.

GitHub credentials are obtained only inside the customer Runner from a customer-managed short-lived token file. The broker binds issuance to an allow-listed tenant, service, environment, and purpose. Credentials never enter the Action Grant, workflow state, protocol result, audit, or evidence.

Datadog is the initial verification source. Metrics expressions are selected from Runner-owned configuration, not from an agent request. Evidence records only the query hash and bounded numeric result, not the raw query, credentials, or log text.

The Packet requested `0006-first-deployment-adapter.md`, but ADR-0006 already defines Action Grant signatures. This decision uses the next available immutable sequence number, ADR-0007.

## Alternatives considered

### AWS ECS

Rejected for the first adapter because no AWS execution identity or dedicated staging cluster is available in the implementation environment. Adding an unexercised ECS client would not satisfy the requirement to choose the path with the strongest available end-to-end validation.

### Allow the Grant to name any repository, workflow, or ref

Rejected because it turns the adapter into a generic GitHub Actions proxy and lets a compromised agent select a more privileged workflow.

### Retry workflow dispatch after a timeout

Rejected because GitHub workflow dispatch does not provide a caller-supplied provider idempotency primitive. The fixed correlation input and exact run-name reconciliation are required before any new decision.

## Consequences

- Every enabled service and environment requires a local target mapping and fixed metrics query.
- Deployment workflows must implement the documented fixed inputs and correlation run name.
- Rollback is a distinct workflow dispatch with its own external execution ID and durable execution record.
- GitHub and Datadog secret-file rotation remains a deployment responsibility.
- A live deploy/failure/rollback staging run was not performed because no dedicated deployment workflow/account was designated. Production readiness remains unproven until that test succeeds.

## Safety implications

The adapter exposes no arbitrary command, URL, workflow input, or cloud payload. Provider ambiguity stops new writes and requires reconciliation. Missing or inconclusive Datadog evidence cannot become PASS. A failed or unverified rollback stops downstream work and escalates the release run.
