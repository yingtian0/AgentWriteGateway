ALTER TABLE action_grants
    ADD COLUMN tenant_id text,
    ADD COLUMN runner_group text,
    ADD COLUMN issuer text,
    ADD COLUMN idempotency_key text,
    ADD COLUMN policy_hash text,
    ADD COLUMN consumed_at timestamptz;

CREATE UNIQUE INDEX action_grants_runner_nonce_idx
    ON action_grants (tenant_id, runner_group, nonce);

CREATE UNIQUE INDEX action_grants_runner_idempotency_idx
    ON action_grants (tenant_id, runner_group, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE runner_journal (
    tenant_id text NOT NULL,
    runner_group text NOT NULL,
    nonce text NOT NULL,
    grant_id text NOT NULL,
    run_id text NOT NULL,
    step_id text NOT NULL,
    idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    status text NOT NULL CHECK (status IN ('RESERVED', 'SUCCEEDED', 'UNKNOWN', 'REJECTED')),
    result jsonb NOT NULL DEFAULT '{}'::jsonb,
    state_version bigint NOT NULL CHECK (state_version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, runner_group, nonce),
    UNIQUE (tenant_id, runner_group, idempotency_key)
);

CREATE INDEX runner_journal_reconcile_idx
    ON runner_journal (tenant_id, runner_group, status, updated_at)
    WHERE status IN ('RESERVED', 'UNKNOWN');

CREATE TABLE policy_bundles (
    hash text PRIMARY KEY,
    version text NOT NULL,
    issuer text NOT NULL,
    payload jsonb NOT NULL,
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE TABLE trusted_subjects (
    issuer text NOT NULL,
    subject text NOT NULL,
    status text NOT NULL CHECK (status IN ('ACTIVE', 'REVOKED')),
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (issuer, subject)
);

CREATE TABLE agent_delegations (
    id text PRIMARY KEY,
    issuer text NOT NULL,
    user_subject text NOT NULL,
    agent_id text NOT NULL,
    payload jsonb NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
