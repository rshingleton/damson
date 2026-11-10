package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/denyszorinets/damson/internal/api"
	"github.com/denyszorinets/damson/internal/store"
)

// nodeSetWorld adds nodes and the sets of project team to the authz world:
// bob (runner) gets the set "web", carol (viewer) gets no sets, gina is admin.
func nodeSetWorld(t *testing.T, st store.Store) *authzWorld {
	t.Helper()
	w := newAuthzWorld(t, st, 1)
	for _, n := range []string{
		`{"name":"web-1","address":"10.0.0.1","tags":["web","prod"]}`,
		`{"name":"web-2","address":"10.0.0.2","tags":["web"]}`,
		`{"name":"db-1","address":"10.0.0.3","tags":["db","prod"],"host_key_policy":"tofu"}`,
	} {
		w.mustAdmin("POST", "/api/v1/nodes", n, 201)
	}
	w.mustAdmin("POST", "/api/v1/projects/team/node-sets", `{"name":"web","description":"web tier","filter":"'web' in node.tags"}`, 201)
	w.mustAdmin("PUT", "/api/v1/projects/team/access-rules", `{"rules":[
		{"role":"runner","rule":"'team-ops' in claims.groups","node_sets":["team:web"]},
		{"role":"viewer","rule":"'team-viewers' in user.groups"},
		{"role":"admin","rule":"'team-admins' in claims.groups"}]}`, 200)
	return w
}

type projectNodes struct {
	Access     string            `json:"access"`
	Items      []api.ProjectNode `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

func (w *authzWorld) projectNodes(user, path string, want int) (projectNodes, string) {
	w.t.Helper()
	var rec = w.do(0, "GET", path, "", withCookies(cookieHeader(w.sess[user])))
	require.Equal(w.t, want, rec.Code, "%s as %s: %s", path, user, rec.Body.String())
	var out projectNodes
	if want == http.StatusOK {
		require.NoError(w.t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return out, rec.Body.String()
}

func nodeNames(items []api.ProjectNode) []string {
	out := []string{}
	for _, n := range items {
		out = append(out, n.Name)
	}
	return out
}

func TestProjectNodesFollowTheCallersSets(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := nodeSetWorld(t, st)
		const path = "/api/v1/projects/team/nodes"

		bob, raw := w.projectNodes("bob", path, 200)
		assert.Equal(t, "sets", bob.Access)
		assert.Equal(t, []string{"web-1", "web-2"}, nodeNames(bob.Items), "only the nodes of bob's set")
		assert.Equal(t, []string{"team:web"}, bob.Items[0].VisibleThrough)
		for _, secret := range []string{"credential", "host_key", "port", "host_key_policy"} {
			assert.NotContains(t, raw, `"`+secret+`"`, "a project member never gets %s", secret)
		}

		// The same rules apply in a child project.
		child, _ := w.projectNodes("bob", "/api/v1/projects/team%2Fweb/nodes", 200)
		assert.Equal(t, []string{"web-1", "web-2"}, nodeNames(child.Items))

		// A rule without sets (carol's viewer rule), an admin rule and an instance admin see every node.
		for _, user := range []string{"carol", "gina", "alice"} {
			got, _ := w.projectNodes(user, path, 200)
			assert.Equal(t, "all", got.Access, user)
			assert.Equal(t, []string{"db-1", "web-1", "web-2"}, nodeNames(got.Items), user)
			assert.Empty(t, got.Items[0].VisibleThrough, user)
		}

		// No role is a missing project; no rule at all is the same answer as a missing project.
		w.projectNodes("erin", path, 404)
		w.projectNodes("alice", "/api/v1/projects/nope/nodes", 404)

		// The ceiling still applies: a node scope narrows what a set can reach.
		w.mustAdmin("PUT", "/api/v1/projects/team/node-scope", `{"node_scope":"'prod' in node.tags"}`, 200)
		bob, _ = w.projectNodes("bob", path, 200)
		assert.Equal(t, []string{"web-1"}, nodeNames(bob.Items), "web and prod")
		gina, _ := w.projectNodes("gina", path, 200)
		assert.Equal(t, []string{"db-1", "web-1"}, nodeNames(gina.Items), "admins are held to the ceiling too")

		// Paging by name.
		page, _ := w.projectNodes("gina", path+"?limit=1", 200)
		require.Len(t, page.Items, 1)
		require.NotEmpty(t, page.NextCursor)
		next, _ := w.projectNodes("gina", path+"?limit=1&cursor="+page.NextCursor, 200)
		assert.Equal(t, []string{"web-1"}, nodeNames(next.Items))
		assert.Empty(t, next.NextCursor)
	})
}

func TestNodeSetRoutes(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := nodeSetWorld(t, st)
		const sets = "/api/v1/projects/team/node-sets"
		body := func(user, method, path, b string) (int, string) {
			opts := []reqOpt{withCookies(cookieHeader(w.sess[user]))}
			if method != http.MethodGet {
				opts = append(opts, withHeader(api.CSRFHeader, w.csrf[user]))
			}
			rec := w.do(0, method, path, b, opts...)
			return rec.Code, rec.Body.String()
		}

		// Only a project admin manages sets; a lower role or no role is told like other admin routes.
		for user, want := range map[string]int{"bob": 403, "carol": 403, "erin": 404, "gina": 200, "alice": 200} {
			got, _ := body(user, "GET", sets, "")
			assert.Equal(t, want, got, "list as %s", user)
		}
		code, _ := body("bob", "POST", sets, `{"name":"x","filter":"true"}`)
		assert.Equal(t, 403, code)

		code, out := body("gina", "POST", sets, `{"name":"db","description":"databases","filter":"'db' in node.tags"}`)
		require.Equal(t, 201, code, out)
		var db api.NodeSet
		require.NoError(t, json.Unmarshal([]byte(out), &db))
		assert.Equal(t, "team:db", db.Ref)
		assert.Equal(t, 1, db.Version)

		for name, b := range map[string]string{
			"a filter that does not compile": `{"name":"bad","filter":"node.tags.("}`,
			"a bad name":                     `{"name":"Bad Name","filter":"true"}`,
		} {
			code, out := body("gina", "POST", sets, b)
			assert.Equal(t, 422, code, "%s: %s", name, out)
		}
		code, out = body("gina", "POST", sets, `{"name":"nofilter","filter":"node.nope("}`)
		assert.Equal(t, 422, code)
		assert.Contains(t, out, "body.filter")
		code, _ = body("gina", "POST", sets, `{"name":"db","filter":"true"}`)
		assert.Equal(t, 409, code, "a name is unique in its project")

		// A child project lists the sets of its ancestors, marked inherited.
		code, out = body("gina", "GET", "/api/v1/projects/team%2Fweb/node-sets", "")
		require.Equal(t, 200, code, out)
		var list struct{ Items []api.NodeSet }
		require.NoError(t, json.Unmarshal([]byte(out), &list))
		require.Len(t, list.Items, 2)
		assert.True(t, list.Items[0].Inherited)

		// A change takes effect at once on this controller, and a stale version is refused.
		code, out = body("gina", "PUT", sets+"/db", `{"filter":"'db' in node.tags && 'prod' in node.tags","if_version":1}`)
		require.Equal(t, 200, code, out)
		code, _ = body("gina", "PUT", sets+"/db", `{"filter":"true","if_version":1}`)
		assert.Equal(t, 409, code)
		code, _ = body("gina", "GET", sets+"/nope", "")
		assert.Equal(t, 404, code)

		// Rules may only name sets that exist on the project or an ancestor, and never carry them on admin.
		ruleBody := func(rule string) string {
			return `{"rules":[` + rule + `]}`
		}
		for name, rule := range map[string]string{
			"a set that does not exist":     `{"role":"viewer","rule":"true","node_sets":["team:gone"]}`,
			"a set of another project":      `{"role":"viewer","rule":"true","node_sets":["ops:web"]}`,
			"a malformed reference":         `{"role":"viewer","rule":"true","node_sets":["web"]}`,
			"admin carrying a set":          `{"role":"admin","rule":"true","node_sets":["team:web"]}`,
			"more than eight set refs":      `{"role":"viewer","rule":"true","node_sets":["team:a","team:b","team:c","team:d","team:e","team:f","team:g","team:h","team:i"]}`,
			"a set of a descendant project": `{"role":"viewer","rule":"true","node_sets":["team/web:x"]}`,
		} {
			code, out := body("gina", "PUT", "/api/v1/projects/team/access-rules", ruleBody(rule))
			assert.Contains(t, []int{400, 422}, code, "%s: %s", name, out)
		}
		code, out = body("gina", "PUT", "/api/v1/projects/team%2Fweb/access-rules", ruleBody(`{"role":"viewer","rule":"true","node_sets":["team:web"]}`))
		assert.Equal(t, 200, code, "a child rule may use a set of its ancestor: %s", out)

		// A set that a rule uses cannot be deleted; the answer names what uses it.
		code, out = body("gina", "DELETE", sets+"/web", "")
		assert.Equal(t, 409, code)
		assert.Contains(t, out, "access rules of project team")
		w.mustAdmin("PUT", "/api/v1/projects/team%2Fweb/access-rules", `{"rules":[]}`, 200)
		code, _ = body("gina", "DELETE", sets+"/db", "")
		assert.Equal(t, 204, code)
		code, _ = body("gina", "GET", sets+"/db", "")
		assert.Equal(t, 404, code)

		// A rule that names a set is audited by reference, never by the filter.
		rec := w.admin("GET", "/api/v1/audit?limit=200", "")
		require.Equal(t, 200, rec.Code)
		assert.Contains(t, rec.Body.String(), `team:web`)
		assert.NotContains(t, rec.Body.String(), `'web' in node.tags`, "the audit trail never holds a filter")
	})
}

func TestNodeSetPreview(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := nodeSetWorld(t, st)
		const path = "/api/v1/projects/team/node-sets/preview"
		var got struct {
			Items     []api.ProjectNode `json:"items"`
			Truncated bool              `json:"truncated"`
		}
		require.NoError(t, json.Unmarshal(w.mustAdmin("POST", path, `{"filter":"node.name.matches('^web-')"}`, 200), &got))
		assert.Equal(t, []string{"web-1", "web-2"}, nodeNames(got.Items))
		assert.False(t, got.Truncated)

		// The project's ceiling is part of the preview.
		w.mustAdmin("PUT", "/api/v1/projects/team/node-scope", `{"node_scope":"'prod' in node.tags"}`, 200)
		require.NoError(t, json.Unmarshal(w.mustAdmin("POST", path, `{"filter":"true"}`, 200), &got))
		assert.Equal(t, []string{"db-1", "web-1"}, nodeNames(got.Items))

		// A filter that fails to compile is a 422 with its position; a filter that fails for a node matches nothing.
		out := w.mustAdmin("POST", path, `{"filter":"node.tags.("}`, 422)
		assert.Contains(t, string(out), "body.filter")
		require.NoError(t, json.Unmarshal(w.mustAdmin("POST", path, `{"filter":"node.attributes.region == 'eu'"}`, 200), &got))
		assert.Empty(t, got.Items)

		// Only an admin of the project previews.
		rec := w.do(0, "POST", path, `{"filter":"true"}`, withCookies(cookieHeader(w.sess["bob"])), withHeader(api.CSRFHeader, w.csrf["bob"]))
		assert.Equal(t, 403, rec.Code)
		assert.False(t, strings.Contains(rec.Body.String(), "web-1"))
	})
}

func TestServiceTokensSeeNodesWithinTheCeilingAndNeverManageSets(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := nodeSetWorld(t, st)
		var acct api.ServiceAccount
		require.NoError(t, json.Unmarshal(w.mustAdmin("POST", "/api/v1/service-accounts", `{"name":"ci"}`, 201), &acct))
		var tok api.CreatedServiceToken
		require.NoError(t, json.Unmarshal(w.mustAdmin("POST", "/api/v1/service-accounts/"+acct.ID+"/tokens", `{"name":"t","project":"team","role":"runner"}`, 201), &tok))
		call := func(method, path, body string) *httptest.ResponseRecorder {
			return w.do(0, method, path, body, withHeader("Authorization", "Bearer "+tok.Token))
		}
		rec := call("GET", "/api/v1/projects/team/nodes", "")
		require.Equal(t, 200, rec.Code, rec.Body.String())
		var got projectNodes
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, "all", got.Access)
		assert.Equal(t, []string{"db-1", "web-1", "web-2"}, nodeNames(got.Items))
		assert.Equal(t, 403, call("GET", "/api/v1/projects/team/node-sets", "").Code)
		assert.Equal(t, 403, call("POST", "/api/v1/projects/team/node-sets", `{"name":"x","filter":"true"}`).Code)
		assert.Equal(t, 404, call("GET", "/api/v1/projects/ops/nodes", "").Code, "out of the token's scope")
	})
}

func TestRunsSchedulesAndWebhooksCarryTheCreatorsSets(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := nodeSetWorld(t, st)
		// dave is an editor in team/web through a rule limited to the set "web".
		w.mustAdmin("PUT", "/api/v1/projects/team%2Fweb/access-rules",
			`{"rules":[{"role":"editor","rule":"'team-leads' in claims.groups","node_sets":["team:web"]}]}`, 200)
		call := func(user, method, path, body string) (int, []byte) {
			opts := []reqOpt{withCookies(cookieHeader(w.sess[user]))}
			if method != http.MethodGet {
				opts = append(opts, withHeader(api.CSRFHeader, w.csrf[user]))
			}
			rec := w.do(0, method, path, body, opts...)
			return rec.Code, rec.Body.Bytes()
		}
		runSets := func(user string) []string {
			code, out := call(user, "POST", "/api/v1/jobs/"+w.webJob+"/runs", `{"options":{}}`)
			require.Equal(t, 201, code, "%s: %s", user, out)
			var run struct {
				ID string `json:"id"`
			}
			require.NoError(t, json.Unmarshal(out, &run))
			sets, err := st.GetRunNodeSets(t.Context(), run.ID)
			require.NoError(t, err)
			return sets
		}
		// A run carries what its caller may use: bob's rule, dave's rule, and every node for admins.
		assert.Equal(t, []string{"team:web"}, runSets("bob"))
		assert.Equal(t, []string{"team:web"}, runSets("dave"))
		assert.Nil(t, runSets("gina"))
		assert.Nil(t, runSets("alice"))

		// A schedule made by dave carries dave's sets; the others' do not.
		sched := func(user, method, path, body string, want int) api.Schedule {
			code, out := call(user, method, path, body)
			require.Equal(t, want, code, "%s %s: %s", user, path, out)
			var s api.Schedule
			require.NoError(t, json.Unmarshal(out, &s))
			return s
		}
		mine := sched("dave", "POST", "/api/v1/schedules", `{"job_id":"`+w.webJob+`","name":"nightly","cron":"0 3 * * *"}`, 201)
		assert.Equal(t, []string{"team:web"}, mine.NodeSets)
		// Pause and resume keep them, whoever does it.
		paused := sched("gina", "PATCH", "/api/v1/schedules/"+mine.ID, `{"enabled":false}`, 200)
		assert.Equal(t, []string{"team:web"}, paused.NodeSets)
		resumed := sched("gina", "PATCH", "/api/v1/schedules/"+mine.ID, `{"enabled":true}`, 200)
		assert.Equal(t, []string{"team:web"}, resumed.NodeSets)
		// Any other change makes the editor the owner: an admin has every node.
		edited := sched("gina", "PATCH", "/api/v1/schedules/"+mine.ID, `{"cron":"0 4 * * *"}`, 200)
		assert.Empty(t, edited.NodeSets)
		// And the same by dave narrows it again.
		again := sched("dave", "PATCH", "/api/v1/schedules/"+mine.ID, `{"cron":"0 5 * * *"}`, 200)
		assert.Equal(t, []string{"team:web"}, again.NodeSets)
		stored, err := st.GetSchedule(t.Context(), mine.ID)
		require.NoError(t, err)
		assert.Equal(t, []string{"team:web"}, stored.NodeSets, "what the scheduler fires with")

		// A webhook is created by an admin, so it may use every node within the project's scope.
		code, out := call("gina", "POST", "/api/v1/webhooks", `{"job_id":"`+w.webJob+`","name":"ci"}`)
		require.Equal(t, 201, code, string(out))
		var hook api.CreatedWebhook
		require.NoError(t, json.Unmarshal(out, &hook))
		assert.Empty(t, hook.NodeSets)
	})
}
