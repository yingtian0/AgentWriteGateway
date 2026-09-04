CREATE TABLE release_plans (
    id text PRIMARY KEY,
    plan_hash text NOT NULL,
    environment text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE INDEX release_plans_expiry_idx ON release_plans (expires_at);
