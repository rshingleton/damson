ADR-0078: Node sets
===================

:Status: Accepted
:Date: 2026-11-10

Roadmap ``ui-project-first-2``, slice R3. Builds on ADR-0012 (CEL), ADR-0043
(inventory nodes), ADR-0064 (access rules), ADR-0066 (project node scope),
ADR-0069 (jobs as code) and ADR-0076 (schedules and webhooks). Answers the
roadmap's open decisions D1 and D2.

Context
-------

``node_scope`` limits what a whole project may use, and only an instance admin
sees the inventory. The maintainer decided (2026-11-10) that nodes become a
project resource: a group owns a filtered set of nodes through the same access
rules that give it a role. ``node_scope`` stays the hard ceiling and sets only
narrow it. CEL is the only filter language (a regex is ``node.name.matches(...)``).
Nodes are visible per project and per caller. A hidden node answers as a missing
one. An instance admin sees everything. Sets give access only; a node can be in
many sets. Project users never write nodes.

Decision
--------

**Node sets are named objects (D2).** A set belongs to one project. Its name
follows the project segment grammar, is unique in that project and never
changes. Its reference is ``<project path>:<name>`` (``acme:prod-web``). A
project path never changes and never contains ``:``, so a reference is stable
and unambiguous. A set has a CEL ``filter`` over ``node``, the map that
selectors and ``node_scope`` use (``id``, ``name``, ``address``, ``tags``,
``attributes``, ``pool``, ``os``, ``transport``; never a credential, host key
or port), plus a ``description``. A rule may reference a set of its own project
or of any ancestor, so a set defined on ``acme`` is shared by every project
below it and edited in one place. ``admin`` on the owning project edits it, and
because roles inherit downward an ancestor admin may too.

**Rules reference sets.** A project or job access rule gains an optional
``node_sets``: a list of at most 8 references. Each reference must name an
existing set in the rule's project (for a job rule, the job's project) or one of
its ancestors. A rule that grants ``admin`` cannot carry sets (``422`` at
``body.rules[i].node_sets``), because a project admin could remove them by
editing the rules. A bad reference is ``422`` at ``body.rules[i].node_sets[j]``.
Rules stay JSON in ``projects.access_rules`` and ``job_access.rules``, and a
missing field means no sets, so stored rules need no data migration.

**What a caller can see.** For a caller and a project, plus a job when the job
is the target, ``NodeAccess`` evaluates every matching rule of the project, its
ancestors and the job. Unlike ``Explain``, it does not skip a rule that cannot
raise the role. The result is ``{All bool, Sets []ref}``:

- ``All`` is true when any matching rule has no ``node_sets``, when the
  effective role is ``admin``, and for an instance admin, the admin token, a
  service token or an anonymous caller.
- Otherwise ``Sets`` is the sorted union of the sets of the matching rules.

A node is visible when the project's effective ``node_scope`` allows it and, if
``All`` is false, at least one set in ``Sets`` matches it. A rule without sets
means every node within the ceiling, for two reasons:

- Existing rules keep today's behaviour.
- Grants stay additive, as roles are in ADR-0064: matching one more rule never
  removes access.

The consequence is that a group is limited only when every rule it matches
carries sets. A broad ``viewer`` rule without sets (for example ``true``) gives
its members every node. The R7 editor warns about this.

**Evaluation.** A set filter is compiled on write with the ``node_scope``
environment. It must be boolean and non-empty (write ``true`` for every node)
and at most 2048 bytes. It is cost-limited like other CEL. A compile error is
``422`` at ``body.filter`` with the compiler message, which describes the
expression and not user data. When a set fails to evaluate for a node, that set
does not match that node, and the other sets still apply. When a set no longer
compiles, or a reference points at a set that does not exist, the set matches
nothing. Both fail closed, and the log names the project and set, never the
expression or node data. A project holds at most 100 sets.

**Implementation note (R4).** The boolean result type of a filter is not checked
at compile time, because the environment types ``node`` as ``dyn``. A filter that
returns a non-boolean matches no node, as ``node_scope`` does. The migration
00025 holds only ``node_sets``; the ``runs``, ``schedules`` and ``webhooks``
columns arrive with R5.

**Caching.** ``inventory.ProjectScope`` loads the sets of the project chain in
the same 10-second fill as the chain and caches the compiled programs. A set
edit applies everywhere within about 10 seconds; the controller that took the
write clears its chain cache. R4 computes ``NodeAccess`` on every request from
the stored rules, so a rule edit applies at once; if that proves costly it may
join the grant cache (60 seconds, keyed ``session|project|job``, cleared on a
local rule write). Listing nodes costs one pass over the inventory, evaluating
the ceiling and at most the caller's sets for each node.

**Triggers and dispatch.** At trigger the caller's ``NodeAccess`` for the job is
written to ``runs.node_sets`` in the transaction that creates the run: ``NULL``
when ``All`` is true, otherwise a JSON array of references. Targets are frozen as
today, but only from visible nodes. A selector that matches a hidden node simply
does not pick it. ``Scope.Allows`` takes the run's set references, and
``InventoryNodes.admit`` checks, in this order:

1. The node exists.
2. The ceiling allows it.
3. One of the run's sets matches it, unless the run's ``node_sets`` is ``NULL``.
4. The node is enabled.

A failure in any of the first three is ``unknown node`` (permanent). The check
runs again for every step, multi-node plugins included, and for
``node.hostkey_observed``. Runs created before this change have ``NULL`` and
behave as today. The engine and the protocol do not change.

**Schedules and webhooks (D1).** The creator's sets are copied at save, not the
whole project scope. Copying the whole scope would let a limited editor create a
schedule that runs on nodes the editor cannot use. ``schedules.node_sets`` and
``webhooks.node_sets`` (``NULL`` means all nodes within the ceiling) take the
creator's ``NodeAccess`` for the job. A schedule patch that changes anything
other than ``paused`` copies the patcher's sets instead. Pause, resume and delete
leave them as they are. A webhook is immutable, so it keeps the sets it got at
create. Only admins create webhooks, so today a webhook always gets ``NULL``. A
fire passes the stored references to the run, and the actor stays
``system:scheduler`` or ``webhook:<id>``.

Only the references are stored, never the creator, so ADR-0076 stands. A
schedule keeps its sets after its creator loses a role or leaves a group. Edits
to those sets apply live, and an editor removes the access by patching or
deleting the schedule. A limited editor who patches a schedule that had all
nodes narrows it. That is visible, because reads return ``node_sets``.

**Hidden nodes.**

- ``GET /projects/{path}/nodes`` leaves hidden nodes out. A caller with no role
  on the project gets ``404``.
- At trigger a hidden node is never a target.
- At dispatch a hidden node is ``unknown node``.
- The instance ``/nodes`` API stays instance-admin only and lists every node.

Runs, run targets and logs are read by role as today and are not redacted for a
caller whose sets are narrower than the triggering caller's.

**Read API.** ``GET /projects/{path}/nodes`` needs ``viewer``. It is cursor-paged
by name (``limit`` defaults to 100, maximum 500) and returns
``{access: "all"|"sets", items, next_cursor}``. Each item has ``id``, ``name``,
``address``, ``tags``, ``attributes``, ``pool``, ``os``, ``transport``,
``enabled`` and ``visible_through`` (the caller's set references that match it,
empty when ``access`` is ``all``). Items never hold a credential ref, host key,
host key policy or port. Disabled nodes are listed with ``enabled: false``.

**Set routes.** Every route is strict (never anonymous) and needs ``admin`` on
``{path}``. Service tokens are never admin, so they never manage sets.

- ``GET /projects/{path}/node-sets`` lists the sets of the project and of its
  ancestors, each with ``ref``, ``project``, ``name``, ``description``,
  ``filter``, ``version`` and ``inherited``.
- ``POST /projects/{path}/node-sets`` takes ``{name, description, filter}`` and
  answers ``201``. A name already taken is ``409``.
- ``GET``, ``PUT`` and ``DELETE /projects/{path}/node-sets/{name}``. ``PUT``
  takes ``{description, filter, if_version}``. A stale version is ``409``.
  Deleting a set that is still referenced is ``409``, with a message that names
  what references it: access rules, job access rules, schedules or webhooks
  anywhere in the subtree. Runs do not block a delete.
- ``POST /projects/{path}/node-sets/preview`` takes ``{filter}`` and returns at
  most 200 nodes within the ceiling that the filter matches, plus
  ``truncated``. It uses the same node shape and is not audited.

A set of a managed project (ADR-0069) is refused like any other edit of that
project.

**Schema** (migration 00025, both dialects, forward only per ADR-0072):

- ``node_sets``:

  - ``id TEXT PRIMARY KEY``
  - ``project_id TEXT NOT NULL REFERENCES projects (id)``
  - ``name TEXT NOT NULL``
  - ``description TEXT NOT NULL DEFAULT ''``
  - ``filter TEXT NOT NULL``
  - ``version INTEGER NOT NULL DEFAULT 1``
  - ``created_at`` and ``updated_at``
  - ``UNIQUE (project_id, name)``, which also serves listing by project

- ``runs``, ``schedules`` and ``webhooks`` each gain ``node_sets TEXT`` (nullable).

An update is ``UPDATE node_sets SET ..., version = version + 1 WHERE id = $1 AND
version = $2``; zero rows is ``ErrNotFound`` or ``ErrStale``. A delete is a
compare-and-swap on the version, with the reference check in the same
transaction. A rule write and a set delete can still race and leave a dangling
reference. That is accepted: it grants nothing (fail closed) until a set of the
same name is created again, which then takes the reference up. The delete check
reads the rules of archived jobs too, so a restored job cannot find its rule
pointing at a recreated set. Deleting a project, which already requires it to have no children and
no jobs, deletes its sets in the same transaction and writes one
``node_set.delete`` event for each. JSON stays in Go and the SQL is in
``queries/shared``.

**Jobs as code.** ``project.json`` gains ``node_sets: [{name, description,
filter}]``, and its ``access_rules`` entries gain ``node_sets``. Export writes
both. Apply creates or updates sets first, then rules, then deletes sets, all in
its single transaction. Reference checks see the state planned by the same
apply. Exporting and then applying changes nothing.

**Audit.**

- ``node_set.create``, ``node_set.update`` and ``node_set.delete``: target type
  ``node_set``, target the set id, ``project_path`` the owning project. Details
  are ``ref``, ``set`` (the names of the changed fields) and ``source`` for
  apply. They never hold the filter.
- ``project.access_rules.update`` and ``job.access_rules.update`` details gain
  ``node_sets``, the sorted distinct references.
- ``schedule.create``, ``schedule.update``, ``webhook.create`` and
  ``run.trigger`` details gain ``node_sets`` (references; absent when the caller
  has every node within the ceiling).

Consequences
------------

- Project members can see nodes for the first time, but only within the ceiling
  and their sets, and never a credential or host key.
- A limit holds only when every rule a group matches carries sets.
- Webhooks, service tokens and project admins are never limited by sets.
  Service-account sets are future work.
- A set or rule edit can take 10 or 60 seconds to reach other controllers.
  Running runs pick up a set edit at their next dispatch.
- Run history can name nodes the reader cannot see.

Alternatives considered
-----------------------

- **A filter field on each rule.** Rejected. Two groups or a parent and child
  could not share one filter, and every copy would have to be edited separately.
- **A matching rule without sets grants no nodes.** Rejected. Every existing
  rule would lose its nodes.
- **Only rules that carry sets count, so a set narrows a caller who also matches
  a rule without sets.** Rejected. Matching more rules would remove access.
- **Full project scope for schedules and webhooks.** Rejected. A limited editor
  could run work on nodes they cannot use.
- **Storing the creator and evaluating at fire time.** Rejected for the same
  reason as in ADR-0076: a schedule belongs to the project.
- **Instance-global sets.** Rejected. Project admins could not manage them.

Tests
-----

Run on SQLite and Postgres:

- Store: set CRUD, compare-and-swap (stale ``409``), a duplicate name, a delete
  refused while referenced by each kind, project delete deleting its sets, and
  the migrated ``NULL`` columns.
- ``NodeAccess``: a rule without sets gives every node; a union of sets;
  ancestor and job rules; ``422`` for sets on an ``admin`` rule; instance admin,
  token and anonymous get every node; a dangling or broken set matches nothing;
  a set that fails for one node.
- API: bob sees only his set, a caller with no role gets ``404``, no item holds
  a credential or host key, the ceiling cuts a set, and the preview.
- Trigger and dispatch: a selector never picks a hidden node; a hidden literal
  ``node:`` is ``unknown node``; a run created before the migration behaves as
  today; a set edit reaches a running run's next step.
- Schedules and webhooks: the creator's sets are copied, a patch replaces them
  with the patcher's, and a fire passes them to the run.
- Audit details never hold a filter.
- Apply round-trip.

References
----------

- :doc:`0012-cel-as-the-single-expression-language`
- :doc:`0043-inventory-nodes`
- :doc:`0064-access-rules-and-authorization`
- :doc:`0066-project-node-scope`
- :doc:`0069-jobs-as-code`
- :doc:`0072-forward-only-migrations`
- :doc:`0076-project-scoped-schedules-and-webhooks`
- ARCHITECTURE sections 5.5, 9.1, 12.1, 12.3, 12.4, 13 and 14 (updated with R4 and R5)
