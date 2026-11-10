package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Limits and audit names of node sets (ADR-0078).
const (
	MaxNodeSetsPerProject = 100
	MaxNodeSetFilterBytes = 2048
	MaxNodeSetRefsPerRule = 8
	MaxNodeSetDescription = 500

	ActionNodeSetCreate = "node_set.create"
	ActionNodeSetUpdate = "node_set.update"
	ActionNodeSetDelete = "node_set.delete"
	AuditTargetNodeSet  = "node_set"
)

// NodeSet is a named CEL filter over a node, owned by a project. Rules
// reference it as Ref (ADR-0078).
type NodeSet struct {
	ID          string
	ProjectID   string
	ProjectPath string
	Name        string
	Description string
	Filter      string
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Ref is "<project path>:<name>". A project path holds no ':'.
func (n NodeSet) Ref() string { return NodeSetRef(n.ProjectPath, n.Name) }

// NodeSetRef builds the reference of a set.
func NodeSetRef(project, name string) string { return project + ":" + name }

// SplitNodeSetRef parses a reference; ok is false when it is not well formed.
func SplitNodeSetRef(ref string) (project, name string, ok bool) {
	project, name, ok = strings.Cut(ref, ":")
	if !ok || ValidateProjectPath(project) != nil || ValidateNodeSetName(name) != nil {
		return "", "", false
	}
	return project, name, true
}

// ValidateNodeSetName accepts the grammar of one project path segment.
func ValidateNodeSetName(name string) error {
	if !projectSegmentRE.MatchString(name) {
		return fmt.Errorf("node set name %q: must match %s: %w", name, projectSegmentRE, ErrInvalid)
	}
	return nil
}

// ValidateNodeSetFields checks what the store can check without compiling CEL.
func ValidateNodeSetFields(description, filter string) error {
	switch {
	case filter == "" || len(filter) > MaxNodeSetFilterBytes:
		return fmt.Errorf("a node set filter must be 1 to %d bytes: %w", MaxNodeSetFilterBytes, ErrInvalid)
	case len(description) > MaxNodeSetDescription:
		return fmt.Errorf("a node set description is at most %d bytes: %w", MaxNodeSetDescription, ErrInvalid)
	}
	return nil
}

// ReferencesNodeSet reports whether any rule of any list names ref.
func ReferencesNodeSet(ref string, lists ...[]AccessRule) bool {
	for _, rules := range lists {
		for _, r := range rules {
			for _, s := range r.NodeSets {
				if s == ref {
					return true
				}
			}
		}
	}
	return false
}

// DistinctNodeSets is the sorted distinct references of a rule list.
func DistinctNodeSets(rules []AccessRule) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rules {
		for _, s := range r.NodeSets {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}

// NodeSetAuditDetails is the content of node set events: never the filter.
type NodeSetAuditDetails struct {
	Ref string   `json:"ref"`
	Set []string `json:"set,omitempty"`
}

// NewNodeSetEvent builds the audit event of a node set write.
func NewNodeSetEvent(ctx context.Context, action, id, project string, d NodeSetAuditDetails, now time.Time) (AuditEvent, error) {
	meta, err := AuditFrom(ctx)
	if err != nil {
		return AuditEvent{}, err
	}
	evID, err := NewID()
	if err != nil {
		return AuditEvent{}, err
	}
	details, err := json.Marshal(d)
	if err != nil {
		return AuditEvent{}, fmt.Errorf("encoding audit details: %w", err)
	}
	return AuditEvent{
		ID: evID, At: NormalizeTime(now), Actor: meta.Actor, Action: action, TargetType: AuditTargetNodeSet,
		Target: id, ProjectPath: project, Details: details, RequestID: meta.RequestID,
	}, nil
}

// NodeSetInUseError says what still references a set. It matches ErrConflict.
type NodeSetInUseError struct {
	Ref  string
	What string
}

func (e *NodeSetInUseError) Error() string {
	return fmt.Sprintf("node set %q is still referenced by %s", e.Ref, e.What)
}

// Is makes the error match ErrConflict.
func (e *NodeSetInUseError) Is(target error) bool { return target == ErrConflict }

// NodeSetStore persists node sets. Writes are audited: they need an actor from WithAudit.
type NodeSetStore interface {
	// CreateNodeSet adds a set to a project: ErrNotFound if the project is
	// absent, ErrConflict if the name is taken, ErrInvalid for a bad name, filter
	// or when the project already holds MaxNodeSetsPerProject. The filter is
	// compiled by the caller.
	CreateNodeSet(ctx context.Context, projectID, name, description, filter string, now time.Time) (NodeSet, error)
	// GetNodeSet returns ErrNotFound if absent.
	GetNodeSet(ctx context.Context, projectID, name string) (NodeSet, error)
	// ListNodeSets returns a project's sets ordered by name.
	ListNodeSets(ctx context.Context, projectID string) ([]NodeSet, error)
	// UpdateNodeSet replaces description and filter as a CAS on the version:
	// ErrNotFound if absent, ErrStale if the version moved.
	UpdateNodeSet(ctx context.Context, id, description, filter string, expectedVersion int, now time.Time) (NodeSet, error)
	// DeleteNodeSet removes a set as a CAS on the version. ErrConflict
	// (*NodeSetInUseError) while an access rule of the owning project's subtree
	// or of a job in it references the set.
	DeleteNodeSet(ctx context.Context, id string, expectedVersion int, now time.Time) error
}

// NodeSetUser names what in a project subtree references ref ("" when nothing
// does): the access rules of a project in subtree, or of a job whose project is
// in it. jobs is every job that has rules of its own.
func NodeSetUser(ref string, subtree []Project, jobs []JobRules) string {
	ids := make(map[string]bool, len(subtree))
	for _, p := range subtree {
		ids[p.ID] = true
		if ReferencesNodeSet(ref, p.AccessRules) {
			return "the access rules of project " + p.Path
		}
	}
	for _, j := range jobs {
		if ids[j.ProjectID] && ReferencesNodeSet(ref, j.Rules) {
			return "the access rules of job " + j.Name
		}
	}
	return ""
}
