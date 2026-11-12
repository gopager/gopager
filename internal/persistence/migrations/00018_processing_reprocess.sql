-- +goose Up
-- One explicit historical Reprocess of a Workflow over a Message-time range. The row is the
-- complete durable state of the run: the pins (Workflow revision and active version) that fence
-- it, the scan cutoff on the primary clock, the cursor of the last scanned Transmission and the
-- worker lease, so a restarted runner resumes without any other memory.
CREATE TABLE processing_reprocess_requests (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    workflow_id uuid NOT NULL REFERENCES processing_workflows(id),
    definition_version_id uuid NOT NULL,
    workflow_revision bigint NOT NULL CHECK (workflow_revision > 0),
    message_time_from timestamptz NOT NULL,
    message_time_to timestamptz NOT NULL,
    cutoff_at timestamptz NOT NULL,
    cursor uuid,
    state varchar NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'running', 'completed', 'superseded')),
    scanned bigint NOT NULL DEFAULT 0 CHECK (scanned >= 0),
    readmitted bigint NOT NULL DEFAULT 0 CHECK (readmitted >= 0),
    retracted bigint NOT NULL DEFAULT 0 CHECK (retracted >= 0),
    requested_by uuid NOT NULL,
    due_at timestamptz NOT NULL,
    worker_id uuid,
    lease_until timestamptz,
    failure_code varchar,
    -- Consecutive failed steps, which set the retry backoff; a successful step resets it.
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    FOREIGN KEY (workflow_id, definition_version_id) REFERENCES processing_workflow_versions(workflow_id, id),
    CHECK (message_time_from < message_time_to),
    CHECK ((worker_id IS NULL) = (lease_until IS NULL))
);
CREATE INDEX processing_reprocess_due ON processing_reprocess_requests (state, due_at, id);
CREATE INDEX processing_reprocess_workflow ON processing_reprocess_requests (workflow_id, created_at DESC, id DESC);
-- A candidate carrying the mark was readmitted by a Reprocess and replaces the earlier published
-- results of its Transmission and Workflow when it publishes.
ALTER TABLE processing_candidates ADD CONSTRAINT processing_candidates_reprocess
    FOREIGN KEY (reprocess_id) REFERENCES processing_reprocess_requests(id);
CREATE INDEX processing_candidates_reprocess ON processing_candidates (reprocess_id) WHERE reprocess_id IS NOT NULL;

-- +goose Down
DROP INDEX processing_candidates_reprocess;
ALTER TABLE processing_candidates DROP CONSTRAINT processing_candidates_reprocess;
DROP TABLE processing_reprocess_requests;
