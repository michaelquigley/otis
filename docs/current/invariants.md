# Operational Invariants

These are the contracts that should survive implementation details moving around.

## BoK

Otis reads the body of knowledge directly from a local filesystem checkout at
pass time. There is no embedding index or cache to refresh.

BoK entries are markdown files under category subtrees such as `vocabulary/`,
`layering/`, and `cognitive-load/`. Root-level markdown is not treated as BoK
content. Project-scoped entries live under `projects/<name>/` and are only
included for that project.

Pass includes are deterministic:

- `vocabulary/` includes every entry under that directory.
- `vocabulary/lens-vs-view` includes that one markdown file.
- bare terms are rejected because they are reserved for future semantic search.

## Scheduling

Cadence controls how often a pass is eligible to fire. Windows control when the
reviewer is allowed to run. A `recent` pass must declare its own
`scope.project.window`; cadence is never used as a review window fallback.

Window times are interpreted in the supervisor host's local timezone. `24:00` is
accepted as the exclusive end of day, and cross-midnight windows such as
`22:00-06:00` wrap through midnight.

`last-run.json` is written at dispatch start, after semaphores are acquired and
before the reviewer runs. Failed runs still consume the cadence cycle. Otis is
not a retry engine; operators force-run when a failed run should be repeated.

Force-runs bypass cadence and window eligibility, but still obey per-reviewer and
global concurrency caps.

The dispatcher keeps one in-flight entry per `(project, pass)`. Scheduled
duplicates are skipped. Force-run duplicates fail with an in-flight error that
reports `queued` or `running` and includes the run ID once one has been allocated.

Scheduled runs re-check their reviewer window after waiting for semaphores. If
the window has closed, the run is dropped without writing `last-run.json`.

## Repository Reads

Otis never syncs or pulls supervised repositories. Sexton, or some other
operator-owned process, owns synchronization.

Each run captures the supervised repository's `HEAD`, creates a temporary git
worktree pinned to that SHA, reviews from the worktree, writes `git-head.txt`,
and removes the worktree at run end. Startup prunes project worktrees and removes
stale Otis scratch directories.

Uncommitted working-tree changes are out of scope. `full`, `paths`, and `recent`
all review the captured commit, not the live mutable checkout.

Recent scopes use first-parent history and committer timestamps. Empty recent
scopes write no-op run artifacts instead of invoking a reviewer.

## Prompt and Reviewers

Every prompt contains the role and goal, BoK slice, project context, scope
manifest, bounded inline scope content, prior findings, and output schema.

`top_findings` means best findings by impact on cognitive load. Returning fewer
or zero findings is valid when nothing stronger exists.

Prior findings include open, accepted, deferred, and rejected findings for the
same `(project, pass)`, including human note history. Reviewers should reuse an
existing ID when surfacing the same open issue. Accepted, deferred, and rejected
issues should not be resurfaced unless the code shows the basis has changed.

Reviewer adapters are read-only. Codex runs with a read-only sandbox,
Claude Code runs with read tools and plan permission mode, and Pi runs with its
read-only tool configuration. The dummy reviewer reads deterministic JSON for
tests and demos.

## Finding Identity

Canonical finding IDs have the form `<project>/<pass>/<NNNN>`, for example
`testproj/vocabulary-sweep/0001`.

Project and pass components use lowercase kebab IDs. The sequence is allocated
per `(project, pass)` and zero-padded to at least four digits.

Finding files map IDs with double dashes:
`testproj/vocabulary-sweep/0001` lives at
`projects/testproj/findings/testproj--vocabulary-sweep--0001.json`.

Reviewer output is lean: optional `id`, `severity`, `title`, `location`,
`bok_refs`, `description`, and `suggested_fix`. The schema rejects additional
fields; project, pass, reviewer, run IDs, timestamps, and disposition are
dispatcher-owned.

Reviewer-supplied IDs are honored only when they match an ID from the prior
findings context. A match updates only `last_run_id` and appends
`finding_reobserved`; it does not reset disposition. Missing or unknown IDs
allocate a fresh open finding and append `finding_created`.

## State and Audit

Every project mutation goes through the state package under a per-project lock:
finding ID allocation, finding JSON writes, disposition events, backlog render,
run ID allocation, run artifact writes, and `last-run.json` updates.

Writes use atomic temp-file rename. Disposition and supervisor event logs are
append-only JSONL.

`backlog.md` is a live rendered view of open findings. Accepted, deferred, and
rejected findings disappear from the backlog but remain available as prior
context.

Run artifacts are immutable audit snapshots. `report.md` and `findings.json` are
written at run completion and served later as stored files. They are not
re-rendered when a finding's disposition changes.

The supervisor is the only state writer. CLI commands and MCP tools mutate state
through the supervisor API, not by opening the state directory directly.
