# Governance

Agent Write Gateway uses a maintainer-led, consensus-seeking governance model. The goal is to keep safety boundaries reviewable while allowing contributors to earn responsibility through sustained work.

## Roles

### Contributors

Anyone who reports issues, improves documentation, proposes designs, writes tests, or contributes code is a contributor.

### Reviewers

Reviewers are contributors trusted to review a defined area. They may approve changes but do not merge safety-critical changes alone. Maintainers appoint or remove reviewers in a public governance issue based on demonstrated judgment and continued participation.

### Maintainers

Maintainers steward releases, the roadmap, repository access, security response, governance, and final merge decisions. New maintainers are nominated by an existing maintainer and accepted by consensus of active maintainers after a public comment period of at least seven days.

An active maintainer has participated in review, release, issue triage, design, or security work during the preceding 90 days. A maintainer may move to emeritus status voluntarily or by consensus after sustained inactivity. Access may be removed immediately when account or project security requires it, followed by a documented review that omits sensitive details.

## Decision process

Routine changes are decided through pull-request review. The author seeks consensus and resolves objections with tests, evidence, or a narrower design.

The following require an ADR and at least two maintainer approvals when two active maintainers are available:

- Trust Boundary or source-of-truth changes;
- public API, Service Contract, Adapter SDK, or Runner protocol changes;
- durable state ownership or migration model changes;
- identity, policy, credential, signing, grant, audit, or replay-protection changes;
- removal or weakening of a safety invariant;
- release or licensing policy changes.

Consensus means no unresolved, reasoned objection from an active maintainer. If consensus cannot be reached after the proposal has had at least seven days of review, maintainers record the alternatives and may decide by a two-thirds vote of non-conflicted active maintainers. A tie means the proposal is not accepted. Security response may use a private, expedited decision and publish a non-sensitive record after disclosure is safe.

## Compatibility and releases

Before v1.0, APIs marked `v1alpha1` may evolve, but changes require migration notes and compatibility assessment. Releases are approved by a maintainer who did not author the final release-only change when enough maintainers are available. Release artifacts, provenance, and signing policy will be finalized before the release-candidate milestone.

## Conflicts of interest

Reviewers disclose employment, financial, or personal conflicts that could reasonably affect a decision and recuse themselves when appropriate. A recused person does not count toward approval or voting thresholds.

## Changes to governance

Changes to this document require an ADR or governance proposal, a public comment period of at least fourteen days, and approval by two-thirds of non-conflicted active maintainers.
