DROP TABLE IF EXISTS agent_delegations;
DROP TABLE IF EXISTS trusted_subjects;
DROP TABLE IF EXISTS policy_bundles;
DROP TABLE IF EXISTS runner_journal;
DROP INDEX IF EXISTS action_grants_runner_idempotency_idx;
DROP INDEX IF EXISTS action_grants_runner_nonce_idx;
ALTER TABLE action_grants
    DROP COLUMN IF EXISTS consumed_at,
    DROP COLUMN IF EXISTS policy_hash,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS issuer,
    DROP COLUMN IF EXISTS runner_group,
    DROP COLUMN IF EXISTS tenant_id;
