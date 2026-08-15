# ADR-0003: OPA policy engine

- Status: Accepted
- Date: 2026-08-13

## Context

The prototype uses fixed Go rules. OSS v1.0 must support platform, environment, team, and service policy managed by different owners without embedding every policy release in the application binary. Policy decisions must remain deterministic, testable, explainable, versioned, and enforceable in the customer environment even when the managed Control Plane is compromised or disconnected.

Application-specific conditionals do not provide bundle distribution, policy tests, or consistent evaluation across the Control Plane and Runner. An LLM cannot provide reliable or auditable authorization.

## Decision

OPA/Rego is the standard policy engine.

The application exposes a typed, versioned canonical policy input and maps OPA output to stable decisions and reason codes. Rego-specific types do not leak into the domain or public API. Policy has four monotonic layers: platform mandatory, environment, team, and service. A lower layer may add constraints but cannot weaken the platform baseline.

Policy is evaluated twice:

1. the Control Plane evaluates it during planning and before grant creation for constraints and explanation;
2. the Runner evaluates the same canonical input immediately before an external write and is the final Policy Enforcement Point.

Policy Bundles carry a version, content hash, signature, compatibility metadata, and tests. Plans and Action Grants pin the bundle hash. The Runner includes a mandatory baseline and accepts only compatible, valid, non-expired bundles. Any Control Plane/Runner decision or hash mismatch stops the action with a structured reason.

## Alternatives considered

### Continue fixed Go policy

Rejected as the v1.0 policy system because it couples organizational policy to application releases and cannot support safe delegated policy ownership and signed bundle distribution.

### Evaluate policy only in the Control Plane

Rejected because a compromised Control Plane or altered request could bypass the customer's final local boundary.

### Evaluate policy only in the Runner

Rejected because planning, constraints, approvals, and user explanations require a decision before dispatch. The Runner evaluation is still final.

### LLM-based policy decisions

Rejected because they are nondeterministic, prompt-injectable, difficult to replay, and inappropriate for authorization.

## Consequences

- Policy input and output schemas become versioned compatibility surfaces.
- Bundle build, lint, unit tests, signing, staged rollout, pinning, revocation, and rollback require tooling.
- Shadow evaluation can observe a candidate policy, but only the enforced pinned version authorizes work.
- The Control Plane and Runner need consistent reason-code mapping and conformance tests.
- OPA adds a runtime dependency and policy-language learning cost while separating policy ownership from application code.

## Safety implications

Unavailable, invalid, expired, or mismatched policy fails closed for new writes. Mandatory policy is not overridable. A policy `ALLOW` is necessary but not sufficient: valid identity/delegation, workflow state, grant, approval when required, context, capability, idempotency reservation, and Runner target checks must also pass.
