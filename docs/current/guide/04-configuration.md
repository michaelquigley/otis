# 4. Configuration

Otis configuration is three layers stacked together:

1. **Global config** — supervisor-wide settings (BoK path, state
   directory, API listener, reviewer definitions, windows, projects).
2. **Shared profiles** — reusable pass declarations that live in the BoK.
3. **Project config** — per-project overrides and additions, usually at
   `<bok.path>/projects/<name>/otis.yaml`.

This chapter walks each layer with snippets from `docs/example/`. The
contract version of this material lives in
[../configuration.md](../configuration.md).

## How Config Is Loaded

Otis uses [`github.com/michaelquigley/df/dd`](https://pkg.go.dev/github.com/michaelquigley/df/dd)
for YAML binding and merge behavior. Every config layer follows the same
sequence:

1. **Constructor defaults** populate sensible zero values.
2. **`dd.MergeYAMLFile`** overlays the YAML on disk.
3. **`Validate`** rejects obviously bad shapes early.
4. **`Resolve`** rewrites relative paths against the file that declared
   them.

Two consequences worth knowing:

- A relative `bok.path: ./bok` in `global.yaml` resolves against
  `global.yaml`'s directory, not your shell's `$PWD`.
- Config loading has no side effects. Commands that need writable
  directories (state, run artifacts) create them explicitly later.

## Global Config

`docs/example/global.yaml` is the canonical small example:

```yaml
bok:
  path: ./bok

storage:
  state_dir: ./.state

prompt:
  per_file_bytes: 8192
  total_scope_bytes: 262144

api:
  listen: "127.0.0.1:8443"

notification:
  mattermost:
    url: ""
    token_env: MATTERMOST_TOKEN
  report_base_url: https://otis.example.com

reviewers:
  dummy:
    concurrency_cap: 1
    window: manual
    output_path: ./dummy-output.json
  codex:
    binary: codex
    default_model: gpt-5.4
    concurrency_cap: 1
    window: anytime
  claude-code:
    binary: claude
    default_model: opus-4.7
    concurrency_cap: 1
    window: anytime
  pi:
    binary: pi
    default_model: ollama/llama3.3
    concurrency_cap: 1
    window: anytime

windows:
  manual:
    hours: "00:00-00:00"
  anytime:
    hours: "00:00-24:00"

global_concurrency_cap: 1

projects:
  - name: testproj
    path: ./testproj
```

### Field-by-field

**`bok.path`** — local BoK checkout. See
[03-body-of-knowledge.md](03-body-of-knowledge.md).

**`storage.state_dir`** — where Otis writes durable state: findings,
disposition logs, backlog renders, run artifacts, supervisor events. Put
it on durable storage you back up. Otis never deletes from here.

**`prompt.per_file_bytes` and `prompt.total_scope_bytes`** — the inline
prompt budget. `per_file_bytes` caps each in-scope file's contribution;
`total_scope_bytes` caps the whole inline scope. Files larger than the
per-file cap are truncated with a marker; if the total cap is exceeded,
later files are dropped. Defaults in the example are 8 KiB per file and
256 KiB total.

**`api.listen`** — the supervisor API listener. Use `127.0.0.1:PORT` for
local-only access; use `0.0.0.0:PORT` (or a specific interface) when
exposing the supervisor to workstations.

**`api.tls.cert` and `api.tls.key`** — optional TLS pair. Configure both
for direct HTTPS; omit both for plain HTTP behind a TLS-terminating
reverse proxy. Configuring only one of the pair is invalid. See
[06-deployment.md](06-deployment.md).

**`notification.mattermost`** — optional. `url` is the Mattermost webhook,
`token_env` names the environment variable that holds the token, and
`report_base_url` is what notifications prefix to run-report links. Leave
`url: ""` to disable notifications. See
[06-deployment.md](06-deployment.md) and `docs/example/mattermost-message.md`.

**`reviewers.<name>`** — per-reviewer adapter configuration:
- `binary` — the executable invoked by the adapter.
- `default_model` — the model passed unless a pass overrides it.
- `concurrency_cap` — how many runs of this reviewer can be in flight.
- `window` — named window controlling when the reviewer may run.
- `dry_run` — `true` to skip actually launching the binary.
- `output_path` — dummy-only; the JSON file the dummy reviewer reads.

See [05-reviewers.md](05-reviewers.md) for adapter-by-adapter notes.

**`windows.<name>.hours`** — `HH:MM-HH:MM`. The example defines `manual`
(`00:00-00:00`, never open) and `anytime` (`00:00-24:00`, always open).
`24:00` is accepted as an exclusive end-of-day sentinel, and
cross-midnight windows such as `22:00-06:00` are interpreted correctly.
Times are in the supervisor host's local timezone.

**`global_concurrency_cap`** — global ceiling on simultaneous in-flight
runs across all reviewers.

**`projects[]`** — supervised projects. Each entry has `name` (lowercase
kebab), `path` (local path to the working tree), and optional `config`
(path to a non-default project YAML). When `config` is omitted, Otis
loads `<bok.path>/projects/<name>/otis.yaml`.

## Project Config

`docs/example/bok/projects/testproj/otis.yaml`:

```yaml
include_configs:
  - standard

project:
  name: testproj
  description: small demonstration project for the Otis harness
  primary_language: go
  notify:
    mattermost: "#otis-testproj"
  top_findings: 2

passes:
  - name: vocabulary-sweep
    top_findings: 2
```

**`include_configs`** — list of shared profiles (root-level BoK YAML files)
to pull in. The example loads `standard` (which is `bok/standard.yaml`).
Cross-profile pass-name collisions are rejected.

**`project`** — metadata and optional overrides:
- `name`, `description`, `primary_language` — descriptive.
- `notify.mattermost` — channel for run notifications. Defaults to
  `#otis-<project>` when notifications are enabled and this is unset.
- `top_findings` — project-wide default cap for findings per run.

**`passes[]`** — additions and overrides. A pass entry whose `name`
matches an inherited pass merges onto it; a new name adds a new pass. To
remove an inherited pass, list it under `disable`.

## Passes, Scopes, and BoK Slices

A pass is the work unit. The example's `standard.yaml`:

```yaml
passes:
  - name: vocabulary-sweep
    description: cross-module naming consistency
    scope:
      project:
        type: full
      bok:
        include:
          - vocabulary/
          - layering/
          - cognitive-load/
    reviewer:
      kind: dummy
    cadence: 24h
    top_findings: 3
```

**Pass and project names** are canonical lowercase kebab
(`vocabulary-sweep`, not `Vocabulary Sweep` or `vocabulary_sweep`).

**`scope.project.type`** — `full`, `paths`, or `recent`:
- `full` — every tracked project file, bounded by the prompt budgets.
- `paths` — explicit paths, directories, and globs (declare them under
  `scope.project.paths`).
- `recent` — first-parent commits whose committer timestamps fall in the
  pass's own `scope.project.window`. Required for `recent` scopes.

**`scope.bok.include`** — the BoK slice. See
[03-body-of-knowledge.md](03-body-of-knowledge.md) for syntax.

**`reviewer.kind`** — adapter name from `global.yaml`'s `reviewers`.
Optional `reviewer.model` overrides that reviewer's `default_model` for
this pass.

**`cadence`** — how often the pass becomes eligible. Go-style durations:
`24h`, `4h`, `30m`.

**`top_findings`** — number of findings the reviewer should aim for.
Returning fewer is valid when nothing stronger exists.

## Cadence vs Window vs Force-Run

These three control different things and stack in a specific way:

| | Honored by scheduled run | Honored by force-run |
|---|---|---|
| **Cadence** | yes — pass not eligible until elapsed | bypassed |
| **Window** | yes — reviewer must be in its window | bypassed |
| **Reviewer concurrency cap** | yes | yes |
| **Global concurrency cap** | yes | yes |

A scheduled run that waits for a semaphore re-checks the reviewer window
after wake-up; if the window closed in the meantime, the run is dropped
without consuming `last-run.json`. Failed runs *do* consume the cadence
cycle — Otis is not a retry engine, so if a run fails and you want a
re-run sooner, force one.

The dispatcher allows at most one in-flight entry per `(project, pass)`.
A scheduled duplicate is silently skipped; a force-run duplicate fails
with an in-flight error that includes the run ID once one has been
allocated.

## Validation

`otis config check <path>` loads, validates, and resolves a global config
without starting the supervisor. Run it after every edit:

```bash
otis config check "$DEMO/global.yaml"
otis bok list --bok-path "$DEMO/bok"
otis bok resolve --bok-path "$DEMO/bok" \
  --include vocabulary/,layering/,cognitive-load/ \
  --project testproj
```

`bok list` enumerates entries; `bok resolve` shows exactly which files an
include list expands to for a given project. Use them to confirm a new
profile or include change is doing what you think.

---

Next: [05-reviewers.md](05-reviewers.md) — pick and wire the reviewer
adapter behind `reviewer.kind`.
