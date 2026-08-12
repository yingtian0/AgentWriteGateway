# ADR-0002: Temporal workflow engine

- Status: Accepted
- Date: 2026-08-13

## Context

Release workflows wait for approvals, observation windows, external systems, retries, Runner reconnection, and compensation. These waits can last hours or days and must survive process failure. A synchronous in-memory engine cannot provide durable timers, signals, replay, or recovery. A bespoke database worker would require us to design and operate workflow history, leases, retries, timers, cancellation, and replay semantics while also building the product domain.

## Decision

Temporal is the standard durable execution engine for OSS v1.0.

Temporal history is authoritative for active workflow progression, durable timers, signals, retry scheduling, cancellation, and compensation sequencing. A Release Run workflow coordinates typed domain commands and Activities. Long-lived steps may use child workflows when history size, isolation, or independent lifecycle justifies them.

Workflow code must remain deterministic. It must not directly read wall-clock time, generate randomness, access the network or database, evaluate live policy, sign grants, or invoke adapters. Those operations occur in Activities or application/domain services, and their results are recorded in workflow history. Temporal versioning and replay tests are required for workflow changes.

PostgreSQL remains the application database and read model; it does not replace Temporal history as the authority for an active workflow. External CI/CD or runtime systems remain authoritative for their actual side effects and are reconciled through external IDs and idempotency keys.

## Alternatives considered

### PostgreSQL queue and custom worker

Rejected as the v1.0 standard. It gives direct control and fewer runtime components, but long waits, signals, compensation, replay, and worker recovery would require substantial custom distributed-systems machinery and a second workflow history model.

### Synchronous application engine

Rejected for production because process lifetime would own state and approval or observation waits could not be recovered safely.

### Cloud-provider-specific orchestrator

Rejected because it would couple the OSS core to one deployment environment and complicate customer-hosted and managed forms.

## Consequences

- Local development and deployment packaging must include a supported Temporal service.
- Workflow/Activity boundaries, retry classification, timeouts, payload size, and history growth require operational guidance.
- The domain layer stays independent of the Temporal SDK; adapters translate domain commands and results.
- UI and API queries use PostgreSQL projections linked to Temporal workflow IDs.
- Worker upgrades require deterministic replay tests and Temporal-compatible versioning.
- Temporal availability is operationally significant, but an outage pauses progression rather than authorizing an alternate write path.

## Safety implications

Retries apply only to classified idempotent Activities. A write timeout that leaves external state unknown is reconciled using the external execution ID or idempotency key before another attempt. Workflow replay must not repeat an external write. No fallback worker may act from a stale PostgreSQL projection while Temporal is unavailable.
