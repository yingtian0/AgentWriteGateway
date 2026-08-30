ALTER TABLE runner_journal
    DROP COLUMN IF EXISTS action,
    DROP COLUMN IF EXISTS target;
