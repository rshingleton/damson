package storetest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/denyszorinets/damson/internal/store"
)

func testNodeSets(t *testing.T, s store.Store) {
	ctx := actorCtx()
	migrated(t, s)
	p := mustProject(t, s, "ns-a", "nsa")
	child := mustProject(t, s, "ns-ab", "nsa/b")

	ns, err := s.CreateNodeSet(ctx, p.ID, "prod-web", "web", `'web' in node.tags`, t0)
	if err != nil || ns.Version != 1 || ns.Ref() != "nsa:prod-web" {
		t.Fatalf("CreateNodeSet = %+v, %v", ns, err)
	}
	if _, err := s.CreateNodeSet(ctx, p.ID, "prod-web", "", "true", t0); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate name: %v; want ErrConflict", err)
	}
	// The same name in another project is fine.
	if _, err := s.CreateNodeSet(ctx, child.ID, "prod-web", "", "true", t0); err != nil {
		t.Fatalf("same name in a child: %v", err)
	}
	for _, bad := range []struct{ name, filter string }{{"Bad Name", "true"}, {"ok", ""}, {"ok", strings.Repeat("x", store.MaxNodeSetFilterBytes+1)}} {
		if _, err := s.CreateNodeSet(ctx, p.ID, bad.name, "", bad.filter, t0); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("CreateNodeSet(%q, %d bytes): %v; want ErrInvalid", bad.name, len(bad.filter), err)
		}
	}
	if _, err := s.CreateNodeSet(ctx, "no-such-project", "x", "", "true", t0); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing project: %v; want ErrNotFound", err)
	}
	if _, err := s.CreateNodeSet(context.Background(), p.ID, "noactor", "", "true", t0); err == nil {
		t.Fatal("a write without an actor must fail")
	}

	got, err := s.GetNodeSet(ctx, p.ID, "prod-web")
	if err != nil || got.ID != ns.ID || got.Filter != `'web' in node.tags` || got.ProjectPath != "nsa" || !got.CreatedAt.Equal(store.NormalizeTime(t0)) {
		t.Fatalf("GetNodeSet = %+v, %v", got, err)
	}
	if _, err := s.GetNodeSet(ctx, p.ID, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing set: %v", err)
	}
	if _, err := s.CreateNodeSet(ctx, p.ID, "a-first", "", "true", t0); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListNodeSets(ctx, p.ID)
	if err != nil || len(list) != 2 || list[0].Name != "a-first" || list[1].Name != "prod-web" {
		t.Fatalf("ListNodeSets = %+v, %v; want ordered by name", list, err)
	}

	// CAS: a stale version loses, the current one wins and bumps the version.
	up, err := s.UpdateNodeSet(ctx, ns.ID, "d2", `node.name.matches('^web')`, 1, t0)
	if err != nil || up.Version != 2 || up.Filter != `node.name.matches('^web')` {
		t.Fatalf("UpdateNodeSet = %+v, %v", up, err)
	}
	if _, err := s.UpdateNodeSet(ctx, ns.ID, "d3", "true", 1, t0); !errors.Is(err, store.ErrStale) {
		t.Fatalf("stale update: %v; want ErrStale", err)
	}
	if _, err := s.UpdateNodeSet(ctx, "no-such", "", "true", 1, t0); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing update: %v", err)
	}

	// The audit trail holds the reference and the changed fields, never the filter.
	evs, err := s.ListAuditEvents(context.Background(), store.AuditFilter{TargetType: store.AuditTargetNodeSet, Target: ns.ID}, 10)
	if err != nil || len(evs) != 2 {
		t.Fatalf("audit = %v, %v; want create and update", evs, err)
	}
	for _, e := range evs {
		if strings.Contains(string(e.Details), "web") && strings.Contains(string(e.Details), "tags") || strings.Contains(string(e.Details), "matches") {
			t.Fatalf("audit details hold the filter: %s", e.Details)
		}
		if e.ProjectPath != "nsa" {
			t.Fatalf("event project = %q", e.ProjectPath)
		}
	}

	// A rule on the project, or on a descendant, keeps the set from being deleted.
	rules := []store.AccessRule{{Role: "viewer", Rule: "true", NodeSets: []string{"nsa:prod-web"}}}
	cur, _ := s.GetProjectByPath(ctx, "nsa/b")
	if _, err := s.SetProjectAccessRules(ctx, child.ID, rules, cur.Version, t0); err != nil {
		t.Fatal(err)
	}
	var inUse *store.NodeSetInUseError
	if err := s.DeleteNodeSet(ctx, ns.ID, 2, t0); !errors.As(err, &inUse) || !errors.Is(err, store.ErrConflict) {
		t.Fatalf("delete of a referenced set: %v; want NodeSetInUseError", err)
	}
	cur, _ = s.GetProjectByPath(ctx, "nsa/b")
	if _, err := s.SetProjectAccessRules(ctx, child.ID, nil, cur.Version, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteNodeSet(ctx, ns.ID, 1, t0); !errors.Is(err, store.ErrStale) {
		t.Fatalf("delete at a stale version: %v", err)
	}
	if err := s.DeleteNodeSet(ctx, ns.ID, 2, t0); err != nil {
		t.Fatalf("DeleteNodeSet: %v", err)
	}
	if err := s.DeleteNodeSet(ctx, ns.ID, 2, t0); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}

	// Deleting a project deletes its sets, with an event each.
	if err := s.DeleteProject(ctx, child.ID, t0); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if left, err := s.ListNodeSets(ctx, child.ID); err != nil || len(left) != 0 {
		t.Fatalf("sets of a deleted project = %v, %v", left, err)
	}
	evs, _ = s.ListAuditEvents(context.Background(), store.AuditFilter{TargetType: store.AuditTargetNodeSet}, 50)
	deletes := 0
	for _, e := range evs {
		if e.Action == store.ActionNodeSetDelete && e.ProjectPath == "nsa/b" {
			deletes++
		}
	}
	if deletes != 1 {
		t.Fatalf("delete events for nsa/b = %d; want 1", deletes)
	}
}

func testNodeSetRuleShape(t *testing.T, s store.Store) {
	ctx := actorCtx()
	migrated(t, s)
	p := mustProject(t, s, "nr-a", "nra")
	ok := []store.AccessRule{{Role: "viewer", Rule: "true", NodeSets: []string{"nra:x"}}}
	if _, err := s.SetProjectAccessRules(ctx, p.ID, ok, p.Version, t0); err != nil {
		t.Fatalf("a viewer rule with a set: %v", err)
	}
	cur, _ := s.GetProjectByPath(ctx, "nra")
	if len(cur.AccessRules) != 1 || len(cur.AccessRules[0].NodeSets) != 1 || cur.AccessRules[0].NodeSets[0] != "nra:x" {
		t.Fatalf("rules did not round-trip: %+v", cur.AccessRules)
	}
	for name, rule := range map[string]store.AccessRule{
		"admin with sets": {Role: "admin", Rule: "true", NodeSets: []string{"nra:x"}},
		"bad reference":   {Role: "viewer", Rule: "true", NodeSets: []string{"no-colon"}},
		"too many":        {Role: "viewer", Rule: "true", NodeSets: []string{"a:1", "a:2", "a:3", "a:4", "a:5", "a:6", "a:7", "a:8", "a:9"}},
	} {
		if _, err := s.SetProjectAccessRules(ctx, p.ID, []store.AccessRule{rule}, cur.Version, t0); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("%s: %v; want ErrInvalid", name, err)
		}
	}
}

func testNodeSetGuards(t *testing.T, s store.Store) {
	ctx := actorCtx()
	migrated(t, s)
	p := mustProject(t, s, "ng-a", "nga")
	ns, err := s.CreateNodeSet(ctx, p.ID, "web", "", "true", t0)
	if err != nil {
		t.Fatal(err)
	}

	// A job's own rule keeps the set, live or archived.
	nj := store.NewJob{ProjectID: p.ID, Name: "j", Spec: []byte(specJSON), Checksum: "c"}
	job, _, err := s.CreateJob(ctx, nj, t0)
	if err != nil {
		t.Fatal(err)
	}
	rules := []store.AccessRule{{Role: "runner", Rule: "true", NodeSets: []string{"nga:web"}}}
	if _, err := s.SetJobAccessRules(ctx, job.ID, rules, 0, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteNodeSet(ctx, ns.ID, 1, t0); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("delete with a job rule: %v; want ErrConflict", err)
	}
	arch := store.ApplyOp{Kind: store.OpJobArchive, Source: "files:main", ExpectManaged: store.SourceUI, ID: job.ID, ExpectCurrent: 1}
	if err := s.ApplyPlan(ctx, []store.ApplyOp{arch}, t0); err != nil {
		t.Fatalf("archive job: %v", err)
	}
	if err := s.DeleteNodeSet(ctx, ns.ID, 1, t0); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("delete with an archived job's rule: %v; want ErrConflict", err)
	}

	// A project that files manage refuses set writes.
	m := []store.ApplyOp{{Kind: store.OpProjectCreate, Source: "files:main", Path: "ngm"}}
	if err := s.ApplyPlan(ctx, m, t0); err != nil {
		t.Fatal(err)
	}
	mp, _ := s.GetProjectByPath(ctx, "ngm")
	var managed *store.ManagedError
	if _, err := s.CreateNodeSet(ctx, mp.ID, "x", "", "true", t0); !errors.As(err, &managed) {
		t.Fatalf("create in a managed project: %v; want ManagedError", err)
	}
}
