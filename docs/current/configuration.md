# Configuration

Otis uses `github.com/michaelquigley/df/dd` for YAML binding and merge behavior.
Defaults are set by constructors, YAML overlays are applied with `dd`, then
configs validate and resolve paths relative to the file that declared them.

## Global Config

The global config names the BoK checkout, state directory, prompt byte budgets,
TLS listener, notifications, reviewer definitions, scheduling windows,
concurrency caps, and supervised projects.

Important fields:

- `bok.path`: local BoK root.
- `storage.state_dir`: durable supervisor state.
- `prompt.per_file_bytes` and `prompt.total_scope_bytes`: inline prompt budget.
- `api.listen`, `api.tls.cert`, `api.tls.key`: HTTPS supervisor listener.
- `notification.mattermost.url`, `notification.mattermost.token_env`,
  `notification.report_base_url`: optional Mattermost posting.
- `reviewers.<name>`: `binary`, `default_model`, `concurrency_cap`, `window`,
  `dry_run`, and for the dummy reviewer `output_path`.
- `windows.<name>.hours`: `HH:MM-HH:MM`, including cross-midnight windows and
  `24:00` as an end-of-day sentinel.
- `projects[]`: `name`, `path`, and optional `config`.

When a project omits `config`, Otis loads `<bok.path>/projects/<name>/otis.yaml`.

## Project Config

Project config can include shared profiles, disable inherited passes, declare
project metadata, and add or override passes.

Pass names and project names use the canonical lowercase kebab grammar. A pass
declares:

- `scope.project.type`: `full`, `paths`, or `recent`.
- `scope.project.paths`: path and glob entries for `paths` scope.
- `scope.project.window`: required for `recent` scope.
- `scope.bok.include`: directory or file-path BoK includes.
- `reviewer.kind` and optional `reviewer.model`.
- `cadence` and `top_findings`.

Shared profiles are root-level BoK YAML files such as `standard.yaml`. Project
composition loads profiles in order, rejects cross-profile pass-name collisions,
applies `disable`, and merges project pass overrides by pass name.

## BoK Resolution

BoK entries are markdown files under category subtrees such as `vocabulary/`,
`layering/`, and `cognitive-load/`. Root-level markdown files are skipped.

Includes support:

- trailing-slash directories, such as `vocabulary/`;
- explicit file paths without `.md`, such as `vocabulary/lens-vs-view`.

Bare terms are reserved for a future semantic-search extension and are rejected
today. Entries under `projects/<name>/` are only included for that project.
