---
title: Follow-ups from the audit-bugfix-sweep audit
type: task
status: open
source: audit-bugfix-sweep
---

## Context

Should and Nice findings of the audit-bugfix-sweep audit, plus defects the build surfaced.

## Problem

- Postgres never ran for this change (no Docker): run `make test-pg`, the audit `hide_system` and actor-name storetest cases, and EXPLAIN of the tree counts query at 100 000 runs; record the plan in ADR-0075.
- Tree counts cost grows with all failed runs ever; a `(status, finished_at)` index would bound it (migration on both databases, needs architect review).
- Audit actor-name filter: `lower()` is ASCII-only on SQLite and Unicode-aware on Postgres; `audit.sql` and `audit.rst` should say so or fold case in Go.
- `SchemaForm.svelte` sets `ON_INPUT` validation for every form; make it opt-in for RunDialog.
- `web/src/lib/jobs.ts` and `agents.ts` have duplicate cursor-paging loops; `isConflict`, `isNotFound`, `problemMessage`, `clockOffset` live in `agents.ts` (move to a neutral module); `STALE_AFTER_MS` copies the server window.
- `/access/me` `truncated`, `check-access` 400, `/audit` 400 and scoped-token `GET /runs?project=` are not all in `http-api.rst` and `access-rules.rst`.
- `internal/api/authz_test.go` (597 lines) and `web/e2e/runs.test.ts` (447) are over the ~400-line convention; `accesscheck.go` `checkAccess` keeps growing.
- `JobsView` is keyed per path so its `more()` scope-change race has no observable test; `triggers.test.ts` and `rundialog.test.ts` both create job `dialog-e2e` and collide when run in order.
- `VersionHistory.svelte` reads "identical content" when both checksums are missing; `admin/audit/+page.svelte` sets `applied` before a fetch that may fail; `CountsFold` has no direct unit test; the lazy-rollup test is API-level, not a storetest.
- Home "Shared with me" and the JobView edit link still use the `/jobs/<id>` redirect (no project field, no project-first edit route).
- `docs/issues/home-page-follow-ups.md` was deleted before this plan with two bullets unfixed: the `visibleRuns` 20-page scan and "N recent failed counts only the newest 25".

## Acceptance

- Each point fixed or dropped with a reason.
