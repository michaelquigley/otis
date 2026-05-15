# Operations

## Supervisor

`otis serve` loads the resolved config, opens the state store, removes stale
scratch worktrees, starts the HTTP(S) API, and runs the scheduler. `otis serve
--once` runs one scheduler tick and waits for the queued work to complete.

The scheduler checks pass cadence and reviewer windows. Force-runs bypass cadence
and windows, but still respect global and per-reviewer concurrency caps.

## Dispatch

Each run captures the project `HEAD`, creates a temporary git worktree, resolves
the project and BoK scopes, builds a reviewer prompt, invokes the configured
reviewer, validates structured output, writes immutable run artifacts, updates
findings, renders `backlog.md`, and removes the scratch worktree.

Scope behavior:

- `full`: tracked project files with bounded inline content.
- `paths`: explicit paths, directories, and globs with bounded inline content.
- `recent`: first-parent commits whose committer timestamps fall in the pass
  window. Empty recent scopes write documented no-op run artifacts.

Reviewer adapters implemented today:

- `dummy`: deterministic file-backed reviewer for tests and demos.
- `codex`: `codex exec` with read-only sandboxing.
- `claude-code`: `claude -p` with read-only tools and plan permission mode.
- `pi`: `pi -p` JSON mode.

## State

State lives under `storage.state_dir`. Project mutations go through the state
package under a per-project lock. Important files:

- `projects/<project>/findings/`: denormalized finding JSON files.
- `projects/<project>/dispositions.jsonl`: append-only lifecycle events.
- `projects/<project>/backlog.md`: rendered open backlog.
- `projects/<project>/runs/<date>/<pass>/<time-seq>/`: immutable run artifacts,
  including `prompt.md`, `output.json`, `findings.json`, `report.md`, and
  `git-head.txt`.
- `projects/<project>/last-run.json`: cadence bookkeeping.
- `supervisor/events.jsonl`: supervisor lifecycle events.

Accepted, deferred, and rejected findings are removed from `backlog.md`, but they
remain available to later prompts as prior context with note history.

## Notifications

Mattermost posting is optional. Non-empty runs post one message to the project
channel from `project.notify.mattermost`, or `#otis-<project>` when not set. The
token is read from the environment named by `notification.mattermost.token_env`.

See `docs/example/mattermost-message.md` for the expected shape.

## Scenarios

Scenario one, vocabulary sweep: run `pass run` or let the scheduler enqueue a due
pass. Otis resolves the configured BoK slice, reviews the scoped code, writes run
artifacts, creates findings, renders `backlog.md`, and posts Mattermost when
configured.

Scenario two, human triage: run `otis accept`, `otis defer`, or `otis reject`
with a finding ID and optional note. The supervisor records the event, updates
the finding, and re-renders the backlog.

Scenario three, digital query: run the MCP bridge with a workstation client
config. MCP clients can list findings, fetch one finding, fetch a run report, and
change a finding disposition through the supervisor API.
