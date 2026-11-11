package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/denyszorinets/damson/internal/store"
)

var _ store.StatsStore = (*Store)(nil)

// ProjectCounts implements store.StatsStore.
func (s *Store) ProjectCounts(ctx context.Context, failedSince time.Time) (map[string]store.ProjectCounts, error) {
	runs, err := s.q.CountRunsByProject(ctx, sql.NullString{String: fmtTime(failedSince), Valid: true})
	if err != nil {
		return nil, fmt.Errorf("counting runs by project: %w", err)
	}
	jobs, err := s.q.CountJobsByProject(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting jobs by project: %w", err)
	}
	out := store.CountsFold{}
	for _, j := range jobs {
		out.AddJobs(j.ProjectID, j.N)
	}
	for _, r := range runs {
		out.AddRuns(r.ProjectID, r.Status, r.N)
	}
	return out, nil
}

// JobRunStats implements store.StatsStore.
func (s *Store) JobRunStats(ctx context.Context, projectID string) (map[string]store.JobRunStats, error) {
	rows, err := s.q.LatestRunsByJobAndStatus(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("reading the latest runs of project %s: %w", projectID, err)
	}
	runs := make([]store.LatestRun, 0, len(rows))
	for _, r := range rows {
		run := store.LatestRun{JobID: r.JobID, ID: r.ID, Status: r.Status}
		if run.CreatedAt, err = parseTime(r.CreatedAt); err != nil {
			return nil, err
		}
		if run.StartedAt, err = parseNullTime(r.StartedAt); err != nil {
			return nil, err
		}
		if run.FinishedAt, err = parseNullTime(r.FinishedAt); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return store.FoldLatestRuns(runs), nil
}
