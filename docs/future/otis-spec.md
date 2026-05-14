# Otis — Spec

This document describes the design of otis, the continuous code-quality agent. The companion vision document at `software/otis/otis-vision.md` in the grimoire frames the problem and thesis; this document covers the shape of the system itself — the components, protocols, data, configuration, and surfaces.

The explicit target of this spec is the *minimal harness*: get something running that we can build the body of knowledge against iteratively. Higher-permission operating modes, multi-reviewer panels, autonomous remediation, and similar elaborations are deferred. Each deferral is named, with reasoning, in the section near the end of this document.

## What This Spec Covers

Otis is a continuous code-quality agent that watches a codebase against a body of architectural guidance and surfaces findings for human triage. The body of knowledge (BoK) is the crown jewel of the project; otis itself is the access pattern — the harness that operationalizes the BoK against codebases on a heartbeat.

This spec covers both the harness and the structural shape of the BoK. The BoK's actual content is harvested over time and lives outside this spec's scope. An otis distribution ships with a limited example BoK so the system is demonstrable; the real, calibrated BoK stays inside the practice.

## Shape

Otis is a singleton service running on the HQ network. It dispatches fresh-context review passes against supervised repositories, accumulates findings to a durable backlog, and exposes those findings to humans and digital collaborators through a thin set of network surfaces.

```mermaid
flowchart LR
    subgraph supervisor["otis supervisor (HQ network)"]
        Scheduler[scheduler]
        Dispatcher[reviewer dispatcher]
        API[HTTPS + MCP listener]
        State[(state directory)]
    end

    BoK[(otis-bok)]
    Repos[(project repos)]
    Sexton[sexton]

    Sexton --> BoK
    Sexton --> Repos
    BoK --> Scheduler
    Repos --> Scheduler
    Scheduler --> Dispatcher
    Dispatcher --> ClaudeCode[claude code]
    Dispatcher --> Codex[codex]
    Dispatcher --> Pi[pi]
    ClaudeCode --> State
    Codex --> State
    Pi --> State
    State --> API
    API <--> Workstation[otis cli]
    API <--> Digital[digital via MCP]
    API --> Mattermost[mattermost]
```

Five elements hang off the supervisor.

The **scheduler** tracks which passes are due based on each pass's cadence and the global scheduling windows, and queues them for dispatch.

The **reviewer dispatcher** invokes a reviewer subprocess (claude code, codex, or pi) in fresh context per pass, with a constructed prompt and a JSON output schema. Concurrency caps per reviewer plus a global cap prevent the supervisor from saturating the host or burning unbounded tokens.

The **HTTPS + MCP listener** exposes the supervisor's state and operations to two audiences. Humans interact through a REST API consumed by an `otis` CLI on each workstation. Digitals interact through an MCP server whose tools mirror the REST endpoints. Bearer-token authentication for both.

The **state directory** persists everything that matters across supervisor restarts: pass-run history, finding objects, dispositions, rendered backlogs, prompt snapshots, raw reviewer outputs, supervisor lifecycle events.

The **body of knowledge** is a separate git repository, sexton-synced. The supervisor maintains an embedding index over it and resolves each pass's declared concerns to a slice of entries via semantic search.

The supervisor itself is a deterministic Go process. The cognitive work happens inside the reviewer subprocesses; the supervisor doesn't accumulate context or make judgment calls. It schedules, dispatches, persists, and routes — normal operational machinery you can reason about with standard tooling.

## The Body of Knowledge

The BoK is a focused, private git repository (anticipated at `git.hq.quigley.com/products/otis-bok`) synchronized by sexton the same way the grimoire is. The supervisor reads from a local checkout that sexton keeps current.

Entries are markdown files, lore-shaped. Light frontmatter for indexing; free-form prose for the guidance itself. The frontmatter fields otis cares about are `title` and `tags` (free-form, for human navigation and as soft anchors for semantic search). **Scope is encoded by location, not frontmatter** — an entry at the top of the central BoK applies to all projects; an entry under `projects/<name>/` in the central BoK applies only to `<name>`. The body is conventional but not enforced: sections like `## guidance`, `## why`, `## examples`, and `## related` recur because they earn their place, but the reviewer reads the whole entry and understands the guidance regardless of structure.

```markdown
---
title: lens vs view vocabulary preference
tags: [vocabulary, naming]
created: 2026-05-13
---

# lens vs view vocabulary preference

Across the codebases, "lens" is the established term for a perspectival
or filtered view of data — see HiveLens, libraryLens, etc. Some
subsystems have drifted into using "view" interchangeably with the same
meaning.

## guidance

Prefer "lens" over "view" in new code for perspectival data surfaces.
When reviewing existing code, flag places where "view" appears in
contexts where "lens" would match the established convention. Don't
flag "view" when it's being used in its more general sense — only the
lens-as-perspectival-surface sense.

## why

Vocabulary consistency reduces cognitive load when navigating across
modules. The cost of standardization is low; the benefit compounds as
the codebase grows.
```

The BoK repository layout is conventional:

```
otis-bok/
  vocabulary/
  layering/
  cognitive-load/
  projects/
    baab/
    lore/
  README.md
```

Top-level directories are conceptual buckets — vocabulary, layering, cognitive-load, and so on. The actual set evolves as the BoK grows; this list is illustrative. The `projects/<name>/` subtree is where project-bound entries live when they're fine in the central repo.

In the minimal harness the BoK lives entirely in the central repository — there is no in-project BoK layer. Project-specific augmentation as an in-repo `<project>/.otis/bok/` directory is deferred (see "Deferred"); when needed, project-bound guidance goes under `projects/<name>/` in the central BoK and gets the same harvest and review treatment as everything else. Convention is preferred over configuration throughout otis: the BoK lives at one known path, scoped by location.

The supervisor maintains an embedding index over the central BoK. At pass time, each pass's `concerns` list resolves to a slice of top-K BoK entries via semantic similarity, **filtered to the entries in scope for the pass's project**. The scope filter is location-based: for project `P`, search results include entries that live outside the `projects/` subtree (general guidance) and entries under `projects/P/` (P's curated subtree). Entries under `projects/X/` for any `X ≠ P` are excluded. No frontmatter field is consulted; location is the contract. Findings' `bok_refs` point at the specific entries that contributed, so a human triaging a finding can read exactly which guidance the reviewer was working from.

The indexing approach is borrowed from lore. The two implementations are deliberately duplicated for now; the shared abstraction is expected to be extracted into a library once both have stabilized, but that extraction is not part of this spec. The planning agent has access to lore's code and will firm up the implementation-level details.

**Index refresh lifecycle.** The supervisor runs an incremental checksum-based BoK sync (the same mechanism lore uses) at two moments: at `otis serve` startup before the scheduler loop begins, and at the top of every scheduler tick before due-list construction. Incremental sync is cheap when nothing has changed (checksum compare, no embedding work), so a per-tick refresh is affordable. New BoK entries that sexton lands on disk are visible to the next pass within one scheduler-tick interval. Force-runs flow through the same dispatch entry point and therefore see whatever the latest tick synced. `otis bok index` remains available as a manual rebuild for operators who want to force a full re-embed.

## The Pass

A pass is the unit of work. Each pass is a single fresh-context invocation of a reviewer against a configured slice of code and a configured slice of BoK, producing a structured set of findings.

### Scope

Each pass declares scope along two axes — project scope (what code to look at) and BoK scope (what guidance to apply).

Project scope has three types in the minimal harness:

- `full` — the entire project tree.
- `paths` — an explicit list of paths or globs within the project. Each entry resolves by one of three rules: if the entry resolves to a directory, it is treated as a tree root and expanded recursively using the same tracked/ignored-file rules as `full` (`.gitignore`-aware via `git ls-files`); if the entry contains glob meta-characters (`*`, `?`, `[`), it is expanded as a standard glob; otherwise it is treated as a single literal file path. The three rules are checked in that order so unambiguous directory inputs do not need glob suffixes.
- `recent` — code touched within a time window, resolved via git: the set of files modified by commits reachable from `HEAD` whose committer timestamp falls within `now - window` to `now`. The `window` is taken **only** from the pass's own `scope.project.window` field; it is required whenever `type: recent` is set and the config loader rejects a `recent` scope that omits it. Cadence is never used as a fallback for the review window — cadence controls firing frequency, `window` controls what each firing reviews. Committer date (not author date) is the anchor — rebases preserve author date but reset committer date, and the supervisor cares about when the change landed on this branch, not when it was first written. Uncommitted working-tree changes are not in scope; if a project wants those reviewed, it points a `paths` pass at them.

BoK scope is expressed as a list of *concerns* — short identifiers that resolve to BoK entries via semantic search. `concerns: [vocabulary, naming]` retrieves the top-K BoK entries semantically related to the combined-concern query (concerns concatenated into one search vector, not run separately and unioned). K is configurable per pass via `scope.bok.top_k`; when the pass omits it, the supervisor's `bok.search.default_top_k` (global config) supplies the default. Different passes legitimately want different K — a narrow vocabulary sweep can run with K=5 and stay precise; a broad architectural pass may want K=15 to surface adjacent guidance. Because resolution is search-based rather than tag-based, the BoK can grow without project configs needing to change.

### Cadence and Windows

Passes don't declare wall-clock schedules. They declare *cadence* — how often the pass should run. The supervisor decides when to actually fire each pass, fitting cadences into global scheduling windows defined in the global config.

Cadence is a duration: `cadence: 24h`, `cadence: 6h`, `cadence: 1w`. Shorthand aliases (`daily`, `weekly`, `hourly`) parse to the obvious durations.

Windows are named time ranges declared in the global config — `overnight`, `working-hours`, `anytime`, and any custom names the operator wants. Each reviewer is assigned a window in the global config; passes inherit that window through their reviewer. A daily pass on a reviewer assigned to the `overnight` window fires once per night within that window; a 6-hourly pass on a reviewer assigned to `anytime` fires four times a day distributed across all hours. Window endpoints are parsed as `HH:MM` with `24:00` accepted as an exclusive end-of-day sentinel so a full-day window writes cleanly as `00:00-24:00`. Windows whose `end` is earlier than their `start` (`22:00-06:00`) wrap past midnight — the supervisor evaluates membership as `now ≥ start || now < end` rather than the in-range check used when `end > start`.

The supervisor's scheduling job is: keep a running view of which passes are due (`now - last_run >= cadence`), pick from due passes while respecting per-reviewer and global concurrency caps, and prefer not to bunch up firings of many cheap passes at startup.

The `last_run` timestamp is written **at dispatch start** — the moment the dispatcher acquires its semaphore slot and is about to launch the reviewer subprocess — not at completion. This prevents a long-running pass from staying "due" across subsequent scheduler ticks and getting enqueued multiple times. The consequence: failed or crashed runs still consume the cadence cycle. The supervisor is not a retry engine; if a failed run needs to be redone, the operator force-runs it. Cadence is about firing rhythm, not delivery guarantees.

Force-runs (`otis pass run <project>/<pass>` and the matching REST endpoint) bypass cadence and window eligibility — they fire immediately regardless of when the pass last ran or whether the reviewer's window is currently open. They still respect per-reviewer and global concurrency caps; a force-run waits in the same queue as any scheduled run when caps are saturated. Force-runs also update `last_run.json` at dispatch start, so a force-run inside a cadence window shifts the next scheduled firing forward by the cadence.

The dispatcher maintains an in-memory `in_flight` map keyed by `(project, pass)`. An entry is added when the pass is accepted for enqueue and removed when the reviewer goroutine returns (success, failure, or panic). Each entry carries a `state` (`queued` while waiting on the reviewer/global semaphores, `running` once the run ID is allocated at dispatch start) and the `run_id` once allocated. Both scheduled enqueues and force-runs check the map before acquiring a semaphore: scheduled enqueues are skipped silently if the pass is already in flight (no `last_run` update, no second goroutine); force-runs are rejected with a 409-shaped error whose body carries `{state: "queued" | "running", run_id: <id or null>}` — `run_id` is `null` while the colliding pass is still queued, populated once it has been dispatched. This keeps the API contract honest about what's knowable at the moment of collision. A long-running pass blocks both its own next scheduled firing and any overlapping force-run until the goroutine completes — at most one active run per `(project, pass)`, so the prior-findings context the reviewer sees can never be observed by two concurrent runs of the same pass. `last_run.json` is still written at dispatch start so failed and crashed runs consume their cadence cycle. The map lives only in process memory; the supervisor is a singleton so no on-disk reservation is required.

### Top-N Discipline

Each pass declares `top_findings: N` — the maximum number of findings the reviewer should return. The default at the project level is low (3 is the suggested default), biasing the whole system toward terse high-signal output.

The semantics are *best N*, not *up to N*. The reviewer's prompt instructs it to rank candidate findings by impact on cognitive load and return only the top N. Returning fewer findings is correct when nothing stronger exists; zero findings is a valid output when the codebase is clean for that pass's concerns.

This discipline is enforced in the global reviewer prompt template rather than per-pass, so all reviewers across all passes operate under the same instruction. The vision doc's calibration target — *reduce cognitive load*, not *find smells* — is what `top_findings` operationalizes.

### The Reviewer Interface

Each reviewer (claude code, codex, pi) implements a small interface: accept an assembled prompt and a JSON output schema; run in fresh, read-only context; return structured findings.

The dispatcher constructs the prompt with a fixed shape:

1. **Role and goal.** Otis-reviewer for pass `X` on project `Y`; the goal is to surface the top N findings that would reduce cognitive load.
2. **BoK slice.** The semantically-resolved entries for this pass's concerns.
3. **Project context.** Non-BoK project metadata that orients the reviewer: project name, description, primary language, anything declared at the project level the reviewer needs to read the code in context. BoK guidance does not enter via this section — it enters only through the resolved BoK slice in item 2, so concerns/top_k actually gate what guidance the reviewer sees.
4. **Scope content.** Every prompt carries a **file manifest** (the resolved in-scope paths relative to the project root) and the project's git HEAD at pass-start. Inline content depends on scope type and a per-supervisor byte budget:
    - **`recent`** — inline the diff that captures the changes that landed in the window. The boundary is derived from the time window precisely: walk **first-parent only** from `HEAD` (`git log --first-parent`) and select commits whose committer timestamp falls in `[now - window, now]`. If the selection is empty the recent scope is empty (no diff, no files, the pass is a no-op for that tick). Otherwise identify the oldest selected first-parent commit `C_oldest`. If `C_oldest` has a first parent, emit `git diff C_oldest^1..HEAD -- <file>` per in-scope file. If `C_oldest` is the repository's root commit (no parent), diff against the empty-tree object (`4b825dc642cb6eb9a060e54bf8d69288fbee4904`). First-parent tracing is essential: walking all reachable commits would let merges pull in changes that landed via side branches outside the current branch's window.
    - **`paths` / `full`** — inline file contents up to a per-file byte cap (default 8 KiB) and a total scope-content cap (default 256 KiB), both configurable in the global config (`prompt.per_file_bytes`, `prompt.total_scope_bytes`). Files above the per-file cap appear in the manifest with a `truncated: N→K bytes` marker.
    Reviewer adapters operate with read-only filesystem access (see permission discipline), so when the reviewer needs to see more than what was inlined it reads files from the manifest's paths directly. The manifest-plus-bounded-inline shape is the same across `full`, `paths`, and `recent` so the reviewer's behavior doesn't fork on scope type — only what's inlined changes.
5. **Prior findings context.** Every non-archived finding for the same `(project, pass)` pair — `open`, `accepted`, `deferred`, and `rejected` alike — with description, location, current disposition, and a short **note history**. The note history is the ordered list of human notes attached to disposition-change events, each tagged with the disposition it accompanied (e.g., `deferred: "needs design review first"` → `accepted: "addressed in PR #42"`). This lets the reviewer see why a finding moved through the states it did, not just where it ended up — which matters when a finding bounced through `deferred` before being `accepted`, or when an `accepted` note explains exactly which commit was supposed to fix it. The reviewer is instructed how to treat each state: for `open`, reference the existing ID when surfacing the same issue; for `accepted`, the fix is planned or in flight and the reviewer should not re-surface unless the code shows the fix did not land; for `deferred` or `rejected`, the issue is known and should not be re-surfaced. This is the substrate that makes cross-run identity work — if only open findings were shown, an accepted finding could be silently duplicated with a new ID the next time the pass ran.
6. **Output schema and budget.** The finding JSON schema and the `top_findings` cap.

The dispatcher then invokes the reviewer in print mode:

- **Claude Code**: `claude -p ... --output-format json --json-schema ... --bare --allowedTools <read-only> --permission-mode plan`
- **Codex**: `codex exec` with read-only sandbox flags and the schema.
- **Pi**: `pi -p ... --mode json` configured with read-only tool access via permission-gate extension or sandboxing.

Each reviewer adapter enforces the read-only invocation discipline for its harness. The minimal harness operates at the conservative end of the permission gradient (see Deferred): reviewers can read code but never write.

### The Finding

A finding is the unit of output. Two related schemas govern its lifecycle: the **reviewer output schema** (what reviewer subprocesses must produce) and the **persisted Finding schema** (what otis stores, returns over the API, and renders into reports). The dispatcher normalizes the first into the second.

#### Reviewer Output Schema

Lean. The reviewer authors only the fields it is in a position to know. The dispatcher owns identity, scope, and lifecycle.

```json
{
  "id": "baab/vocabulary-sweep/0042",
  "severity": "medium",
  "title": "inconsistent use of \"lens\" vs \"view\" in hive subsystem",
  "location": {
    "file": "internal/hive/library.go",
    "lines": "42-78"
  },
  "bok_refs": ["vocabulary/lens-vs-view"],
  "description": "The hive subsystem uses both \"lens\" and \"view\" interchangeably for the same conceptual surface...",
  "suggested_fix": "Standardize on \"lens\" throughout the hive subsystem."
}
```

`id` is optional and only honored when it matches an entry from the prior-findings context the dispatcher passed in (see Reviewer Interface item 5). Any other `id` is discarded; the dispatcher allocates a fresh one. Reviewers should not echo back `project`, `pass`, `reviewer`, run IDs, timestamps, or disposition — those are dispatcher-owned and any reviewer-supplied values are silently dropped during normalization.

#### Persisted Finding Schema

Rich. What lives at `state/projects/<project>/findings/<file>.json`, what the REST API returns, what `report.md` renders against.

```json
{
  "id": "baab/vocabulary-sweep/0042",
  "project": "baab",
  "pass": "vocabulary-sweep",
  "reviewer": "pi",
  "first_run_id": "baab/vocabulary-sweep/2026-05-13/060423Z-001",
  "last_run_id": "baab/vocabulary-sweep/2026-05-13/120511Z-001",
  "created_at": "2026-05-13T06:04:23Z",
  "severity": "medium",
  "title": "inconsistent use of \"lens\" vs \"view\" in hive subsystem",
  "location": {
    "file": "internal/hive/library.go",
    "lines": "42-78"
  },
  "bok_refs": ["vocabulary/lens-vs-view"],
  "description": "The hive subsystem uses both \"lens\" and \"view\" interchangeably for the same conceptual surface...",
  "suggested_fix": "Standardize on \"lens\" throughout the hive subsystem.",
  "disposition": "open"
}
```

The dispatcher's normalization step takes the reviewer output, attaches `project` / `pass` / `reviewer` / `first_run_id` / `last_run_id` / `created_at` / `disposition`, resolves the canonical `id` (reusing prior-findings match, or allocating fresh), and writes the persisted Finding under the per-project lock. Reports, mattermost messages, and API responses all read the persisted Finding — never the raw reviewer output. The raw `output.json` is preserved under `runs/.../` as an audit artifact only.

### Finding ID

The finding ID has one canonical form and explicit mappings to file paths and URLs. No alternate display forms.

- **Canonical form** (used in JSON, in CLI arguments, in mattermost messages, in `bok_refs`-style references): `<project>/<pass-slug>/<NNNN>` — lowercase, slash-delimited, zero-padded four-digit sequence. Example: `baab/vocabulary-sweep/0042`. The sequence is monotonic per `(project, pass-slug)` pair, allocated by the dispatcher inside the per-project critical section.
- **Filename mapping**: components are joined with a double-dash separator. `baab/vocabulary-sweep/0042` lives at `state/projects/baab/findings/baab--vocabulary-sweep--0042.json`. The mapping is reversible by splitting on `--`. Project names and pass slugs are validated against the canonical grammar `^[a-z0-9]+(?:-[a-z0-9]+)*$` — lowercase kebab ASCII, no `--`, no leading or trailing `-`, no empty components, no `/`, no `.`, no uppercase, no whitespace. The sequence component is `[0-9]{4,}` (zero-padded, four digits minimum). Splitting a filename on `--` therefore yields exactly three components and the grammar prevents accidental noncanonical state in filenames, URLs, and BoK project scoping.
- **REST encoding**: each component is its own path segment. `GET /api/v1/projects/{project}/findings/{pass}/{seq}` rather than `GET /findings/{id}`. This keeps the URL unambiguous and avoids percent-encoding slashes.
- **Ownership**: the dispatcher assigns IDs for new findings. The reviewer is given the prior-findings context (see Reviewer Interface) and may reference existing IDs from that context when surfacing the same issue. Any `id` field in reviewer output that does not match an existing ID from the prior-findings context is discarded — the dispatcher allocates a fresh ID instead. This makes the reviewer schema-permissive but semantically authoritative only for existing IDs.

The CLI accepts the canonical form everywhere a finding is named — `otis accept baab/vocabulary-sweep/0042 --note "..."`. Mattermost messages display the canonical form. No all-caps or hyphenated display variant exists in the protocol.

`reviewer` records which reviewer surfaced the finding. Useful for triage and for understanding finding character, since each reviewer's findings tend to read differently and the human's calibration develops over time.

`severity` is `low | medium | high` in the minimal harness.

`location` is file plus line range. AST-level anchors (function, type) are deferred.

`bok_refs` is the list of BoK entries that contributed to the reviewer's understanding. The reviewer is asked to populate this honestly — entries that actually shaped the finding, not all entries it was shown.

`description` and `suggested_fix` are prose from the reviewer, kept as distinct fields. The cost-calibration discipline (don't over-engineer fixes) is easier to enforce when the fix suggestion is in its own slot the reviewer can be instructed to keep minimal. The minimal harness operates with prose suggested fixes rather than structured diffs; this sits between pure level 1 and level 2 of the permission gradient.

`disposition` is `open | accepted | deferred | rejected`. The value on the finding object is a denormalized cache of the latest event in `dispositions.jsonl` (see State and Audit); API consumers read current state directly without replaying the event log.

### Cross-Run Identity

When a pass runs again and the same underlying issue is still present, the existing finding's ID gets reused rather than a new finding created. The mechanism is the **prior findings context** assembled into the prompt (see Reviewer Interface item 5): all non-archived findings for that `(project, pass)` pair — `open`, `accepted`, `deferred`, `rejected` — with description, location, current disposition, and any human note. The reviewer is told how to treat each state and references the existing ID when surfacing the same issue.

The dispatcher's normalize step has exactly two branches:

- **Fresh ID** (the reviewer output has no `id`, or the supplied `id` does not match any entry in the prior-findings context the dispatcher provided): allocate a new canonical ID `<project>/<pass>/<NNNN>`, write a new persisted Finding with `created_at = now`, `first_run_id = last_run_id = <current run>`, `disposition = "open"`, append a `finding_created` event to `dispositions.jsonl`.
- **Existing ID** (the supplied `id` matches an entry from the prior-findings context): load the existing persisted Finding, **update only `last_run_id`** to the current run, leave `id`, `created_at`, `first_run_id`, and `disposition` untouched, and append a `finding_reobserved` event to `dispositions.jsonl` so the audit trail records the recurrence. No new `finding_created` event is appended; the finding's disposition is not reset to `open`.

Both branches happen inside the per-project critical section so the sequence allocation, event append, and finding-file write commit together.

Reviewer judgment is the dedupe heuristic. Content-hash dedupe could be added later if reviewer behavior turns out to be inconsistent, but trusting the reviewer keeps the protocol simple and matches the reviewer-agnostic discipline applied elsewhere.

## State and Audit

The state directory on disk persists everything that matters across supervisor restarts. The shape:

```
<state_dir>/
  supervisor/
    events.jsonl           # supervisor lifecycle events (start, stop, pass dispatch); append-only from one process, safe
  projects/
    baab/
      last-run.json        # per-pass last-dispatched timestamps for this project; sharded so the project lock covers it
      backlog.md           # rolling backlog of open findings across all passes (rendered view)
      dispositions.jsonl   # append-only log of finding lifecycle events (source of truth)
      findings/
        baab--vocabulary-sweep--0042.json   # one file per finding (filename = ID components joined by `--`)
      runs/
        2026-05-13/
          vocabulary-sweep/
            060423Z-001/       # one directory per run; the leaf is the run ID's time+seq portion
              prompt.md        # the assembled prompt sent to the reviewer
              output.json      # raw structured output from the reviewer (audit only)
              findings.json    # frozen manifest: canonical finding IDs surfaced by this run + their disposition snapshot at completion
              report.md        # human-readable report, rendered once at completion (mattermost links here; never re-rendered)
              git-head.txt     # commit SHA of the project at pass-start time
            120511Z-001/       # same pass, second run of the day
              ...
```

### Run ID

Every run gets a globally-unique-within-otis run ID assigned by the dispatcher at run-start time:

```
<project>/<pass>/<YYYY-MM-DD>/<HHMMSSZ>-<NNN>
```

`HHMMSSZ` is UTC time-of-day; `NNN` is a zero-padded sequence (`001`, `002`, …) that disambiguates runs starting in the same second. The run ID is the canonical handle everywhere reports are addressed — REST paths, CLI subcommands, finding records, mattermost links. Same-day reruns (6-hour cadences, force-runs, retries after failures) get their own directory and their own ID; nothing is ever overwritten.

`dispositions.jsonl` is the source of truth for finding state. Every finding creation and every disposition change is appended as a structured event with timestamp, actor, and any notes. The current state of any finding is derivable by replaying its events; the `disposition` field on the finding's JSON file is a denormalized cache regenerated on every change.

`backlog.md` is a rendered human-readable view of open findings, regenerated after each disposition change. It exists so humans browsing the state directory directly (in an editor, file browser, or git log) see something current; the API is the primary interaction surface.

`runs/YYYY-MM-DD/<pass-name>/<HHMMSSZ-NNN>/` is the per-pass audit trail (see Run ID). Each run produces a fresh directory containing the assembled prompt, the raw reviewer output, a frozen finding manifest, a rendered markdown report, and the git HEAD at pass-start. Findings are always traceable to a specific commit, even when the working tree moves between runs.

The run report is **frozen at completion**: `report.md` and `findings.json` are written once inside the per-project critical section, after every persisted Finding for this run has been written and every disposition event has been appended. From then on, the report endpoint and any mattermost message link serve the stored markdown verbatim — re-rendering does not happen, so a disposition flipped tomorrow does not change what yesterday's report says. To see current state, consumers use `otis findings list` / `findings show` / the live API endpoints, which read the persisted Findings as they are *now*. The run report is *then*. This separation prevents the audit-trail slippage where an old mattermost message silently updates as state moves.

### State Mutation Invariant

The supervisor is a single process, but inside it the scheduler can fire parallel runs for the same project (per-reviewer and global concurrency caps both allow this) and the API can flip dispositions while runs are writing findings. To keep the state directory consistent, every mutation for a given project — finding-ID sequence allocation, `dispositions.jsonl` append, finding-cache JSON write, `backlog.md` re-render, run-ID `NNN` sequence allocation, **`last-run.json` read-modify-write** — happens inside a single process-local critical section, keyed by project name. All these files live under `state/projects/<project>/` precisely so the project lock is sufficient — no global mutable state file exists. Different projects do not contend with each other.

Concurrent readers (CLI list, API `GET`) acquire a read lock; writers acquire a write lock. Reads see a consistent view as of some moment after the last completed write. Per-project rather than global because there's no good reason for a run on `lore` to block a triage on `baab`.

Cross-process locking (file locks, advisory locks) stays deferred. The supervisor is a singleton on the HQ network; a second supervisor process is the failure mode the operator is expected to avoid, not one otis hardens against in the minimal harness.

## Configuration

Otis has two configuration files: per-project (`otis.yaml` at the project repo root) and global (location TBD by the planning agent — likely `/etc/otis/otis.yaml` or similar).

### Per-Project: `otis.yaml`

```yaml
project:
  name: baab
  description: groove-tagged drum library compositional tool
  notify:
    mattermost: "#otis-baab"
  top_findings: 3

passes:
  - name: vocabulary-sweep
    description: cross-module naming consistency
    scope:
      project: { type: full }
      bok:
        concerns: [vocabulary, naming]
    reviewer: { kind: pi }
    cadence: 24h

  - name: layering-check
    description: dependency-direction violations in /internal
    scope:
      project:
        type: paths
        paths: ["internal/"]
      bok:
        concerns: [layering, dependency-direction]
        top_k: 5
    reviewer: { kind: codex }
    cadence: 6h
    top_findings: 5

  - name: recent-changes-review
    description: structural review of the last day of commits
    scope:
      project:
        type: recent
        window: 24h
      bok:
        concerns: [architecture, cognitive-load]
        top_k: 15
    reviewer: { kind: claude-code }
    cadence: 24h
```

Project-level defaults (notification routing, finding budget) apply to all passes; individual passes override as needed.

### Global Config

```yaml
bok:
  path: /var/otis/bok
  search:
    default_top_k: 8           # K for combined-concern semantic search when a pass omits scope.bok.top_k

storage:
  state_dir: /var/otis/state

prompt:
  per_file_bytes:       8192      # per-file inline cap for paths/full scopes; above this, file is in the manifest only
  total_scope_bytes:    262144    # total inline scope-content cap; remaining files appear in the manifest only

api:
  listen: "0.0.0.0:8443"          # supervisor HTTPS bind address; required when `otis serve` is run
  tls:
    cert: /etc/otis/tls/cert.pem  # PEM-encoded certificate (self-signed acceptable on HQ network)
    key:  /etc/otis/tls/key.pem   # PEM-encoded private key

notification:
  mattermost:
    url: https://mm.hq.quigley.com
    token_env: MATTERMOST_TOKEN
  report_base_url: https://otis.hq.quigley.com   # externally-clickable base for links in messages; falls back to api.listen when absent

reviewers:
  claude-code:
    binary: claude
    default_model: opus-4.7
    concurrency_cap: 2
    window: overnight
  codex:
    binary: codex
    default_model: gpt-5.4
    concurrency_cap: 2
    window: overnight
  pi:
    binary: pi
    default_model: ollama/llama3.3
    concurrency_cap: 4
    window: anytime

windows:
  overnight:
    hours: "22:00-06:00"
  working-hours:
    hours: "09:00-17:00"
  anytime:
    hours: "00:00-24:00"           # `24:00` is parsed as exclusive end-of-day so the full 24-hour span is in-window

global_concurrency_cap: 6

projects:
  - path: /repos/baab
  - path: /repos/lore
  - path: /repos/mercurius
```

Operator-level concerns live in the global config: BoK location, storage paths, mattermost details, reviewer binaries and models, concurrency caps, scheduling windows, and the list of supervised repository paths. Project configs don't see any of this — they declare intent, the operator declares policy.

Project names (across all loaded `otis.yaml` files) and pass names (within a single project) must be unique. The config loader rejects duplicates as a hard error — both the `ResolvedConfig` map and the canonical finding ID format `<project>/<pass>/<NNNN>` assume uniqueness, and silently picking "the last one wins" would scramble the state directory's identity model.

Several knobs honor the convention-over-configuration preference: notification channel defaults to `#otis-<project-name>` if not specified, reviewer binaries default to looking on `PATH` if not given an explicit path, and the mattermost token reads from environment rather than from the config file.

## Interfaces

### HTTPS REST API

The supervisor exposes a REST API on the HQ network. The endpoints in the minimal harness:

```
GET    /api/v1/projects
GET    /api/v1/projects/{project}/passes
GET    /api/v1/projects/{project}/findings?disposition=open
GET    /api/v1/projects/{project}/findings/{pass}/{seq}
POST   /api/v1/projects/{project}/findings/{pass}/{seq}/disposition
GET    /api/v1/projects/{project}/runs/{pass}/{date}/{time_seq}/report
POST   /api/v1/projects/{project}/passes/{pass}/run    # force-run
```

### The `otis` CLI

The same binary that runs the supervisor is installed on workstations as a thin client. The mode is determined by subcommand and config. `otis serve` runs the supervisor daemon; everything else hits the supervisor's API over HTTPS.

```
otis findings list [--project X] [--pass Y] [--open]
otis findings show FINDING-ID
otis accept FINDING-ID [--note "..."]
otis defer FINDING-ID [--note "..."]
otis reject FINDING-ID [--note "..."]
otis report show RUN-ID
otis projects list
otis passes list [--project X]
otis pass run PROJECT/PASS-NAME
```

Workstation configuration is a small file with the supervisor's URL and a bearer token, conventionally at `~/.config/otis/config.yaml`.

### MCP Server

MCP tools mirror the REST surface. Digitals call them the way they call any other MCP tool.

The `otis mcp` subcommand runs locally on the workstation as a thin stdio bridge — it speaks MCP to the workstation's MCP client and forwards each tool invocation to the supervisor over the same authenticated HTTPS surface the `otis` CLI uses, reading `~/.config/otis/config.yaml` for the supervisor URL and bearer token. **The MCP bridge never opens the state store directly.** Every state-mutating tool call goes through an HTTPS endpoint on the supervisor, so the single process that owns the per-project locks is also the only writer. This keeps the C6 state-mutation invariant intact regardless of how many MCP clients are connected from how many workstations.

The minimal harness exposes the read side plus disposition writes: `otis_list_findings`, `otis_get_finding`, `otis_get_report`, `otis_disposition_finding`. Operator actions (force-run a pass, reload config) are CLI-only in the minimal harness; MCP exposure can be added later if there's a real reason for a digital to trigger one.

### Authentication

Bearer tokens. Each workstation and each digital MCP client carries a token; the supervisor verifies on every request. Token storage on the supervisor is a simple file or directory of tokens with optional labels for which token belongs to whom. Token issuance is out-of-band (the operator generates tokens and distributes them).

mTLS is a reasonable long-term direction but adds operational overhead the minimal harness doesn't need yet.

## Repository Management

The supervisor needs local checkouts of every project it watches — for reading `otis.yaml` and feeding code content to reviewers (which operate on filesystem paths).

In the minimal harness, sexton handles repository sync. The operator declares the repository paths in the global config; sexton keeps those paths current using the same mechanism that keeps grimoire checkouts current across workstations. Otis itself does not perform git operations on supervised repositories.

The freshness model is loose. When a pass runs, the checkout's freshness is whatever sexton's last sync delivered. This is acceptable for the minimal harness — passes are about state-over-time, not real-time analysis. Otis records the git HEAD at pass-start in the run's audit trail, so every finding is traceable to a specific commit even if the working tree moves between runs.

If a declared project path doesn't exist when the supervisor starts (sexton hasn't run yet, or the path is misconfigured), the supervisor logs a warning and skips that project's passes. The operator can fix it and otis picks up on the next config reload. This is the right failure mode for a long-running supervisor — fail loud, keep running.

A future capability — `git worktree add` for ephemeral, clean checkouts at arbitrary refs — is anticipated. The pass configuration will grow a `git:` block declaring a ref and a `clean: true` flag, and the supervisor will spin up an ephemeral worktree in `<state_dir>/scratch/<run_id>/` for the duration of the pass. Sharing the object store with sexton's clone keeps this fast. The reviewer interface and pass schema have natural extension points for this. It is not part of the minimal harness.

## Notification

The minimal harness writes to mattermost. Other notification surfaces are pluggable (the notification subsystem is an interface in the supervisor with one implementation); adding email, webhook, or other surfaces is mechanical when the need surfaces.

Each pass run that produces findings posts one mattermost message to the configured channel. The shape:

```
otis: baab vocabulary-sweep, 2026-05-13
3 findings (1 high, 2 medium)

#1  inconsistent use of "lens" vs "view" in hive subsystem    [baab/vocabulary-sweep/0042]  medium
#2  "library" overloaded between hive and storage contexts    [baab/vocabulary-sweep/0043]  medium
#3  "tag" and "label" used interchangeably in groove API      [baab/vocabulary-sweep/0044]  high

Full report: <notification.report_base_url>/api/v1/projects/baab/runs/vocabulary-sweep/2026-05-13/060423Z-001/report
Triage: otis accept baab/vocabulary-sweep/0042 / otis defer baab/vocabulary-sweep/0042 / otis reject baab/vocabulary-sweep/0042
```

A pass run that produces zero findings posts no message; silence is a valid output. The pass's last-run timestamp still updates so the scheduler doesn't re-fire it before the cadence elapses.

## Scenarios

Three concrete walk-throughs.

### A Vocabulary Sweep Fires

The scheduler observes that the `baab/vocabulary-sweep` pass is due (last fired more than 24 hours ago, currently inside the `anytime` window for the `pi` reviewer). It allocates a worker slot from pi's concurrency pool and dispatches.

The dispatcher constructs the prompt: it resolves the pass's concerns (`[vocabulary, naming]`) against the BoK index, retrieving the top-K relevant entries — `vocabulary/lens-vs-view`, `vocabulary/library-overloads`, `naming/established-conventions`. It loads the project context (name, description, primary language from `otis.yaml`), the project's full source tree (the pass declares `scope.project.type: full`), and the seven open findings in `baab/dispositions.jsonl`. It assembles all of this into a single prompt with the finding output schema and `top_findings: 3` cap.

The dispatcher invokes `pi -p ... --mode json` configured for read-only tool access. Pi runs in fresh context, reads what it needs from the filesystem, and emits structured findings on stdout. The dispatcher validates the output against the schema, writes the run's artifacts to `state/projects/baab/runs/2026-05-13/vocabulary-sweep/`, generates new finding records and appends `finding_created` events to `dispositions.jsonl`, renders an updated `backlog.md`, and posts a mattermost message to `#otis-baab` linking to the rendered report.

### A Human Triages a Finding

Michael, reading mattermost on his phone, sees the message above. He opens the linked report, reads finding `baab/vocabulary-sweep/0042`, agrees with it, and on his workstation runs:

```
otis accept baab/vocabulary-sweep/0042 --note "good catch; will fix as part of the hive refactor"
```

The CLI reads the local workstation config, looks up the supervisor URL and bearer token, and issues `POST /api/v1/projects/baab/findings/vocabulary-sweep/0042/disposition` with body `{"disposition": "accepted", "note": "..."}`. The supervisor authenticates the request, appends an event to `dispositions.jsonl`, updates the finding's denormalized state, and re-renders `backlog.md`. The finding's status moves from open to accepted; the next vocabulary-sweep pass will see it in the prior findings context with `accepted` status and not re-surface the same issue.

### A Digital Queries Open Findings

A claude code session running on Michael's workstation needs to do work in the baab repo. It connects to the otis MCP server (configured in the session's MCP client setup), calls `otis_list_findings` with `{"project": "baab", "disposition": "open"}`, and receives the current set of open findings as structured data.

The agent reads through the findings, identifies one that's adjacent to the work it's about to do (`baab/vocabulary-sweep/0043: "library" overloaded between hive and storage contexts`), and folds the rename into its planned changes. After the changes are committed, the next vocabulary-sweep run no longer surfaces that finding; the human can dispose of it as accepted with a note that it was fixed in passing.

## Deferred (and Why)

The following are deliberately out of scope for the minimal harness. They are real, but each carries enough design weight that pulling them into the first build would compromise the iterative shape we need.

**Higher permission gradient levels.** The vision document names four levels — surface only, propose diff, autonomous on clear-cut, continuous autonomous. The minimal harness sits between levels 1 and 2 (findings carry a prose `suggested_fix`, but no structured diffs are produced). Moving to true level 2 (structured proposed diffs alongside or replacing the prose) and beyond requires both BoK maturity and operational confidence neither of which exists at first run. Evolution happens through the design review process when the moment is right.

**Event-triggered passes.** `on-commit` and similar event-based triggers (PR open, branch push) are real modes worth supporting eventually. The minimal harness is cadence-based only.

**Continuous-mode passes.** A pass that runs, finishes, and immediately starts again — useful for very cheap reviewers — is supportable in the same architecture but not in the first build.

**Clean ephemeral checkouts via `git worktree`.** The minimal harness reads from sexton-synced working trees. Branch reviews, PR reviews, and other use cases that need a pristine checkout at an arbitrary ref are anticipated; the `git:` block on a pass is the configuration surface, the worktree mechanism is the implementation. Not in scope for the minimal harness because sexton-synced trees cover the foundational use cases.

**AST-level location anchors.** Findings reference code by file and line range. Function, type, and other AST-level anchors are useful but not load-bearing for the minimal harness.

**Content-hash-based finding dedupe.** The minimal harness trusts the reviewer to reference existing finding IDs when surfacing the same issue. A content-hash heuristic could be layered on later if reviewer behavior turns out to be inconsistent.

**Structured operator-action MCP tools.** The MCP surface in the minimal harness exposes read-side and disposition-write tools. Force-running a pass, reloading config, and similar operator actions are CLI-only. MCP exposure follows if real digital workflows need them.

**`severity_hint` and other BoK frontmatter expansions.** The minimal entry frontmatter has `title`, `tags`, and `created`. Calibration aids like a BoK-author's severity suggestion are interesting but might not earn their keep; defer until operational feedback says otherwise.

**In-project BoK augmentation at `<project>/.otis/bok/`.** Project-bound guidance can already live under `projects/<name>/` in the central BoK, which keeps the corpus, the embedding index, and the editorial pipeline single-sourced. Letting projects ship their own BoK fragments in-repo is a real eventual capability — a project's `AGENTS.md`-adjacent surface where local conventions get codified without round-tripping through the central repo — but it introduces a second source, a second sync path, and a merge model. Defer until the central-only model is exercised enough to know what the in-repo layer should actually carry.

**Per-finding-type permission gradients.** The vision document raises the possibility that vocabulary fixes could graduate to autonomous fast while architectural fixes stay surface-only indefinitely. This is appealing but is a refinement on the higher-permission-level work — not load-bearing until the gradient itself is moving forward.

**Shared embedding library between lore and otis.** Both will implement the same indexing pattern using lore's approach as the reference. Library extraction happens once both have stabilized and the right abstraction is visible. The planning agent has access to lore's code and will firm up the implementation specifics.

**Codified operational feedback ritual.** The harvest practice (humans and design agents collaborating to develop new BoK entries from observed runs) is expected to be emergent rather than scheduled, the way the mercurius feedback loop is. No protocol, no calendar — operational observation drives improvement when the operator is moved to act.

## Relationships

- `software/otis/otis-vision.md` (in the grimoire) — the philosophical framing this spec descends from.
- `software/mercurius/mercurius-vision.md` — the forward-motion sibling. Mercurius drives design toward buildable; otis tends code toward continuous quality. Complementary motions with a clean escalation handoff when an otis finding is design-level rather than maintenance-level.
- `concepts/frankie.md` — the architectural sibling. Otis is frankie-shaped: heartbeat-driven peer agent with a chat surface and a rolling backlog over a different domain.
- `research/software/ralph-loops.md` — the broader pattern landscape. Each otis pass is a slow-cadence Ralph iteration; the supervisor is the loop driver.
- `software/sexton/sexton-spec.md` — the upstream of all otis repository management.
- `software/lore/` — the reference implementation for the embedding index. Duplicated in otis for the minimal harness; library extraction deferred.

## Status

Design only. No implementation, no repository, no first heartbeat. The planning agent has the next move — translating this spec into a work order in the otis repo, grounded in the project's actual Go toolchain and integration points. Mercurius review of the spec (and the work order) follows the standard design-build pipeline.

The body-of-knowledge harvest can begin in parallel. Even with no otis to run against, articulating the first few entries — the lens/view convention, the dependency-direction principles in lore and baab, the cognitive-load discipline — is useful work in its own right, and gives the minimal harness something real to operate on the moment it runs its first pass.
