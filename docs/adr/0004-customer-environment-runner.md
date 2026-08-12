# ADR-0004: Customer Environment Runner

- Status: Accepted
- Date: 2026-08-13

## Context

A managed Control Plane needs to coordinate production changes without storing customer production credentials or requiring inbound access to customer networks. Sending direct cloud or CI/CD API calls from SaaS would enlarge the credential and network blast radius. Relying only on a SaaS authorization decision would also let a Control Plane compromise become a production compromise.

## Decision

Production external writes execute through a Customer Environment Runner deployed in the customer's VPC, cluster, or equivalent security domain.

The Runner connects outbound over an authenticated encrypted channel using workload identity. It accepts only versioned, signed Action Grants scoped to one typed step. Before obtaining credentials or invoking an adapter, it verifies issuer, audience, tenant, target, subject proof, plan/contract/policy/evidence hashes, approvals, nonce, expiry, current journal state, and local capability allowlist, then obtains final local OPA `ALLOW`.

The Runner obtains short-lived, least-privileged credentials through a local Credential Broker. Credential material never appears in a grant and never returns to the Control Plane or agent. Adapter requests are typed and allowlisted; arbitrary shell, arbitrary HTTP, generic cloud APIs, and agent-accessible break-glass credentials are prohibited.

The Runner keeps a durable local idempotency and execution journal, records nonce consumption before credential acquisition, persists external execution IDs, and signs normalized results and evidence. When disconnected, it starts no new production write. It may reconcile an in-flight action or perform a previously authorized compensation only when the grant, local policy, and journal permit it.

## Alternatives considered

### Store credentials and execute from the managed Control Plane

Rejected because it centralizes customer secrets, requires broad network reach, and turns a SaaS compromise into direct production access.

### Customer exposes inbound deployment endpoints

Rejected because it increases network attack surface and complicates firewall, tenancy, and authentication controls.

### Runner trusts any signed SaaS request

Rejected because signature proves origin, not local authorization or target safety. Local policy, customer proof, allowlists, idempotency, and current state remain required.

### Install an autonomous agent with generic tools

Rejected because model behavior and generic credentials cannot enforce a closed, auditable action boundary.

## Consequences

- Customers operate or host Runner instances and configure identity, egress, policy, capabilities, credentials, journal persistence, upgrades, and monitoring.
- The protocol needs version negotiation, heartbeats, capacity, dispatch, acknowledgement, result/evidence signing, reconnection, and revocation semantics.
- Runner and adapter artifacts require hardened, signed, staged release and update processes.
- Availability is split: Control Plane loss pauses new writes while local reconciliation and narrowly pre-authorized compensation may continue.
- A compromised Runner remains a serious risk; provider-side least privilege, workload isolation, short credential lifetimes, and audit reduce its blast radius.

## Safety implications

APIs must make verification order difficult to bypass. No adapter invocation occurs before grant validation, nonce reservation, local policy, and idempotency journal persistence. Tenant, Runner group, audience, capability, hash, or policy mismatch is a terminal rejection, not a warning. Human break-glass remains a separate path unavailable to agents, SaaS, and the normal Runner process.
