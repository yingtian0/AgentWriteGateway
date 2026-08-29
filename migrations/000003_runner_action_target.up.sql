ALTER TABLE runner_journal
    ADD COLUMN target jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN action jsonb NOT NULL DEFAULT '{}'::jsonb;
