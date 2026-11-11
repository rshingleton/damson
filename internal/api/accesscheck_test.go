package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/denyszorinets/damson/internal/api"
	"github.com/denyszorinets/damson/internal/store"
)

type accessCheckBody struct {
	Project string `json:"project"`
	JobID   string `json:"job_id"`
	Role    string `json:"role"`
	Rules   int    `json:"rules"`
	Source  *struct {
		Kind      string `json:"kind"`
		Project   string `json:"project"`
		RuleIndex int    `json:"rule_index"`
		Rule      string `json:"rule"`
	} `json:"source"`
}

func (w *authzWorld) check(body string) (int, accessCheckBody) {
	w.t.Helper()
	rec := w.admin("POST", "/api/v1/access/check", body)
	var b accessCheckBody
	if rec.Code == http.StatusOK {
		require.NoError(w.t, json.Unmarshal(rec.Body.Bytes(), &b))
	}
	return rec.Code, b
}

func TestAccessCheckNamesTheGrantingRule(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := newAuthzWorld(t, st, 1)

		// A group on the parent: direct on team, inherited on team/web.
		code, b := w.check(`{"groups":["team-ops"],"project":"team"}`)
		require.Equal(t, 200, code)
		assert.Equal(t, "runner", b.Role)
		require.NotNil(t, b.Source)
		assert.Equal(t, "direct", b.Source.Kind)
		assert.Equal(t, "team", b.Source.Project)
		assert.Equal(t, 0, b.Source.RuleIndex)
		assert.Contains(t, b.Source.Rule, "team-ops")

		_, b = w.check(`{"groups":["team-ops"],"project":"team/web"}`)
		assert.Equal(t, "runner", b.Role)
		require.NotNil(t, b.Source)
		assert.Equal(t, "inherited", b.Source.Kind)
		assert.Equal(t, "team", b.Source.Project, "names the ancestor whose rule granted it")
		assert.Equal(t, 4, b.Rules, "three rules on team and one on team/web")

		// The email counts through user.email; the rule's index is its place in its list.
		_, b = w.check(`{"groups":["team-viewers"],"project":"team"}`)
		assert.Equal(t, "viewer", b.Role)
		assert.Equal(t, 1, b.Source.RuleIndex)

		// A child rule does not climb to the parent.
		_, b = w.check(`{"groups":["team-leads"],"project":"team"}`)
		assert.Empty(t, b.Role)
		assert.Nil(t, b.Source)
		_, b = w.check(`{"groups":["team-leads"],"project":"team/web"}`)
		assert.Equal(t, "editor", b.Role)
		assert.Equal(t, "direct", b.Source.Kind)

		// A job's own rule, and a job whose project the identity cannot reach.
		_, b = w.check(`{"groups":["extra-runners"],"job_id":"` + w.opsJob + `"}`)
		assert.Equal(t, "runner", b.Role)
		require.NotNil(t, b.Source)
		assert.Equal(t, "job", b.Source.Kind)
		assert.Equal(t, "ops", b.Project)
		_, b = w.check(`{"groups":["extra-runners"],"project":"ops"}`)
		assert.Empty(t, b.Role, "the job rule does not reach the project")
	})
}

// The check and the enforcement are one evaluator: for every user, the role the
// check reports for their groups on a project is the one the API gives them.
func TestAccessCheckAgreesWithEnforcement(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := newAuthzWorld(t, st, 1)
		users := map[string][]string{
			"bob": {"team-ops"}, "carol": {"team-viewers"}, "dave": {"team-leads"},
			"erin": {"nobody"}, "frank": {"extra-runners"}, "gina": {"team-admins"},
		}
		for user, groups := range users {
			enforced := w.roles(user)
			for _, project := range []string{"team", "team/web", "ops"} {
				gs, _ := json.Marshal(groups)
				_, b := w.check(`{"groups":` + string(gs) + `,"project":"` + project + `"}`)
				assert.Equal(t, enforced[project], b.Role, "%s on %s", user, project)
			}
		}
	})
}

func TestAccessCheckIsAdminOnlyValidatedAndAudited(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := newAuthzWorld(t, st, 1)
		// Only an instance admin may ask.
		for _, user := range []string{"bob", "carol", "gina", "erin"} {
			assert.NotEqual(t, 200, w.as(user, "POST", "/api/v1/access/check", `{"groups":["x"],"project":"team"}`), user)
		}
		assert.Equal(t, 200, w.as("alice", "POST", "/api/v1/access/check", `{"groups":["team-ops"],"project":"team"}`))
		// Input is checked and a missing target is not found.
		code, _ := w.check(`{"groups":["x"]}`)
		assert.Equal(t, http.StatusUnprocessableEntity, code)
		code, _ = w.check(`{"project":"nope"}`)
		assert.Equal(t, http.StatusNotFound, code)
		code, _ = w.check(`{"job_id":"nope"}`)
		assert.Equal(t, http.StatusNotFound, code)

		// Audited with counts and the outcome, never the groups or the email.
		w.mustAdmin("POST", "/api/v1/access/check", `{"groups":["secret-group-name"],"email":"someone@example.org","project":"team"}`, 200)
		rec := w.admin("GET", "/api/v1/audit?action=access.check", "")
		require.Equal(t, 200, rec.Code, rec.Body.String())
		body := rec.Body.String()
		assert.Contains(t, body, "access.check")
		assert.NotContains(t, body, "secret-group-name")
		assert.NotContains(t, body, "someone@example.org")
		assert.NotContains(t, body, "team-ops", "no rule text either")
	})
}

func TestMyAccessShowsOwnRolesAndSources(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := newAuthzWorld(t, st, 1)
		mine := func(user string) []api.MyAccessItem {
			rec := w.do(0, "GET", "/api/v1/access/me", "", withCookies(cookieHeader(w.sess[user])))
			require.Equal(t, 200, rec.Code, rec.Body.String())
			var b struct{ Items []api.MyAccessItem }
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &b))
			return b.Items
		}
		bob := mine("bob")
		require.Len(t, bob, 2)
		assert.Equal(t, api.MyAccessItem{Project: "team", Role: "runner", Source: api.AccessSource{Kind: "direct", Project: "team", RuleIndex: 0}}, bob[0])
		assert.Equal(t, "inherited", bob[1].Source.Kind)
		assert.Equal(t, "team", bob[1].Source.Project)
		for _, it := range bob {
			assert.Empty(t, it.Source.Rule, "rule text is not shown to the user")
		}
		dave := mine("dave")
		require.Len(t, dave, 1)
		assert.Equal(t, "team/web", dave[0].Project)
		frank := mine("frank")
		require.Len(t, frank, 1)
		assert.Equal(t, api.MyAccessItem{JobID: w.opsJob, Name: "opsjob", Role: "runner", Source: api.AccessSource{Kind: "job", RuleIndex: 0}}, frank[0])
		assert.Empty(t, mine("erin"))
		assert.Len(t, mine("alice"), 4, "an instance admin: every project")
		assert.Equal(t, "instance_admin", mine("alice")[0].Source.Kind)
	})
}

// With login off there are no access rules to explain: the check says so instead
// of failing, and my-access lists the admin token's reach.
func TestAccessCheckWithoutLoginIsRefused(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		f := newSchedFixture(t, st)
		f.callJSON(t, "POST", "/api/v1/access/check", `{"groups":["x"],"project":"default"}`, http.StatusUnprocessableEntity, nil)
		var mine struct{ Items []api.MyAccessItem }
		f.callJSON(t, "GET", "/api/v1/access/me", "", 200, &mine)
		require.NotEmpty(t, mine.Items)
		assert.Equal(t, "instance_admin", mine.Items[0].Source.Kind)
	})
}

func TestAccessCheckOfAJobUsesTheJobsOwnProject(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := newAuthzWorld(t, st, 1)
		_, b := w.check(`{"groups":["extra-runners"],"job_id":"` + w.opsJob + `"}`)
		assert.Equal(t, "ops", b.Project)
		assert.Equal(t, "runner", b.Role)
		require.NotNil(t, b.Source)
		assert.Equal(t, "job", b.Source.Kind)
	})
}

// A job is checked in its own project; a body that names another project too is
// ambiguous, so it is refused instead of silently ignoring one of the two.
func TestAccessCheckRefusesAJobTogetherWithAProject(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := newAuthzWorld(t, st, 1)
		code, _ := w.check(`{"groups":["extra-runners"],"project":"team","job_id":"` + w.opsJob + `"}`)
		assert.Equal(t, http.StatusBadRequest, code)
	})
}

func TestAccessCheckOfAMissingProjectSaysSo(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := newAuthzWorld(t, st, 1)
		rec := w.admin("POST", "/api/v1/access/check", `{"project":"nope"}`)
		require.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "no project named", "a message, not only the status")
	})
}

// sharedRuleJobs makes n jobs in ops that carry a rule frank matches.
func sharedRuleJobs(t *testing.T, st store.Store, n int) {
	t.Helper()
	ctx := store.WithAudit(t.Context(), store.AuditMeta{Actor: "user:test"})
	ops, err := st.GetProjectByPath(ctx, "ops")
	require.NoError(t, err)
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		job, _, err := st.CreateJob(ctx, store.NewJob{ProjectID: ops.ID, Name: fmt.Sprintf("bulk-%04d", i), Spec: []byte(`{}`), Checksum: "c"}, now)
		require.NoError(t, err)
		_, err = st.SetJobAccessRules(ctx, job.ID, []store.AccessRule{{Role: "runner", Rule: "'extra-runners' in claims.groups"}}, 0, now)
		require.NoError(t, err)
	}
}

// More jobs with rules than the scan reads must be said, not read as "no roles".
func TestMyAccessSaysWhenTheJobRuleScanWasCut(t *testing.T) {
	forEachBackend(t, func(t *testing.T, st store.Store) {
		w := newAuthzWorld(t, st, 1)
		mine := func(user string) (items []api.MyAccessItem, truncated bool) {
			rec := w.do(0, "GET", "/api/v1/access/me", "", withCookies(cookieHeader(w.sess[user])))
			require.Equal(t, 200, rec.Code, rec.Body.String())
			var b struct {
				Items     []api.MyAccessItem
				Truncated bool
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &b))
			return b.Items, b.Truncated
		}
		_, truncated := mine("frank")
		assert.False(t, truncated, "a short scan is complete")
		sharedRuleJobs(t, st, 1000) // with the one rule job of the world: 1001
		items, truncated := mine("frank")
		assert.True(t, truncated)
		assert.Len(t, items, 1000, "the scan reads 1000 rule rows")
	})
}
