package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/denyszorinets/damson/internal/store"
	"github.com/denyszorinets/damson/internal/store/sqlite/gen"
)

var _ store.NodeSetStore = (*Store)(nil)

// maxJobRuleScan bounds the job rule lists read to check a node set delete.
const maxJobRuleScan = 100000

func toNodeSet(r gen.NodeSet, projectPath string) (store.NodeSet, error) {
	created, err := parseTime(r.CreatedAt)
	if err != nil {
		return store.NodeSet{}, err
	}
	updated, err := parseTime(r.UpdatedAt)
	if err != nil {
		return store.NodeSet{}, err
	}
	return store.NodeSet{
		ID: r.ID, ProjectID: r.ProjectID, ProjectPath: projectPath, Name: r.Name, Description: r.Description,
		Filter: r.Filter, Version: int(r.Version), CreatedAt: created, UpdatedAt: updated,
	}, nil
}

// CreateNodeSet implements store.NodeSetStore.
func (s *Store) CreateNodeSet(ctx context.Context, projectID, name, description, filter string, now time.Time) (store.NodeSet, error) {
	if err := store.ValidateNodeSetName(name); err != nil {
		return store.NodeSet{}, err
	}
	if err := store.ValidateNodeSetFields(description, filter); err != nil {
		return store.NodeSet{}, err
	}
	id, err := store.NewID()
	if err != nil {
		return store.NodeSet{}, err
	}
	var out store.NodeSet
	err = s.WithTx(ctx, func(tx store.Tx) error {
		q := tx.(*txn).q
		proj, err := q.GetProject(ctx, projectID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("project %q: %w", projectID, store.ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("reading project: %w", err)
		}
		if proj.ManagedBy != "ui" {
			return &store.ManagedError{What: "project " + proj.Path, By: proj.ManagedBy}
		}
		n, err := q.CountNodeSetsByProject(ctx, projectID)
		if err != nil {
			return fmt.Errorf("counting node sets: %w", err)
		}
		if n >= store.MaxNodeSetsPerProject {
			return fmt.Errorf("a project holds at most %d node sets: %w", store.MaxNodeSetsPerProject, store.ErrInvalid)
		}
		err = q.InsertNodeSet(ctx, gen.InsertNodeSetParams{ID: id, ProjectID: projectID, Name: name, Description: description, Filter: filter, Now: fmtTime(now)})
		if isUnique(err) {
			return fmt.Errorf("node set %q: %w", name, store.ErrConflict)
		}
		if err != nil {
			return fmt.Errorf("creating node set: %w", err)
		}
		ns := store.NodeSet{ID: id, ProjectID: projectID, ProjectPath: proj.Path, Name: name, Description: description, Filter: filter,
			Version: 1, CreatedAt: store.NormalizeTime(now), UpdatedAt: store.NormalizeTime(now)}
		ev, err := store.NewNodeSetEvent(ctx, store.ActionNodeSetCreate, id, proj.Path, store.NodeSetAuditDetails{Ref: ns.Ref()}, now)
		if err != nil {
			return err
		}
		out = ns
		return tx.AppendAuditEvent(ctx, ev)
	})
	return out, err
}

// GetNodeSet implements store.NodeSetStore.
func (s *Store) GetNodeSet(ctx context.Context, projectID, name string) (store.NodeSet, error) {
	row, err := s.q.GetNodeSetByName(ctx, gen.GetNodeSetByNameParams{ProjectID: projectID, Name: name})
	if errors.Is(err, sql.ErrNoRows) {
		return store.NodeSet{}, fmt.Errorf("node set %q: %w", name, store.ErrNotFound)
	}
	if err != nil {
		return store.NodeSet{}, fmt.Errorf("getting node set: %w", err)
	}
	proj, err := s.q.GetProject(ctx, projectID)
	if err != nil {
		return store.NodeSet{}, fmt.Errorf("reading project of node set: %w", err)
	}
	return toNodeSet(row, proj.Path)
}

// ListNodeSets implements store.NodeSetStore.
func (s *Store) ListNodeSets(ctx context.Context, projectID string) ([]store.NodeSet, error) {
	rows, err := s.q.ListNodeSetsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing node sets: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	proj, err := s.q.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("reading project of node sets: %w", err)
	}
	out := make([]store.NodeSet, len(rows))
	for i, r := range rows {
		if out[i], err = toNodeSet(r, proj.Path); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// UpdateNodeSet implements store.NodeSetStore.
func (s *Store) UpdateNodeSet(ctx context.Context, id, description, filter string, expectedVersion int, now time.Time) (store.NodeSet, error) {
	if _, err := store.AuditFrom(ctx); err != nil {
		return store.NodeSet{}, err
	}
	if err := store.ValidateNodeSetFields(description, filter); err != nil {
		return store.NodeSet{}, err
	}
	var out store.NodeSet
	err := s.WithTx(ctx, func(tx store.Tx) error {
		q := tx.(*txn).q
		cur, err := q.GetNodeSet(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("node set %q: %w", id, store.ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("reading node set: %w", err)
		}
		proj, err := q.GetProject(ctx, cur.ProjectID)
		if err != nil {
			return fmt.Errorf("reading project of node set: %w", err)
		}
		if proj.ManagedBy != "ui" {
			return &store.ManagedError{What: "project " + proj.Path, By: proj.ManagedBy}
		}
		n, err := q.UpdateNodeSetCAS(ctx, gen.UpdateNodeSetCASParams{ID: id, Description: description, Filter: filter, UpdatedAt: fmtTime(now), ExpectedVersion: int64(expectedVersion)})
		if err != nil {
			return fmt.Errorf("updating node set: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("node set %q version %d: %w", id, expectedVersion, store.ErrStale)
		}
		changed := []string{}
		if cur.Description != description {
			changed = append(changed, "description")
		}
		if cur.Filter != filter {
			changed = append(changed, "filter")
		}
		cur.Description, cur.Filter, cur.Version, cur.UpdatedAt = description, filter, int64(expectedVersion)+1, fmtTime(now)
		if out, err = toNodeSet(cur, proj.Path); err != nil {
			return err
		}
		ev, err := store.NewNodeSetEvent(ctx, store.ActionNodeSetUpdate, id, proj.Path, store.NodeSetAuditDetails{Ref: out.Ref(), Set: changed}, now)
		if err != nil {
			return err
		}
		return tx.AppendAuditEvent(ctx, ev)
	})
	return out, err
}

// DeleteNodeSet implements store.NodeSetStore.
func (s *Store) DeleteNodeSet(ctx context.Context, id string, expectedVersion int, now time.Time) error {
	if _, err := store.AuditFrom(ctx); err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx store.Tx) error {
		q := tx.(*txn).q
		cur, err := q.GetNodeSet(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("node set %q: %w", id, store.ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("reading node set: %w", err)
		}
		proj, err := q.GetProject(ctx, cur.ProjectID)
		if err != nil {
			return fmt.Errorf("reading project of node set: %w", err)
		}
		if proj.ManagedBy != "ui" {
			return &store.ManagedError{What: "project " + proj.Path, By: proj.ManagedBy}
		}
		ref := store.NodeSetRef(proj.Path, cur.Name)
		subtree, err := q.ListProjectSubtree(ctx, gen.ListProjectSubtreeParams{Path: proj.Path, PathSlash: proj.Path + "/", PathNext: proj.Path + "0"})
		if err != nil {
			return fmt.Errorf("listing project subtree: %w", err)
		}
		projects, err := toProjects(subtree)
		if err != nil {
			return err
		}
		jobRows, err := q.ListAllJobRuleRows(ctx, maxJobRuleScan)
		if err != nil {
			return fmt.Errorf("listing job rules: %w", err)
		}
		jobs := make([]store.JobRules, 0, len(jobRows))
		for _, r := range jobRows {
			rules, err := store.DecodeAccessRules([]byte(r.Rules))
			if err != nil {
				return err
			}
			jobs = append(jobs, store.JobRules{JobID: r.JobID, Name: r.Name, ProjectID: r.ProjectID, Rules: rules})
		}
		if what := store.NodeSetUser(ref, projects, jobs); what != "" {
			return &store.NodeSetInUseError{Ref: ref, What: what}
		}
		schedTexts, err := q.ListScheduleNodeSetTexts(ctx)
		if err != nil {
			return fmt.Errorf("listing schedule node sets: %w", err)
		}
		hookTexts, err := q.ListWebhookNodeSetTexts(ctx)
		if err != nil {
			return fmt.Errorf("listing webhook node sets: %w", err)
		}
		kind, err := store.NodeSetUsedByGrants(ref, schedTexts, hookTexts)
		if err != nil {
			return err
		}
		if kind != "" {
			return &store.NodeSetInUseError{Ref: ref, What: kind}
		}
		n, err := q.DeleteNodeSetCAS(ctx, gen.DeleteNodeSetCASParams{ID: id, ExpectedVersion: int64(expectedVersion)})
		if err != nil {
			return fmt.Errorf("deleting node set: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("node set %q version %d: %w", id, expectedVersion, store.ErrStale)
		}
		ev, err := store.NewNodeSetEvent(ctx, store.ActionNodeSetDelete, id, proj.Path, store.NodeSetAuditDetails{Ref: ref}, now)
		if err != nil {
			return err
		}
		return tx.AppendAuditEvent(ctx, ev)
	})
}
