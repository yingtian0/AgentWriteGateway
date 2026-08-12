# Agent Write Gateway

> **Prototype / not production ready.** The current implementation uses an in-memory store and a mock adapter. It must not be connected to production credentials or production write APIs.

Agent Write Gateway is an open-source Change Execution Control Plane for planning, authorizing, executing, verifying, stopping, and auditing changes requested by people, CI, CLIs, or AI agents through the same deterministic safety boundary.

The current repository is an executable prototype. It performs no external writes and demonstrates safety properties with a 20-service catalog, an in-memory store, and a mock deploy/verify/rollback adapter.

## What the prototype demonstrates

- dependency validation, cycle detection, and deterministic release phases
- plan hashes that do not depend on input order
- separate requester and delegated-agent identities
- fail-closed policy checks for agent scope, CI, and dependency health
- `ALLOW`, `DENY`, and `REQUIRE_APPROVAL` decisions
- role checks, separation of duties, approval expiry, and plan-hash binding
- request idempotency and execution idempotency keys
- separate deploy and verify stages, rollback on verification failure, and downstream stopping
- audit records for subjects, policy inputs, decisions, and operations
- optimistic state versions and a deliberately limited HTTP API

## Safety boundary

Values such as agent scopes, approver roles, CI status, dependency health, and service metadata are demo inputs today. They are not trusted production identity or evidence.

| Prototype input | Required production source |
|---|---|
| Agent scopes | verified short-lived delegation from an identity provider |
| Approver roles | identity provider or organization directory |
| `ci_success` | typed read adapter for the CI system |
| `dependencies_healthy` | typed runtime or observability evidence |
| Service metadata | versioned Service Contracts and trusted dynamic sources |

The target architecture separates a Control Plane from a Customer Environment Runner. Production credentials remain in the customer environment. The Runner accepts only signed, narrowly scoped Action Grants, re-evaluates pinned policy locally, obtains short-lived credentials, and invokes typed adapters. Arbitrary shell, arbitrary HTTP, and generic cloud APIs are not exposed.

See the [architecture overview](docs/architecture/overview.md), [threat model](docs/security/threat-model.md), and [architecture decisions](docs/adr/README.md) before extending an execution path.

## Run the prototype

Go 1.26 or later is required. The prototype has no external module dependencies.

```bash
make test
make run
```

In another terminal, create a plan:

```bash
curl -sS -X POST http://localhost:8080/v1/release-runs:plan \
  -H 'Content-Type: application/json' \
  --data-binary @examples/release.json
```

Start the same request. The default adapter is a mock and performs no external write:

```bash
curl -sS -X POST http://localhost:8080/v1/release-runs \
  -H 'Content-Type: application/json' \
  --data-binary @examples/release.json
```

## API

```text
GET  /healthz
GET  /v1/services
POST /v1/release-runs:plan
POST /v1/release-runs
GET  /v1/release-runs/{id}
GET  /v1/release-runs/{id}/events
POST /v1/release-runs/{id}/cancel
POST /v1/release-runs/{id}/approvals/{approvalID}/approve
POST /v1/release-runs/{id}/approvals/{approvalID}/deny
```

The prototype approval body is:

```json
{
  "actor": "sre-user-1",
  "roles": ["service-owner", "sre"]
}
```

`roles` is a prototype-only input. A production implementation must resolve roles from the authenticated subject on the server.

## Roadmap

Development is split into bounded Task Packets. Each packet must preserve the safety invariants and must be completed and verified before work begins on the next one.

| Packet | Outcome |
|---:|---|
| 00 | architecture baseline, threat model, OSS governance, and CI |
| 01 | Service Contract, Release Profile, and typed Plan v2 |
| 02 | PostgreSQL durable state and Temporal workflows |
| 03 | Customer Runner, signed Action Grants, identity, and OPA |
| 04 | production-quality deploy adapter, evidence, verification, and rollback |
| 05 | REST/CLI/MCP/UI interfaces, concurrency budgets, and scale validation |
| 06 | packaging, security hardening, release engineering, and OSS v1.0 |

The target public OSS v1.0 date is 2027-03-01. Dates are planning targets, not compatibility or support commitments. See the [implementation plan](docs/implementation-plan.md) for milestones and packet rules.

## Package map

```text
cmd/gateway       HTTP server composition root
internal/api      deliberately limited external API
internal/catalog  prototype service metadata loader
internal/domain   run, step, approval, and audit model
internal/planner  dependency graph and plan hashing
internal/policy   deterministic prototype policy engine
internal/engine   prototype state transitions and orchestration
internal/executor typed adapter interface and mock implementation
internal/store    store interface and in-memory implementation
```

## Contributing and security

Read [CONTRIBUTING.md](CONTRIBUTING.md), [GOVERNANCE.md](GOVERNANCE.md), and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before contributing. Report suspected vulnerabilities privately as described in [SECURITY.md](SECURITY.md); do not open a public issue for a vulnerability.

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
