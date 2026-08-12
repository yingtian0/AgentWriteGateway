# ADR-0005: PostgreSQL and audit state

- Status: Accepted
- Date: 2026-08-13

## Context

The prototype stores runs and audit events in process memory. OSS v1.0 needs durable contracts, plans, approvals, executions, evidence, idempotency constraints, outbox delivery, query indexes, and optimistic concurrency. It must also explain decisions after a process restart without confusing UI projections, workflow history, and external provider state.

Using a single mutable row model as the authority for every concern would allow projection bugs or administrative edits to affect execution. Conversely, adopting multiple unrelated databases in the initial core would increase operational and transactional complexity before evidence justifies it.

## Decision

PostgreSQL is the standard relational store for durable application state and query projections. Versioned migrations define contracts, profiles, plans, run/step indexes, policy decisions, approvals, grants, executions, evidence metadata, audit events, idempotency records, leases, and transactional outbox records as each packet introduces them.

Authority is deliberately split:

- Temporal history is authoritative for active workflow progression, signals, timers, and retry/compensation sequencing.
- PostgreSQL is authoritative for durable application records, uniqueness constraints, indexes, and rebuildable API/UI projections.
- The append-only audit history is authoritative for decisions, evidence references, and action attribution.
- External systems are authoritative for their actual deployed state and side effects.

The initial OSS implementation may keep append-only audit events in PostgreSQL behind a dedicated audit interface, provided application roles cannot update or delete events and events can be exported to an independent retention store. Audit event insertion or an outbox record is committed atomically with the application transition that permits grant issuance. A projection is never sufficient authorization for a write.

Required constraints include stable tenant scoping when tenancy is introduced, optimistic state versions, unique external event IDs, unique grant nonces, and `(tenant, adapter, idempotency_key)` uniqueness. SQL rows and driver types do not leak into domain packages.

## Alternatives considered

### Continue with in-memory storage

Retained only as a test implementation. It cannot survive restart or enforce production uniqueness and atomicity.

### Use Temporal as the only database

Rejected because contracts, search, approvals, public API projections, audit retention, relational constraints, and migration needs do not all belong in workflow history.

### Use PostgreSQL as a custom workflow engine

Rejected by ADR-0002. PostgreSQL supports application records and projections but does not replace Temporal's workflow authority.

### Introduce a dedicated event/audit database immediately

Deferred. It may provide stronger isolation and retention at scale, but PostgreSQL plus a strict append-only interface and export path gives the initial OSS release a smaller operational footprint. Moving audit retention requires a new ADR and must preserve completeness and ordering.

## Consequences

- Every schema change uses ordered up/down migrations and compatibility review.
- Repository interfaces separate domain models from SQL and support contract tests against memory and PostgreSQL implementations.
- Transactions, row-level access, roles, backup, restore, retention, projection rebuild, and migration operations require runbooks.
- Workflow-to-projection delivery uses an outbox or an equivalently atomic recorded handoff; consumers are idempotent.
- Audit payloads require redaction, stable event schemas, integrity protection, retention controls, and export.
- PostgreSQL unavailability pauses state transitions that would authorize new writes; it is not bypassed with in-memory state.

## Safety implications

A new external write cannot start unless its authorization-relevant transition, idempotency reservation, and required audit/outbox intent are durable. Duplicate events and commands are rejected or return the prior result through database constraints. Administrative projection repair must not fabricate authorization, grant consumption, or external success. Recovery reconciles Temporal history, PostgreSQL records, the Runner journal, and external provider state rather than declaring one mutable table universally correct.
