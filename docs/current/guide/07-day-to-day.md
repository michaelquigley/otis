# 7. Day-to-Day

Once Otis is running, the work is triage: read what the supervisor
surfaced, decide what to do with each finding, and let prior context
carry forward into the next pass. This chapter covers the routine flow,
the audit trail it produces, and the MCP bridge for working through an
agent client.

The HTTP, CLI, and MCP surfaces themselves are listed in
[../api.md](../api.md); the contracts behind triage live in
[../invariants.md](../invariants.md) and
[../operations.md](../operations.md).

## The Workstation CLI

All workstation commands take `--client-config /path/to/client.yaml` and
talk to the supervisor API. They never open the state directory directly.
The same `otis` binary that runs the supervisor also runs the
workstation commands; only the flags differ.

A typical session:

```bash
otis --client-config ~/.config/otis/client.yaml projects list
otis --client-config ~/.config/otis/client.yaml passes list --project alpha
otis --client-config ~/.config/otis/client.yaml findings list --project alpha --open
```

`findings list` accepts `--pass <name>` and `--disposition
<open|accepted|deferred|rejected>` filters in addition to `--open` (a
shortcut for `--disposition open`).

Drill into a finding:

```bash
otis --client-config ~/.config/otis/client.yaml \
  findings show alpha/vocabulary-sweep/0001
```

And the full reviewer report from a specific run:

```bash
otis --client-config ~/.config/otis/client.yaml \
  report show alpha/vocabulary-sweep/2026-05-15/142233Z-001
```

The run ID format is `<project>/<pass>/<YYYY-MM-DD>/<HHMMSSZ-NNN>`.

## Triage

Every finding gets one of three terminal dispositions:

```bash
otis --client-config ~/.config/otis/client.yaml \
  accept alpha/vocabulary-sweep/0001 --note "fixed in #482"

otis --client-config ~/.config/otis/client.yaml \
  defer alpha/vocabulary-sweep/0003 --note "blocked on platform v3"

otis --client-config ~/.config/otis/client.yaml \
  reject alpha/vocabulary-sweep/0005 --note "false positive — see ADR-014"
```

What happens on the supervisor:

1. The state package takes the per-project lock.
2. The finding JSON is rewritten with the new disposition and your note.
3. A `finding_accepted` / `finding_deferred` / `finding_rejected` event
   is appended to `projects/<project>/dispositions.jsonl`.
4. `backlog.md` is re-rendered. Accepted, deferred, and rejected
   findings disappear from it.

The finding file itself never disappears. It stays in
`projects/<project>/findings/`, the disposition log keeps the event
history, and the next pass sees the disposition plus your note in its
prior-context section. That is what stops the reviewer from re-raising
issues you have already triaged — and what gives a future reviewer a
real reason to surface the same issue again if the basis has changed.

## Backlog and Run Artifacts

`projects/<project>/backlog.md` is the rendered view of open findings,
nothing else. It is the right thing to look at in a quick "what's open
on this project" sweep. It is regenerated on every state mutation, so
treat it as a view, not a source of truth.

Each run writes an immutable artifact directory:

```
projects/<project>/runs/<date>/<pass>/<time-seq>/
├── prompt.md       # the full prompt sent to the reviewer
├── output.json     # the reviewer's raw JSON response
├── findings.json   # normalized findings for this run
├── report.md       # human-readable run report
└── git-head.txt    # the commit SHA the worktree was pinned to
```

These are the audit trail. They are written once at run completion and
are not re-rendered when a finding's disposition changes later. If you
need to know exactly what a reviewer saw at the time, read `prompt.md`
and the corresponding `git-head.txt`.

## Force-Runs

The scheduler honors cadence and windows; force-runs do not. Force a
specific pass:

```bash
otis --client-config ~/.config/otis/client.yaml \
  pass run alpha/vocabulary-sweep
```

Use force-runs when:

- You changed a BoK entry and want to see it applied immediately.
- The supervised repo had a meaningful change that should not wait for
  the next cadence tick.
- A scheduled run failed; force-runs are the closest thing to a retry
  (Otis itself is not a retry engine — see
  [../invariants.md](../invariants.md)).

Force-runs still respect global and per-reviewer concurrency caps, and
the dispatcher rejects a second force-run for a `(project, pass)` that
already has an in-flight entry. The error includes the existing run ID
once one has been allocated.

## MCP Bridge

`otis mcp` runs a stdio MCP bridge that proxies through the same
workstation client config. Wire it into your MCP client with
[`docs/example/mcp.json`](../../example/mcp.json):

```json
{
  "mcpServers": {
    "otis": {
      "command": "otis",
      "args": ["mcp"]
    }
  }
}
```

If your `client.yaml` is not at the default path, supply
`--client-config` in the `args` array.

The bridge exposes four tools today:

- `otis_list_findings` — same filters as `findings list`.
- `otis_get_finding` — fetch a single finding by canonical ID.
- `otis_get_report` — fetch a run report.
- `otis_disposition_finding` — accept, defer, or reject with a note.

Operator-action tools (force-run a pass, reload config) are
intentionally not exposed today — see [../deferred.md](../deferred.md).

## A Routine Workflow

A typical day:

1. Skim `backlog.md` for each project, or use
   `otis findings list --project <p> --open` in a terminal.
2. For each finding, open `report show` (or the MCP equivalent) and
   read the reviewer's reasoning and suggested fix.
3. Triage: `accept` with a fix reference, `defer` with a blocker, or
   `reject` with a justification. Every triage gets a note — your
   future self and the next reviewer will both read it.
4. When you change something the BoK should know about, edit the BoK
   repo, deploy it, restart the supervisor (config reload is deferred),
   and force-run the relevant pass to see the change land.
5. Once a quarter, look back at `dispositions.jsonl` for any project to
   see whether the same finding keeps coming back. Persistent
   reobservations are a signal either to fix the code or to write a
   sharper BoK entry that explains why this stance is the right one.

## What Otis Does Not Do Today

A short reminder so you set expectations correctly with whoever uses
your supervisor:

- No proposed diffs, no autonomous remediation. Reviewers are read-only.
- No event-triggered passes. Cadence + windows + force-runs are it.
- No continuous-mode passes that restart on completion.
- No PR/branch/tag scopes. Otis reviews the current `HEAD` of the
  configured project working tree.
- No live config reload. Restart the supervisor after config changes.
- No content-hash dedupe for findings; identity flows through prior
  context and reviewer-supplied existing IDs.

The full list is in [../deferred.md](../deferred.md).

---

That is the end of the guide. The reference layer in `docs/current/`
covers anything beyond this; the demo in
[`docs/example/`](../../example/) is the safest sandbox to experiment
with configuration or BoK changes before applying them to a real
project.
