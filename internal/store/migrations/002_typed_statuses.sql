-- Migration 002: Add CHECK constraints to enforce valid status values.
-- Also adds new statuses: 'destroying' for clusters, 'building'/'deploying' for deployments.

ALTER TABLE clusters
    ADD CONSTRAINT chk_cluster_status
    CHECK (status IN ('provisioning', 'installing', 'active', 'failed', 'destroying', 'destroyed'));

ALTER TABLE deployments
    ADD CONSTRAINT chk_deployment_status
    CHECK (status IN ('pending', 'building', 'deploying', 'active', 'failed', 'deleting', 'deleted', 'orphaned'));

ALTER TABLE jobs
    ADD CONSTRAINT chk_job_status
    CHECK (status IN ('queued', 'running', 'completed', 'failed'));
