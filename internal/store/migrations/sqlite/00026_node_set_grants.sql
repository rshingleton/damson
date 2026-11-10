-- +goose Up
-- The node sets a run, a schedule or a webhook may use (ADR-0078): a JSON array
-- of set references. No row means every node within the project's node scope.
-- Side tables keep the run, schedule and webhook queries as they are.
CREATE TABLE run_node_sets (
    run_id TEXT PRIMARY KEY REFERENCES runs (id) ON DELETE CASCADE,
    sets   TEXT NOT NULL
);

CREATE TABLE schedule_node_sets (
    schedule_id TEXT PRIMARY KEY REFERENCES schedules (id) ON DELETE CASCADE,
    sets        TEXT NOT NULL
);

CREATE TABLE webhook_node_sets (
    webhook_id TEXT PRIMARY KEY REFERENCES webhooks (id) ON DELETE CASCADE,
    sets       TEXT NOT NULL
);

-- +goose Down
DROP TABLE webhook_node_sets;
DROP TABLE schedule_node_sets;
DROP TABLE run_node_sets;
