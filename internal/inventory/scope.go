package inventory

import (
	"context"

	"github.com/denyszorinets/damson/internal/store"
)

// Scope decides which nodes a project may use (ARCHITECTURE 12.1 node_scope).
type Scope interface {
	Allows(ctx context.Context, project string, n store.Node) (bool, error)
}

// AllNodes matches every node. It is the scope of tests and of the dev-nodes
// seeder; the controller uses ProjectScope.
type AllNodes struct{}

// Allows implements Scope.
func (AllNodes) Allows(context.Context, string, store.Node) (bool, error) { return true, nil }

// SetScope is a Scope that also knows node sets (ADR-0078). ProjectScope is one.
type SetScope interface {
	Scope
	InSets(ctx context.Context, project string, refs []string, n store.Node) (bool, error)
}

// AllowedInSets applies a project's scope and, when sets is not nil, the node
// sets of a run, schedule or webhook. A Scope that knows no sets allows nothing
// to a restricted holder (fail closed).
func AllowedInSets(ctx context.Context, sc Scope, project string, sets []string, n store.Node) (bool, error) {
	ok, err := sc.Allows(ctx, project, n)
	if err != nil || !ok || sets == nil {
		return ok, err
	}
	ss, known := sc.(SetScope)
	if !known {
		return false, nil
	}
	return ss.InSets(ctx, project, sets, n)
}
