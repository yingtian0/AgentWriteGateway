# ADR-0001: Change Execution Control Plane

- Status: Accepted
- Date: 2026-08-13

## Context

The prototype is described as an AI Agent execution harness, but production changes can originate from people, CI, CLIs, or agents. Treating AI as the primary authority would duplicate safety rules across interfaces and tempt implementations to use model output as authorization, policy, or workflow state.

The product must coordinate changes across 50–500 services while preserving deterministic authorization, dependency ordering, idempotency, verification, rollback, and audit. It must complement existing service catalogs, CI/CD, runtime, observability, identity, policy, and workflow products rather than proxy their low-level APIs.

## Decision

We will build a Change Execution Control Plane with a shared structured-intent and domain-command path for every client.

The Control Plane owns context pinning, deterministic planning, identity and delegation verification, policy evaluation, approvals, workflow orchestration, and audit initiation. It emits a signed, short-lived Action Grant for one typed step. A separate Customer Environment Runner performs final grant and policy enforcement, obtains credentials locally, and invokes an allowlisted typed adapter.

AI is an optional interface for intent structuring, read-side investigation, explanation, and bounded tool calls. It is not:

- a policy engine;
- a credential holder;
- the source of truth for workflow state or evidence;
- permitted to construct arbitrary shell, HTTP, or cloud-provider actions.

Service differences are represented with versioned Service Contracts and a small set of Release Profiles, not bespoke workflows per service.

## Alternatives considered

### Agent-specific deploy service

Rejected because it creates a second authorization and execution path, makes non-agent use harder, and over-trusts model behavior.

### Generic workflow or cloud API proxy

Rejected because low-level primitives cannot enforce service-level meaning, rollback capability, dependency constraints, or a closed action allowlist.

### Documentation and agent skills as the safety layer

Rejected because instructions improve behavior but cannot technically prevent a compromised or mistaken caller from crossing a boundary.

## Consequences

- HTTP, CLI, UI, and MCP must converge on application use cases rather than adapters.
- Planning and execution are separate and bound by hashes, versions, and expiry.
- Identity, policy, workflow, Runner protocol, credentials, adapters, evidence, and audit remain explicit packages and review boundaries.
- Adding a new action requires typed contract, capability, policy, idempotency, evidence, and failure semantics.
- The system has more components than a direct deploy bot, but failures and authority become inspectable and testable.
- Managed features may add fleet operations and enterprise integrations, but core Runner enforcement and safety tests remain available in OSS.

## Safety implications

No interface, including AI, can bypass deterministic policy and workflow state. No arbitrary action surface may be introduced for convenience. Any future alternate execution path must satisfy the same invariants and requires a superseding ADR.
