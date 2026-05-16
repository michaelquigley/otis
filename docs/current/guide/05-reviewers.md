# 5. Reviewers

A reviewer adapter is a read-only wrapper around a model CLI. The adapter
launches the binary with the right sandbox flags, passes it the prompt
Otis built, and parses the JSON it returns. Otis ships four adapters
today: `dummy`, `codex`, `claude-code`, and `pi`.

This chapter helps you pick one, wire it into `global.yaml`, and reason
about prompts, models, and concurrency. The reviewer schema, prior
context, and reuse semantics are covered in
[../invariants.md](../invariants.md).

## How Adapters Work

Every adapter is read-only. Otis snapshots the project's `HEAD` into a
throwaway git worktree, builds a prompt that contains the role/goal, the
BoK slice, the project context, the scope manifest, bounded inline scope
content, prior findings, and the output schema. The adapter launches its
configured binary with read-only flags, hands it that prompt on stdin (or
the adapter's equivalent), and receives JSON back. Otis validates the
JSON against the finding schema — extra fields are rejected — and the
dispatcher attributes project, pass, reviewer, run ID, and disposition.

The prompt is bounded by `prompt.per_file_bytes` and
`prompt.total_scope_bytes` from `global.yaml`. Larger files are truncated
with a marker; the total scope cap drops later files. If a reviewer needs
to see more of the codebase than fits, narrow the scope (use `paths` or
`recent`) rather than raising the budgets.

## Choosing a Reviewer

| Reviewer | When to pick it |
|---|---|
| `dummy` | tests, demos, CI smoke runs, any time you need a deterministic finding without an LLM |
| `codex` | OpenAI Codex CLI with read-only sandboxing |
| `claude-code` | Anthropic's Claude Code CLI in plan mode with read-only tools |
| `pi` | Pi CLI in JSON mode; treat real Pi usage as an adapter-calibration pass (see [../deferred.md](../deferred.md)) |

A pass selects its reviewer via `reviewer.kind`. You can run different
passes on different reviewers in the same project — e.g., a fast
`vocabulary-sweep` on `claude-code` and a deeper `layering-audit` on
`codex`.

## `dummy`

Deterministic, file-backed reviewer for tests and demos. The example
config wires it like this:

```yaml
reviewers:
  dummy:
    concurrency_cap: 1
    window: manual
    output_path: ./dummy-output.json
```

The adapter reads `output_path` at run time and returns whatever JSON
findings are there. This is what makes the quickstart reproducible — the
[02-quickstart.md](02-quickstart.md) walkthrough swaps `dummy-output.json`
between a one-finding payload and an empty `{"findings":[]}` to drive
the demo without needing an LLM.

`output_path` resolves relative to the global config file. You can point
multiple passes at the same dummy reviewer; they all read the same file.

## `codex`

Wraps `codex exec` with a read-only sandbox.

```yaml
reviewers:
  codex:
    binary: codex
    default_model: gpt-5.4
    concurrency_cap: 1
    window: anytime
```

- `binary` — the Codex CLI binary. Make sure it is on the supervisor's
  PATH or give an absolute path.
- `default_model` — passed to `codex exec` unless a pass overrides it
  with `reviewer.model`.
- `concurrency_cap` — how many parallel `codex` runs are allowed.
- `window` — named window from `windows.*`.

The adapter constrains Codex to read-only sandboxing. Otis never invokes
Codex with write permissions; this is a design invariant rather than a
configuration option.

## `claude-code`

Wraps `claude -p` with the plan-mode permission profile and read-only
tools.

```yaml
reviewers:
  claude-code:
    binary: claude
    default_model: opus-4.7
    concurrency_cap: 1
    window: anytime
```

Same fields as `codex`. The adapter requests plan permission mode and a
read-only tool surface; the reviewer can read the worktree and produce
JSON output but cannot modify files.

## `pi`

Wraps `pi -p` in JSON mode.

```yaml
reviewers:
  pi:
    binary: pi
    default_model: ollama/llama3.3
    concurrency_cap: 1
    window: anytime
```

The Pi CLI contract is less settled than Codex and Claude Code today.
Treat any real Pi usage as an adapter-calibration pass: run it in
parallel with a known-good reviewer on the same pass, compare findings,
and feed back what breaks. See the deferred note in
[../deferred.md](../deferred.md).

## Per-Pass Model Override

Use the global `default_model` for the common case, and override per pass
when a specific pass benefits from a different model:

```yaml
passes:
  - name: vocabulary-sweep
    reviewer:
      kind: claude-code
      model: haiku-4.5
    cadence: 4h
    top_findings: 3
```

A faster, cheaper model is often appropriate for tight passes (vocabulary
sweeps, naming checks) while heavier passes (layering audits, behavior
reviews) want the strongest model you have.

## Concurrency

Two caps protect the supervisor host:

- **Per-reviewer `concurrency_cap`** — how many runs of *this* reviewer
  can be in flight at once.
- **Global `global_concurrency_cap`** — total in-flight runs across all
  reviewers.

Force-runs respect both caps; they only bypass cadence and window. Pick
caps based on the supervisor host's CPU/memory and what the reviewer
binaries do under load. Starting with `1` everywhere and raising once you
have a feel for actual run times is a safe default.

## Reviewer-Supplied IDs and Reuse

When Otis builds a prompt, it includes the prior findings (open,
accepted, deferred, rejected) for the same `(project, pass)`. Reviewers
are asked to reuse an existing ID when surfacing the same open issue.

Otis honors that:

- If the reviewer's output sets `id` to a known ID from the prior
  context, the dispatcher updates `last_run_id` and appends a
  `finding_reobserved` event. Disposition is not reset.
- Missing or unknown IDs allocate a fresh open finding and append
  `finding_created`.

The reviewer schema is otherwise lean: `severity`, `title`, `location`,
`bok_refs`, `description`, `suggested_fix`. Anything beyond that is
dispatcher-owned and is rejected if it appears in reviewer output.

---

Next: [06-deployment.md](06-deployment.md) — install, run, secure, and
notify a real supervisor.
