# ADR-0006: Action Grant and identity signature profile

- Status: Accepted
- Date: 2026-08-22

## Context

Action Grants cross the Control Plane/Runner trust boundary and bind every authorization-relevant field. Policy Bundles also cross that boundary. OIDC proofs originate at a separate identity authority. The protocol needs deterministic verification without placing a production private key, production credential, or attacker-selected algorithm fallback in the Runner.

## Decision

Action Grants use the versioned `awg.protocol/v1alpha1` JSON structure and a detached Ed25519 signature over its canonical Go-struct JSON representation. The signature object is excluded from the signed value; every other field is included. The algorithm and key ID are explicit, and the Runner accepts only its configured issuer, audience, algorithm, and trust key. Unknown JSON fields and unknown protocol versions fail closed.

Policy Bundles use the same Ed25519 signature boundary. Their content hash is calculated over normalized metadata and modules, then the hash and all metadata are signed. Modules are sorted by layer and name. The production signer is a KMS/HSM interface that receives canonical bytes and returns a signature. An ephemeral self-signed Ed25519 implementation exists only behind an explicitly named development constructor and is never a production default.

OIDC user proofs use compact JWS. The initial verifier accepts only EdDSA or RS256 keys from a configured trusted resolver and requires issuer, audience, subject, signature, and expiry. Claims are not returned to policy or authorization code until signature verification succeeds. Discovery, JWKS refresh, rotation, and revocation are deployment integrations behind the resolver interface.

## Alternatives considered

### Store a private signing key in configuration

Rejected because configuration files, containers, and the Control Plane database are inappropriate production key custody boundaries.

### Allow the token or grant to select any advertised algorithm

Rejected because algorithm agility without a configured allowlist creates downgrade and algorithm-confusion risk.

### Sign non-canonical maps

Rejected because map construction and alternate encodings create ambiguous authorization payloads and brittle cross-process verification.

## Consequences

- Protocol evolution requires a new explicit protocol version and conformance tests.
- Production deployments must provide KMS-backed signing and independently configured Runner trust roots.
- OIDC deployments must implement key refresh and revocation without making key-source failure an allow condition.
- Canonicalization tests and field-mutation tests are security release gates.

## Safety implications

A missing, expired, modified, incorrectly addressed, or unverifiable Grant starts no credential acquisition and no adapter write. Signature validity proves origin and integrity only; identity, delegation, pinned hashes, approval, capability, local OPA, connectivity, and durable nonce reservation remain mandatory independent checks.
