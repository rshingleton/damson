package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/denyszorinets/damson/internal/store"
	"github.com/denyszorinets/damson/internal/store/postgres/gen"
)

// The node sets of runs, schedules and webhooks (ADR-0078). No row is nil: every
// node within the project's scope.

func decodeGrant(sets string, err error) ([]string, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading node sets: %w", err)
	}
	return store.DecodeNodeSets(sets)
}

// GetRunNodeSets implements store.NodeSetStore.
func (s *Store) GetRunNodeSets(ctx context.Context, runID string) ([]string, error) {
	return decodeGrant(s.q.GetRunNodeSets(ctx, runID))
}

func setRunGrant(ctx context.Context, q *gen.Queries, runID string, sets []string) error {
	if sets == nil {
		return nil
	}
	text, err := store.EncodeNodeSets(sets)
	if err != nil {
		return err
	}
	if err := q.SetRunNodeSets(ctx, gen.SetRunNodeSetsParams{RunID: runID, Sets: text}); err != nil {
		return fmt.Errorf("storing run node sets: %w", err)
	}
	return nil
}

func setScheduleGrant(ctx context.Context, q *gen.Queries, id string, sets []string) error {
	if sets == nil {
		if err := q.ClearScheduleNodeSets(ctx, id); err != nil {
			return fmt.Errorf("clearing schedule node sets: %w", err)
		}
		return nil
	}
	text, err := store.EncodeNodeSets(sets)
	if err != nil {
		return err
	}
	if err := q.SetScheduleNodeSets(ctx, gen.SetScheduleNodeSetsParams{ScheduleID: id, Sets: text}); err != nil {
		return fmt.Errorf("storing schedule node sets: %w", err)
	}
	return nil
}

func setWebhookGrant(ctx context.Context, q *gen.Queries, id string, sets []string) error {
	if sets == nil {
		return nil
	}
	text, err := store.EncodeNodeSets(sets)
	if err != nil {
		return err
	}
	if err := q.SetWebhookNodeSets(ctx, gen.SetWebhookNodeSetsParams{WebhookID: id, Sets: text}); err != nil {
		return fmt.Errorf("storing webhook node sets: %w", err)
	}
	return nil
}

// withScheduleSets fills the node sets of schedules read through q.
func withScheduleSets(ctx context.Context, q *gen.Queries, in []store.Schedule, err error) ([]store.Schedule, error) {
	if err != nil {
		return nil, err
	}
	for i := range in {
		sets, err := decodeGrant(q.GetScheduleNodeSets(ctx, in[i].ID))
		if err != nil {
			return nil, err
		}
		in[i].NodeSets = sets
	}
	return in, nil
}

func withWebhookSets(ctx context.Context, q *gen.Queries, in []store.Webhook, err error) ([]store.Webhook, error) {
	if err != nil {
		return nil, err
	}
	for i := range in {
		sets, err := decodeGrant(q.GetWebhookNodeSets(ctx, in[i].ID))
		if err != nil {
			return nil, err
		}
		in[i].NodeSets = sets
	}
	return in, nil
}
