CREATE TABLE release_runs (
    id text PRIMARY KEY,
    request_id text NOT NULL UNIQUE,
    workflow_id text NOT NULL UNIQUE,
    temporal_run_id text,
    release_version text NOT NULL,
    environment text NOT NULL,
    requested_by text NOT NULL,
    status text NOT NULL,
    state_version bigint NOT NULL CHECK (state_version > 0),
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX release_runs_status_updated_idx ON release_runs (status, updated_at DESC);

CREATE TABLE release_steps (
    run_id text NOT NULL REFERENCES release_runs(id) ON DELETE CASCADE,
    service text NOT NULL,
    environment text NOT NULL,
    phase integer NOT NULL CHECK (phase >= 0),
    status text NOT NULL,
    state_version bigint NOT NULL CHECK (state_version > 0),
    payload jsonb NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (run_id, service)
);

CREATE INDEX release_steps_status_idx ON release_steps (status, updated_at DESC);

CREATE TABLE policy_decisions (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES release_runs(id) ON DELETE CASCADE,
    service text NOT NULL,
    decision text NOT NULL,
    policy_version text NOT NULL,
    input_hash text NOT NULL,
    reason_code text NOT NULL,
    snapshot jsonb NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE approvals (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES release_runs(id) ON DELETE CASCADE,
    service text NOT NULL,
    status text NOT NULL,
    plan_hash text NOT NULL,
    required_roles jsonb NOT NULL,
    requested_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    decided_by text,
    decided_at timestamptz,
    state_version bigint NOT NULL CHECK (state_version > 0)
);

CREATE TABLE action_grants (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES release_runs(id) ON DELETE CASCADE,
    nonce text NOT NULL UNIQUE,
    status text NOT NULL,
    payload jsonb NOT NULL,
    state_version bigint NOT NULL CHECK (state_version > 0),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE TABLE executions (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES release_runs(id) ON DELETE CASCADE,
    service text NOT NULL,
    adapter text NOT NULL,
    idempotency_key text NOT NULL,
    status text NOT NULL,
    external_execution_id text,
    payload jsonb NOT NULL,
    state_version bigint NOT NULL CHECK (state_version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (adapter, idempotency_key)
);

CREATE TABLE evidence (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES release_runs(id) ON DELETE CASCADE,
    service text NOT NULL,
    kind text NOT NULL,
    verdict text NOT NULL CHECK (verdict IN ('PASS', 'FAIL', 'INCONCLUSIVE', 'MISSING')),
    reference text,
    payload jsonb NOT NULL,
    observed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE audit_events (
    sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id text NOT NULL UNIQUE,
    correlation_id text NOT NULL,
    actor_type text NOT NULL,
    actor_id text NOT NULL,
    delegated_by text,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    result text NOT NULL,
    details jsonb NOT NULL,
    occurred_at timestamptz NOT NULL
);

CREATE INDEX audit_events_correlation_idx ON audit_events (correlation_id, sequence);

CREATE FUNCTION reject_audit_mutation() RETURNS trigger
LANGUAGE plpgsql AS $function$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END
$function$;

CREATE TRIGGER audit_events_append_only
BEFORE UPDATE OR DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION reject_audit_mutation();

CREATE TRIGGER audit_events_no_truncate
BEFORE TRUNCATE ON audit_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

CREATE TABLE outbox_events (
    id text PRIMARY KEY,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL,
    published_at timestamptz,
    created_at timestamptz NOT NULL
);

CREATE INDEX outbox_events_pending_idx
    ON outbox_events (available_at, created_at)
    WHERE published_at IS NULL;

CREATE TABLE service_leases (
    service text NOT NULL,
    environment text NOT NULL,
    holder_run_id text NOT NULL REFERENCES release_runs(id) ON DELETE CASCADE,
    lease_version bigint NOT NULL CHECK (lease_version > 0),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (service, environment)
);
