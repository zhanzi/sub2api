-- Native image generation history and hidden page API keys.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS purpose TEXT NOT NULL DEFAULT 'user';

CREATE INDEX IF NOT EXISTS idx_api_keys_user_purpose
    ON api_keys(user_id, purpose)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS image_generation_tasks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    usage_log_id BIGINT NULL REFERENCES usage_logs(id) ON DELETE SET NULL,
    mode TEXT NOT NULL,
    status TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt TEXT NOT NULL DEFAULT '',
    request_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_image_generation_tasks_user_created
    ON image_generation_tasks(user_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_image_generation_tasks_expires
    ON image_generation_tasks(expires_at)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS image_generation_results (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES image_generation_tasks(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    result_index INTEGER NOT NULL DEFAULT 0,
    mime_type TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_image_generation_results_task
    ON image_generation_results(task_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_image_generation_results_user
    ON image_generation_results(user_id, created_at DESC)
    WHERE deleted_at IS NULL;

INSERT INTO settings (key, value, updated_at)
VALUES
    ('image_generation_enabled', 'false', NOW()),
    ('image_generation_default_group_id', '', NOW()),
    ('image_generation_default_model', 'gpt-image-2', NOW()),
    ('image_generation_retention_days', '30', NOW())
ON CONFLICT (key) DO NOTHING;
