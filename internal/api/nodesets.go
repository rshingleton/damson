package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/denyszorinets/damson/internal/auth/policy"
	"github.com/denyszorinets/damson/internal/inventory"
	"github.com/denyszorinets/damson/internal/store"
)

// NodeSet is a named CEL filter over nodes, owned by a project (ADR-0078).
type NodeSet struct {
	Ref         string `json:"ref" doc:"<project path>:<name>, how access rules name the set."`
	Project     string `json:"project" doc:"The owning project's path."`
	Name        string `json:"name"`
	Description string `json:"description"`
	Filter      string `json:"filter" doc:"A CEL expression over node (id, name, address, tags, attributes, pool, os, transport) that yields a boolean."`
	Version     int    `json:"version" doc:"Send as if_version on the next change."`
	Inherited   bool   `json:"inherited" doc:"True when the set belongs to an ancestor of the project asked about."`
}

func toNodeSetBody(ns store.NodeSet, inherited bool) NodeSet {
	return NodeSet{Ref: ns.Ref(), Project: ns.ProjectPath, Name: ns.Name, Description: ns.Description, Filter: ns.Filter, Version: ns.Version, Inherited: inherited}
}

// ProjectNode is a node as a project member sees it: no credential, host key or port.
type ProjectNode struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Address        string         `json:"address"`
	Tags           []string       `json:"tags"`
	Attributes     map[string]any `json:"attributes"`
	Pool           string         `json:"pool"`
	OS             string         `json:"os"`
	Transport      string         `json:"transport"`
	Enabled        bool           `json:"enabled"`
	VisibleThrough []string       `json:"visible_through" doc:"The caller's node sets that match the node; empty when the caller sees every node."`
}

func toProjectNode(n store.Node, through []string) ProjectNode {
	pn := ProjectNode{ID: n.ID, Name: n.Name, Address: n.Address, Tags: n.Tags, Attributes: n.Attributes, Pool: n.Pool, OS: n.OS,
		Transport: n.Transport, Enabled: n.Enabled, VisibleThrough: through}
	if pn.Tags == nil {
		pn.Tags = []string{}
	}
	if pn.Attributes == nil {
		pn.Attributes = map[string]any{}
	}
	if pn.VisibleThrough == nil {
		pn.VisibleThrough = []string{}
	}
	return pn
}

// nodeAccess is what the caller may do with nodes in a project (ADR-0078). An
// instance admin, a service token and an anonymous caller see every node within
// the project's node scope; a user sees what the matching rules give.
func (d deps) nodeAccess(ctx context.Context, project string) (policy.NodeAccess, error) {
	p, ok := PrincipalFrom(ctx)
	if !ok || p.InstanceAdmin || p.Token != nil || p.Session == nil || d.policy == nil || d.oidc == nil {
		return policy.NodeAccess{All: true}, nil
	}
	chain, err := d.ds.ListProjectChain(ctx, project)
	if err != nil {
		return policy.NodeAccess{}, err
	}
	lists := make([][]store.AccessRule, 0, len(chain))
	for _, pr := range chain {
		lists = append(lists, pr.AccessRules)
	}
	return d.policy.Nodes(d.identityOf(p.Session), lists...), nil
}

// checkNodeSetRefs makes sure every set a rule names exists on the project the
// rules belong to or on one of its ancestors. A problem is a 422 naming the
// reference by position.
func (d deps) checkNodeSetRefs(ctx context.Context, rules []AccessRule, projectPath string) error {
	var fields []ProblemDetail
	for i, r := range rules {
		if r.Role == store.RoleAdmin && len(r.NodeSets) > 0 {
			fields = append(fields, ProblemDetail{Message: "a rule that grants admin cannot carry node sets", Location: fmt.Sprintf("body.rules[%d].node_sets", i)})
			continue
		}
		for j, ref := range r.NodeSets {
			loc := fmt.Sprintf("body.rules[%d].node_sets[%d]", i, j)
			proj, name, ok := store.SplitNodeSetRef(ref)
			switch {
			case !ok:
				fields = append(fields, ProblemDetail{Message: "not a node set reference (<project path>:<name>)", Location: loc})
			case proj != projectPath && !strings.HasPrefix(projectPath, proj+"/"):
				fields = append(fields, ProblemDetail{Message: "the set must belong to this project or one of its ancestors", Location: loc})
			default:
				owner, err := d.ds.GetProjectByPath(ctx, proj)
				if err == nil {
					_, err = d.ds.GetNodeSet(ctx, owner.ID, name)
				}
				if err != nil {
					fields = append(fields, ProblemDetail{Message: "no such node set", Location: loc})
				}
			}
		}
	}
	if len(fields) > 0 {
		return invalid("the access rules are invalid", fields...)
	}
	return nil
}

func (d deps) checkNodeSetFilter(filter string) error {
	if filter == "" {
		return invalid("the node set is invalid", ProblemDetail{Message: "the filter is required; write true for every node", Location: "body.filter"})
	}
	if _, err := d.nodeScopeEnv.CompileRule(filter); err != nil {
		return invalid("the node set is invalid", ProblemDetail{Message: err.Error(), Location: "body.filter"})
	}
	return nil
}

type nodeSetPathInput struct {
	Path string `path:"path" maxLength:"600"`
	Name string `path:"name" maxLength:"63"`
}

type createNodeSetInput struct {
	Path string `path:"path" maxLength:"600"`
	Body struct {
		Name        string `json:"name" required:"true" minLength:"1" maxLength:"63"`
		Description string `json:"description,omitempty" maxLength:"500"`
		Filter      string `json:"filter" required:"true" minLength:"1" maxLength:"2048"`
	}
}

type putNodeSetInput struct {
	Path string `path:"path" maxLength:"600"`
	Name string `path:"name" maxLength:"63"`
	Body struct {
		Description string `json:"description,omitempty" maxLength:"500"`
		Filter      string `json:"filter" required:"true" minLength:"1" maxLength:"2048"`
		IfVersion   *int   `json:"if_version,omitempty" minimum:"1" doc:"The version the caller read; 409 if it moved. Omitted: the version current when handled."`
	}
}

type previewNodeSetInput struct {
	Path string `path:"path" maxLength:"600"`
	Body struct {
		Filter string `json:"filter" required:"true" minLength:"1" maxLength:"2048"`
	}
}

type nodeSetBody struct{ Body NodeSet }

type listNodeSetsOutput struct {
	Body struct {
		Items []NodeSet `json:"items"`
	}
}

type previewNodeSetOutput struct {
	Body struct {
		Items     []ProjectNode `json:"items" doc:"At most 200 nodes within the project's node scope that the filter matches."`
		Truncated bool          `json:"truncated"`
	}
}

type listProjectNodesInput struct {
	Path   string `path:"path" maxLength:"600"`
	Cursor string `query:"cursor" maxLength:"512"`
	Limit  int    `query:"limit" default:"100" minimum:"1" maximum:"500"`
}

type listProjectNodesOutput struct {
	Body struct {
		Access     string        `json:"access" enum:"all,sets" doc:"all: every node within the project's scope; sets: only the nodes of the caller's node sets."`
		Items      []ProjectNode `json:"items"`
		NextCursor string        `json:"next_cursor,omitempty"`
	}
}

const (
	previewLimit = 200
	nodePage     = 500
)

func registerNodeSets(api huma.API, d deps) {
	op := func(id, method, path, summary string, status int, errs ...int) huma.Operation {
		return huma.Operation{
			OperationID: id, Method: method, Path: path, Summary: summary, Tags: []string{"projects"}, Security: adminSecurity, DefaultStatus: status,
			Errors: append([]int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusServiceUnavailable}, errs...),
		}
	}
	huma.Register(api, op("list-project-nodes", http.MethodGet, "/projects/{path}/nodes", "List the nodes the caller may see in a project", 0, http.StatusBadRequest),
		func(ctx context.Context, in *listProjectNodesInput) (*listProjectNodesOutput, error) {
			return listProjectNodes(ctx, d, in)
		})
	huma.Register(api, op("list-node-sets", http.MethodGet, "/projects/{path}/node-sets", "List the node sets of a project and of its ancestors", 0),
		func(ctx context.Context, in *projectPathInput) (*listNodeSetsOutput, error) {
			return listNodeSets(ctx, d, in)
		})
	huma.Register(api, op("create-node-set", http.MethodPost, "/projects/{path}/node-sets", "Create a node set", http.StatusCreated, http.StatusConflict, http.StatusUnprocessableEntity),
		func(ctx context.Context, in *createNodeSetInput) (*nodeSetBody, error) {
			if err := d.ready(); err != nil {
				return nil, apiError(ctx, err)
			}
			if err := d.checkNodeSetFilter(in.Body.Filter); err != nil {
				return nil, apiError(ctx, err)
			}
			if err := store.ValidateNodeSetName(in.Body.Name); err != nil {
				return nil, apiError(ctx, invalid("the node set is invalid", ProblemDetail{Message: "the name must match [a-z0-9][a-z0-9-]*", Location: "body.name"}))
			}
			proj, err := d.ds.GetProjectByPath(ctx, in.Path)
			if err != nil {
				return nil, apiError(ctx, err)
			}
			ns, err := d.ds.CreateNodeSet(ctx, proj.ID, in.Body.Name, in.Body.Description, in.Body.Filter, d.now())
			if err != nil {
				return nil, apiError(ctx, err)
			}
			d.scope.Reset()
			return &nodeSetBody{toNodeSetBody(ns, false)}, nil
		})
	huma.Register(api, op("get-node-set", http.MethodGet, "/projects/{path}/node-sets/{name}", "Get a node set", 0),
		func(ctx context.Context, in *nodeSetPathInput) (*nodeSetBody, error) {
			ns, err := d.ownedSet(ctx, in.Path, in.Name)
			if err != nil {
				return nil, apiError(ctx, err)
			}
			return &nodeSetBody{toNodeSetBody(ns, false)}, nil
		})
	huma.Register(api, op("put-node-set", http.MethodPut, "/projects/{path}/node-sets/{name}", "Change a node set", 0, http.StatusConflict, http.StatusUnprocessableEntity),
		func(ctx context.Context, in *putNodeSetInput) (*nodeSetBody, error) {
			if err := d.checkNodeSetFilter(in.Body.Filter); err != nil {
				return nil, apiError(ctx, err)
			}
			cur, err := d.ownedSet(ctx, in.Path, in.Name)
			if err != nil {
				return nil, apiError(ctx, err)
			}
			version := cur.Version
			if in.Body.IfVersion != nil {
				version = *in.Body.IfVersion
			}
			ns, err := d.ds.UpdateNodeSet(ctx, cur.ID, in.Body.Description, in.Body.Filter, version, d.now())
			if err != nil {
				return nil, apiError(ctx, err)
			}
			d.scope.Reset()
			return &nodeSetBody{toNodeSetBody(ns, false)}, nil
		})
	huma.Register(api, op("delete-node-set", http.MethodDelete, "/projects/{path}/node-sets/{name}", "Delete a node set", http.StatusNoContent, http.StatusConflict),
		func(ctx context.Context, in *nodeSetPathInput) (*struct{}, error) {
			cur, err := d.ownedSet(ctx, in.Path, in.Name)
			if err != nil {
				return nil, apiError(ctx, err)
			}
			if err := d.ds.DeleteNodeSet(ctx, cur.ID, cur.Version, d.now()); err != nil {
				return nil, apiError(ctx, err)
			}
			d.scope.Reset()
			return &struct{}{}, nil
		})
	huma.Register(api, op("preview-node-set", http.MethodPost, "/projects/{path}/node-sets/preview", "Preview the nodes a filter matches", 0, http.StatusUnprocessableEntity),
		func(ctx context.Context, in *previewNodeSetInput) (*previewNodeSetOutput, error) {
			return previewNodeSet(ctx, d, in)
		})
}

// ownedSet is the set name of the project at path; the project's own, not an ancestor's.
func (d deps) ownedSet(ctx context.Context, path, name string) (store.NodeSet, error) {
	if err := d.ready(); err != nil {
		return store.NodeSet{}, err
	}
	proj, err := d.ds.GetProjectByPath(ctx, path)
	if err != nil {
		return store.NodeSet{}, err
	}
	return d.ds.GetNodeSet(ctx, proj.ID, name)
}

func listNodeSets(ctx context.Context, d deps, in *projectPathInput) (*listNodeSetsOutput, error) {
	if err := d.ready(); err != nil {
		return nil, apiError(ctx, err)
	}
	chain, err := d.ds.ListProjectChain(ctx, in.Path)
	if err != nil {
		return nil, apiError(ctx, err)
	}
	if len(chain) == 0 || chain[len(chain)-1].Path != in.Path {
		return nil, apiError(ctx, store.ErrNotFound)
	}
	out := &listNodeSetsOutput{}
	out.Body.Items = []NodeSet{}
	for _, pr := range chain {
		sets, err := d.ds.ListNodeSets(ctx, pr.ID)
		if err != nil {
			return nil, apiError(ctx, err)
		}
		for _, ns := range sets {
			out.Body.Items = append(out.Body.Items, toNodeSetBody(ns, pr.Path != in.Path))
		}
	}
	return out, nil
}

func previewNodeSet(ctx context.Context, d deps, in *previewNodeSetInput) (*previewNodeSetOutput, error) {
	if err := d.ready(); err != nil {
		return nil, apiError(ctx, err)
	}
	if err := d.checkNodeSetFilter(in.Body.Filter); err != nil {
		return nil, apiError(ctx, err)
	}
	if _, err := d.ds.GetProjectByPath(ctx, in.Path); err != nil {
		return nil, apiError(ctx, err)
	}
	prog, err := d.nodeScopeEnv.CompileRule(in.Body.Filter)
	if err != nil {
		return nil, apiError(ctx, err)
	}
	out := &previewNodeSetOutput{}
	out.Body.Items = []ProjectNode{}
	after := ""
	for {
		rows, err := d.ds.ListNodes(ctx, store.NodeFilter{AfterName: after, Limit: nodePage})
		if err != nil {
			return nil, apiError(ctx, err)
		}
		for _, n := range rows {
			ok, err := d.scope.Allows(ctx, in.Path, n)
			if err != nil {
				return nil, apiError(ctx, err)
			}
			if m, merr := d.nodeScopeEnv.EvalNodeScope(prog, inventory.TargetMap(n)); !ok || merr != nil || !m {
				continue // outside the ceiling, or a filter that fails for a node does not match it
			}
			if len(out.Body.Items) == previewLimit {
				out.Body.Truncated = true
				return out, nil
			}
			out.Body.Items = append(out.Body.Items, toProjectNode(n, nil))
		}
		if len(rows) < nodePage {
			return out, nil
		}
		after = rows[len(rows)-1].Name
	}
}

func listProjectNodes(ctx context.Context, d deps, in *listProjectNodesInput) (*listProjectNodesOutput, error) {
	if err := d.ready(); err != nil {
		return nil, apiError(ctx, err)
	}
	after, err := decodeCursor(ctx, in.Cursor)
	if err != nil {
		return nil, err
	}
	if _, err := d.ds.GetProjectByPath(ctx, in.Path); err != nil {
		return nil, apiError(ctx, err)
	}
	access, err := d.nodeAccess(ctx, in.Path)
	if err != nil {
		return nil, apiError(ctx, err)
	}
	out := &listProjectNodesOutput{}
	out.Body.Access = "sets"
	if access.All {
		out.Body.Access = "all"
	}
	out.Body.Items = []ProjectNode{}
	for {
		rows, err := d.ds.ListNodes(ctx, store.NodeFilter{AfterName: after, Limit: nodePage})
		if err != nil {
			return nil, apiError(ctx, err)
		}
		for _, n := range rows {
			ok, err := d.scope.Visible(ctx, in.Path, access.All, access.Sets, n)
			if err != nil {
				return nil, apiError(ctx, err)
			}
			if !ok {
				continue
			}
			if len(out.Body.Items) == in.Limit { // one more than the page: there is a next page
				out.Body.NextCursor = encodeCursor(out.Body.Items[len(out.Body.Items)-1].Name)
				return out, nil
			}
			out.Body.Items = append(out.Body.Items, toProjectNode(n, d.visibleThrough(ctx, in.Path, access, n)))
		}
		if len(rows) < nodePage {
			return out, nil
		}
		after = rows[len(rows)-1].Name
	}
}

// visibleThrough names the caller's sets that match n; none when the caller sees every node.
func (d deps) visibleThrough(ctx context.Context, project string, access policy.NodeAccess, n store.Node) []string {
	if access.All {
		return nil
	}
	var through []string
	for _, ref := range access.Sets {
		if ok, _ := d.scope.InSets(ctx, project, []string{ref}, n); ok {
			through = append(through, ref)
		}
	}
	return through
}
