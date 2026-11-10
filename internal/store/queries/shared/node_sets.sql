-- name: InsertNodeSet :exec
INSERT INTO node_sets (id, project_id, name, description, filter, version, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(filter), 1, sqlc.arg(now), sqlc.arg(now));

-- name: GetNodeSetByName :one
SELECT id, project_id, name, description, filter, version, created_at, updated_at
FROM node_sets WHERE project_id = sqlc.arg(project_id) AND name = sqlc.arg(name);

-- name: GetNodeSet :one
SELECT id, project_id, name, description, filter, version, created_at, updated_at
FROM node_sets WHERE id = sqlc.arg(id);

-- name: ListNodeSetsByProject :many
SELECT id, project_id, name, description, filter, version, created_at, updated_at
FROM node_sets WHERE project_id = sqlc.arg(project_id) ORDER BY name;

-- name: CountNodeSetsByProject :one
SELECT count(*) FROM node_sets WHERE project_id = sqlc.arg(project_id);

-- name: UpdateNodeSetCAS :execrows
UPDATE node_sets SET description = sqlc.arg(description), filter = sqlc.arg(filter), version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND version = sqlc.arg(expected_version);

-- name: DeleteNodeSetCAS :execrows
DELETE FROM node_sets WHERE id = sqlc.arg(id) AND version = sqlc.arg(expected_version);

-- name: SetRunNodeSets :exec
INSERT INTO run_node_sets (run_id, sets) VALUES (sqlc.arg(run_id), sqlc.arg(sets));

-- name: GetRunNodeSets :one
SELECT sets FROM run_node_sets WHERE run_id = sqlc.arg(run_id);

-- name: SetScheduleNodeSets :exec
INSERT INTO schedule_node_sets (schedule_id, sets) VALUES (sqlc.arg(schedule_id), sqlc.arg(sets))
ON CONFLICT (schedule_id) DO UPDATE SET sets = excluded.sets;

-- name: ClearScheduleNodeSets :exec
DELETE FROM schedule_node_sets WHERE schedule_id = sqlc.arg(schedule_id);

-- name: GetScheduleNodeSets :one
SELECT sets FROM schedule_node_sets WHERE schedule_id = sqlc.arg(schedule_id);

-- name: SetWebhookNodeSets :exec
INSERT INTO webhook_node_sets (webhook_id, sets) VALUES (sqlc.arg(webhook_id), sqlc.arg(sets));

-- name: GetWebhookNodeSets :one
SELECT sets FROM webhook_node_sets WHERE webhook_id = sqlc.arg(webhook_id);

-- name: ListScheduleNodeSetTexts :many
SELECT sets FROM schedule_node_sets;

-- name: ListWebhookNodeSetTexts :many
SELECT sets FROM webhook_node_sets;
