CREATE TABLE IF NOT EXISTS jobs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id     TEXT NOT NULL UNIQUE,
    task_type  TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'queued',
    payload    JSONB NOT NULL,
    cluster_id UUID REFERENCES clusters(id) ON DELETE SET NULL,
    deployment_id UUID REFERENCES deployments(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error      TEXT
);
