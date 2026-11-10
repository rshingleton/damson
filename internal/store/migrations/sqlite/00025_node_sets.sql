-- +goose Up
-- Node sets (ADR-0078): a named CEL filter over a node, owned by a project and
-- referenced by access rules as "<project path>:<name>".
CREATE TABLE node_sets (
    id          TEXT        PRIMARY KEY,
    project_id  TEXT        NOT NULL REFERENCES projects (id),
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    filter      TEXT        NOT NULL,
    version     INTEGER      NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    UNIQUE (project_id, name)
);

-- +goose Down
DROP TABLE node_sets;
