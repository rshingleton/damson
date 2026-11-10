// Package policy decides the role a user holds from CEL access rules
// (ARCHITECTURE 12.4, ADR-0064). It is pure: it reads no store and no clock.
package policy

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/denyszorinets/damson/internal/expr"
	"github.com/denyszorinets/damson/internal/store"
)

// maxCached bounds the compiled-program cache; it is cleared when full.
const maxCached = 4096

// Evaluator compiles and evaluates access rules. Programs are cached by their
// source text, so a rule that did not change is compiled once.
type Evaluator struct {
	env *expr.Env
	log *slog.Logger

	mu    sync.Mutex
	cache map[string]*expr.Program
}

// New returns an Evaluator. A nil logger uses the default.
func New(log *slog.Logger) (*Evaluator, error) {
	env, err := expr.NewAccessEnv(expr.Limits{})
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &Evaluator{env: env, log: log, cache: map[string]*expr.Program{}}, nil
}

// Check compiles a rule without running it. The error says what is wrong with
// the expression and never quotes it.
func (e *Evaluator) Check(rule string) error {
	_, err := e.program(rule)
	return err
}

func (e *Evaluator) program(rule string) (*expr.Program, error) {
	e.mu.Lock()
	p, ok := e.cache[rule]
	e.mu.Unlock()
	if ok {
		return p, nil
	}
	p, err := e.env.CompileRule(rule)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	if len(e.cache) >= maxCached {
		clear(e.cache)
	}
	e.cache[rule] = p
	e.mu.Unlock()
	return p, nil
}

// Grant is the outcome of evaluating rule lists for one identity: the role and
// the rule that gave it. List and Index locate the rule (which of the lists
// passed in, and its place in that list); both are -1 when no rule granted a role.
type Grant struct {
	Role  string
	List  int
	Index int
}

// Explain evaluates the rule lists for id and says which rule granted the role:
// the highest role any rule grants, and of equal roles the first. A rule grants
// its role when it evaluates to true; one that does not compile or does not
// evaluate (for example because a claim is missing) grants nothing. The log names
// the rule's role, never claim values (at debug level: a rule on a claim some
// users lack fails on every request). Role is this function, so what is explained
// is what is enforced.
func (e *Evaluator) Explain(id expr.Identity, lists ...[]store.AccessRule) Grant {
	best := Grant{List: -1, Index: -1}
	for li, rules := range lists {
		for i, r := range rules {
			if store.RoleRank(r.Role) <= store.RoleRank(best.Role) {
				continue // cannot raise the result; skip the evaluation
			}
			p, err := e.program(r.Rule)
			if err != nil {
				e.log.Warn("access rule does not compile; it grants nothing", "role", r.Role, "index", i)
				continue
			}
			ok, err := e.env.EvalRule(p, id)
			if err != nil {
				e.log.Debug("access rule did not evaluate; it grants nothing", "role", r.Role, "index", i, "error", err)
				continue
			}
			if ok {
				best = Grant{Role: r.Role, List: li, Index: i}
			}
		}
	}
	return best
}

// Role is the highest role granted to id by any rule of any list ("" for none).
func (e *Evaluator) Role(id expr.Identity, lists ...[]store.AccessRule) string {
	return e.Explain(id, lists...).Role
}

// FieldError locates one invalid rule.
type FieldError struct {
	Index   int
	Message string
}

// Validate compiles every rule of a list and reports the invalid ones with the
// compiler's message (which describes the expression, not user data).
func (e *Evaluator) Validate(rules []store.AccessRule) []FieldError {
	var out []FieldError
	for i, r := range rules {
		if err := e.Check(r.Rule); err != nil {
			out = append(out, FieldError{Index: i, Message: fmt.Sprint(err)})
		}
	}
	return out
}

// NodeAccess is what a user may do with nodes (ADR-0078): every node within the
// project's node scope (All), or only those in one of Sets (references, sorted).
type NodeAccess struct {
	All  bool
	Sets []string
}

// Nodes evaluates every rule of the lists for id, without the shortcut Explain
// takes, and returns the node access they give. A matching admin rule or a
// matching rule with no node sets gives All; otherwise the access is the union
// of the sets of the matching rules. No matching rule yields no sets (the caller
// holds no role, so the question does not arise).
func (e *Evaluator) Nodes(id expr.Identity, lists ...[]store.AccessRule) NodeAccess {
	seen := map[string]bool{}
	var out NodeAccess
	for _, rules := range lists {
		for i, r := range rules {
			p, err := e.program(r.Rule)
			if err != nil {
				continue // logged by Explain; it grants nothing
			}
			ok, err := e.env.EvalRule(p, id)
			if err != nil {
				e.log.Debug("access rule did not evaluate; it grants nothing", "role", r.Role, "index", i, "error", err)
				continue
			}
			if !ok {
				continue
			}
			if r.Role == store.RoleAdmin || len(r.NodeSets) == 0 {
				out.All = true
				continue
			}
			for _, s := range r.NodeSets {
				if !seen[s] {
					seen[s] = true
					out.Sets = append(out.Sets, s)
				}
			}
		}
	}
	sort.Strings(out.Sets)
	if out.All {
		out.Sets = nil
	}
	return out
}
