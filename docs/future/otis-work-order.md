# Otis — Work Order

Companion to `docs/future/otis-spec.md`. The spec is the **what** and **why**; this work order is the **how**, grounded in the otis repo's actual toolchain and the implementation patterns already established across lore, mercurius, sexton, and the df libraries.

## Context

`/home/michael/Repos/q/products/otis/` is greenfield: a `go.mod` (Go 1.26.2, module `github.com/michaelquigley/otis`), a `.gitignore`, an example `mercurius.yaml` ready for the review loop, and two `docs/future/` documents — the spec and a harvest agent guide. No source code yet.

The work below translates the spec into a phased build with human review/commit between phases. Each phase leaves the repo in a green state, can be exercised in isolation, and pushes the system one observable step closer to the first heartbeat. Phase boundaries are intended as commit points; mid-phase commits are fine but the boundary commits are where Michael reviews, reads, and decides whether to roll forward.

## Decisions Locked Going In

Three calibration calls were made with Michael before drafting:

- **Reviewer scope.** Codex adapter first (mirroring mercurius's proven `codex exec` dispatch); claude-code + pi adapters as a discrete later phase. Pi is the least-validated harness and gets the most learning value from being added once the rest of the pipeline is real.
- **MCP SDK.** `github.com/modelcontextprotocol/go-sdk` (matches mercurius). Lore's `mark3labs/mcp-go` is not adopted — the practice prefers one MCP SDK going forward.
- **BoK reading model.** Filesystem walk, no index. The minimal harness reads the BoK directly from the sexton-synced tree at pass time. Embedding-backed semantic search is deferred (see the spec's Deferred section); `include` lists support directory subtrees and explicit file paths only. Bare-term entries are reserved syntax and rejected by the loader today.

## Toolchain & Idiom Anchors

| Concern | Decision | Anchor in the practice |
|---|---|---|
| Logging | `github.com/michaelquigley/df/dl`, init in `cmd/otis/main.go` | mercurius `cmd/mercurius/main.go:212–215`, sexton `cmd/sexton/main.go:24–25` |
| Config binding | `github.com/michaelquigley/df/dd` with `dd:"name,+required,+secret"` tags | lore `internal/config/load.go`, mercurius `internal/config/config.go:25–75` |
| CLI | `github.com/spf13/cobra`, single binary, subcommands for `serve` and client commands | mercurius `cmd/mercurius/main.go:37–101` |
| MCP server | `github.com/modelcontextprotocol/go-sdk` | mercurius `internal/mcpserver/mcpServer.go:275+` |
| Frontmatter parsing | `dd` YAML unmarshal (with `+extra` capture for unmapped keys) | lore `internal/markdown/frontmatter.go` |
| Reviewer dispatch | `exec.CommandContext` with prompt-via-stdin, schema-via-file, output-via-file | mercurius `internal/reviewer/codex/codexReviewer.go:41–107` |
| State persistence | atomic temp-file-rename writes; append-only `.jsonl` event logs; immutable round logs | mercurius `internal/monitor/monitor.go:69`, `internal/roundlog/roundLog.go:48` |
| Signal handling | `signal.NotifyContext` for SIGINT/SIGTERM threaded through cobra `ExecuteContext` | mercurius `cmd/mercurius/main.go:24–28` |
| JSON schema validation | `github.com/santhosh-tekuri/jsonschema/v6` | mercurius `internal/schema/reviewOutput.go` |
| Repo sync | Sexton, out-of-band; otis reads working trees, never `git pull`s | sexton README + status CLI |

## Practice Idioms — df/dl and df/dd

These patterns are convergent across mercurius, lore, sexton, archive, and the df library's own examples. Otis adopts them verbatim.

### Logging (`df/dl`)

**Init.** Exactly once at process start, in `cmd/otis/main.go`:

```go
dl.Init(dl.DefaultOptions().
    SetTrimPrefix("github.com/michaelquigley/otis/").
    SetLevel(slog.LevelInfo))
```

Mirrors mercurius `cmd/mercurius/main.go:212–215`. The trim prefix is the otis module path so call sites render cleanly.

**Verbose.** A cobra `PersistentPreRun` hook on the root command re-initializes the logger at debug level when `--verbose` is set (anchor: sexton `cmd/sexton/main.go:14–26`).

**Channels.** `dl.ChannelLog(name)` is for *output routing* (per-channel format or destination), not for business-logic separation. Default to the global `dl.Log()` everywhere. If a subsystem genuinely needs separated output (e.g., the dispatcher writing reviewer-subprocess transcripts somewhere richer than stderr), introduce a channel then — not preemptively.

**Structured fields.** Ad-hoc `.With(key, val).With(...)` chains. No fixed key vocabulary; keys are domain-driven (`project`, `pass`, `run`, `reviewer`, `finding`). Anchor: `df/dl/examples/dl_01_hello_world/main.go:57–62`.

**Wrapping.** Never. Call `dl.Log()`, `dl.Infof(...)`, `dl.Debugf(...)`, `dl.Errorf(...)` directly. No project in the practice wraps `dl`.

**Fatal.** `dl.Fatalf` only at the top-level main on unrecoverable startup errors (mercurius `cmd/mercurius/main.go:29`). Subsystem errors propagate as `error` return values up the call stack.

### Data binding (`df/dd`)

**Tag idiom.** `dd:"snake_case_name[,+required][,+secret][,+omitempty][,+extra]"`. Custom name first, then flags. If the auto snake_case from the field name suffices, the name can be omitted (`dd:",+required"`). No `+default=...` or `+match=...` modifiers in production use.

**Anchors:** `df/dd/examples/dd_02_struct_tags/main.go:12–25`, lore frontmatter (where `+extra` captures unmapped keys).

**Defaults via struct field initialization, not `ApplyDefaults()`.** No project in the practice uses the `Defaulter` interface. The convention is:

```go
func DefaultGlobalConfig() *GlobalConfig {
    return &GlobalConfig{
        StorageStateDir:      "/var/otis/state",
        GlobalConcurrencyCap: 6,
        // ...
    }
}

func LoadGlobal(path string) (*GlobalConfig, error) {
    cfg := DefaultGlobalConfig()
    if err := dd.MergeYAMLFile(cfg, path); err != nil {
        return nil, err
    }
    if err := cfg.Validate(); err != nil {
        return nil, err
    }
    if err := cfg.Resolve(filepath.Dir(path)); err != nil {
        return nil, err
    }
    return cfg, nil
}
```

Anchor: mercurius `internal/config/config.go:45–75`.

**Loading.** `dd.MergeYAMLFile(cfg, absPath)` after defaults are populated. The merge overlays YAML on top of defaults, so absent keys retain their default values cleanly.

**Layered configs (global + per-project).** Lore's two-part struct pattern fits otis well — the two configs have different audiences (operator vs. project author) and shouldn't be silently merged into one struct. Adopt:

```go
type ResolvedConfig struct {
    Global   *GlobalConfig
    Projects map[string]*ProjectConfig
}
```

Anchor: lore `internal/config/load.go:18–32`. The scheduler holds the `ResolvedConfig` map and consults both halves when dispatching a pass.

**Validation.** Each config struct grows a `Validate() error` method called after `dd.MergeYAMLFile`. Use `+required` for shape checks (the dd machinery enforces them) and the `Validate()` body for cross-field business rules. Anchor: mercurius `internal/config/config.go:83–100`.

**Path resolution.** Post-load `Resolve(baseDir string) error` method expands `~`, `$VAR`, and joins relative paths against `baseDir` (the directory of the loaded config file). Anchors: mercurius `internal/config/config.go:157–180`, lore `internal/config/load.go:111–134`. Apply to every path field — `bok.path`, `storage.state_dir`, project paths in the global config, etc.

**Secrets.** `dd:",+secret"` masks the value in `dd.Inspect()` debug dumps. It does *not* load from env. The env-indirection pattern is an explicit `token_env: MATTERMOST_TOKEN` field that the loader resolves at use-time (per spec section 7.2) — separate from `+secret`. Otis follows that pattern for the mattermost token.

**Writing config back.** Not done in production code anywhere in the practice. Don't add it.

### Frontmatter parsing (`dd` + markdown)

BoK entries use YAML frontmatter. Parse with `dd` (not `gopkg.in/yaml.v3` directly) so the same tag idiom carries through. The `+extra` modifier captures unmapped keys into a `map[string]any` field — useful because the BoK frontmatter is intentionally light and unknown keys should round-trip rather than error. Anchor: lore `internal/markdown/frontmatter.go`.

## Target Repo Layout

```
otis/
  cmd/
    otis/
      main.go              # cobra root, df/dl init, ExecuteContext + SIGINT/SIGTERM
      serve.go             # `otis serve` — supervisor daemon
      client_*.go          # `otis findings list/show/accept/defer/reject`, `otis pass run`, etc.
  internal/
    config/
      global.go            # global config struct + DefaultGlobalConfig() + Validate() + Resolve()
      profile.go           # shared pass profile (.yaml at BoK root) struct + LoadProfile
      project.go           # per-project config (include_configs, disable, passes) + Validate + Resolve
      compose.go           # one-level composition: profiles + disable + project add/override → ResolvedProject
      load.go              # Load(globalPath) → ResolvedConfig{Global, Projects map[string]*ResolvedProject}
    state/
      paths.go             # state-dir layout helpers
      store.go             # per-project sync.RWMutex registry; entry point for all mutating ops
      findings.go          # Finding struct + JSON file IO; canonical-ID validators; writes take project write lock
      dispositions.go      # append-only dispositions.jsonl; current-state + note-history reduction; writes under project lock
      backlog.go           # backlog.md renderer; called under project write lock after each event
      runs.go              # runs/<date>/<pass>/<HHMMSSZ-NNN>/ artifact writers; allocates the time+seq; writes frozen findings.json + report.md
      lastrun.go           # per-project last-run.json (dispatch-start timestamps); under project lock
      events.go            # supervisor/events.jsonl lifecycle event append (single-writer, no lock needed)
      scratch.go           # state_dir/scratch/<run_id>/ worktree directory helpers + orphan cleanup at startup
    bok/
      entry.go             # Entry struct + ReadEntry(bokPath, relpath) — single-entry read from disk (.md appended at I/O)
      resolve.go           # filesystem walker; directory + file-path forms of `include`; project-location filter; root-file skip; bare-term rejection
      frontmatter.go       # dd-based YAML frontmatter parsing (title, tags, created)
    reviewer/
      reviewer.go          # Reviewer interface + Request/Result types
      dummy/dummy.go       # deterministic test reviewer (mirrors mercurius)
      codex/codex.go       # codex exec adapter (phase 5)
      claudecode/cc.go     # `claude -p ...` adapter (phase 10)
      pi/pi.go             # `pi -p ...` adapter (phase 10)
    prompt/
      prompt.go            # assemble role+goal, BoK slice, project context, scope content, prior findings context, schema+budget
      scope_content.go     # manifest + bounded inline (recent diffs, paths/full body inlining with byte caps)
      schema.go            # reviewer output JSON schema (lean shape; top_findings as maxItems)
    scheduler/
      scheduler.go         # tick loop: BoK incremental sync, then due-list construction
      windows.go           # window membership (HH:MM, cross-midnight, 24:00 sentinel)
    dispatcher/
      dispatcher.go        # per-reviewer + global semaphores; inFlight map; defer-released
      run.go               # one pass execution end-to-end (worktree create/remove, normalize, persist, frozen artifacts)
      scope.go             # resolves scope.project (full / paths / recent) into a concrete file set, run against the worktree
      worktree.go          # git worktree add/remove wrappers; capturedSHA helper
    notify/
      notify.go            # interface
      mattermost/mm.go     # mattermost poster (phase 10)
    api/
      router.go            # stdlib mux; the seven endpoints
      auth.go              # bearer-token file store
      handlers.go
      render.go            # report.md rendering at run-completion (called from dispatcher, not API)
    client/
      config.go            # ~/.config/otis/config.yaml loader (supervisor URL + bearer token)
      http.go              # authed HTTP client shared by CLI subcommands and MCP bridge
    mcp/
      bridge.go            # `otis mcp` stdio↔HTTPS bridge; reuses internal/client/{config,http}
      tools.go             # MCP tool handlers that forward to the supervisor over HTTPS (no direct state writes)
  docs/
    future/
      otis-spec.md         # already present
      otis-work-order.md   # this document
      harvest-agent-guide.md
    example/
      bok/                 # ships a tiny BoK so the system is demoable
  go.mod
  go.sum
  README.md
  AGENTS.md
```

Internal-only packages by convention — only `cmd/` reaches across.

## Phases

Eleven phases. Each phase: state the goal, the files touched, the verification that proves it landed. **Bold** items are the demoable check at phase end. Phase boundaries are commit points where Michael reviews and decides whether to roll forward.

---

### Phase 1 — Scaffolding & config

**Goal.** Bring the repo up to a buildable state with the toolchain wired and both config files parsing.

**Lands.**
- `cmd/otis/main.go` with cobra root, `df/dl` init (output stderr, trim prefix `github.com/michaelquigley/otis/`), `signal.NotifyContext` wired into `ExecuteContext`.
- `otis serve` subcommand stub — loads global config, logs "supervisor would start here," exits.
- `otis version`, `otis config check <path>` subcommands.
- `internal/config/global.go` — global config struct with `dd` tags. Fields per spec: `bok.path` (filesystem path to the sexton-synced BoK repo — read directly, no index), `storage.state_dir`, `prompt.per_file_bytes` (default 8192), `prompt.total_scope_bytes` (default 262144), `api.listen` (e.g. `0.0.0.0:8443`), `api.tls.cert`, `api.tls.key`, `notification.{mattermost.{url, token_env}, report_base_url}`, `reviewers.<name>.{binary, default_model, concurrency_cap, window}`, `windows.<name>.hours`, `global_concurrency_cap`, `projects[].{name, path, config}`. The `config` field is optional and defaults to `<bok.path>/projects/<name>/otis.yaml` when omitted (convention-over-config). Defaults via a `DefaultGlobalConfig()` constructor (struct-field init — practice convention, see Practice Idioms). Resolver expands `~` / env / relative paths.
- `internal/config/profile.go` — shared pass profile struct (the shape of a `<bok.path>/<name>.yaml` file). Carries `passes[]` only; no `include_configs`, `disable`, or `project` block. `LoadProfile(path)` returns the parsed profile or a hard error citing the file path and offending line.
- `internal/config/project.go` — per-project config struct with `dd` tags. Fields: `include_configs []string`, `project: { name, description, notify, top_findings, ... }`, `disable []string`, `passes []Pass`. Same `Default*` + `Validate()` + `Resolve(baseDir)` shape. `Validate()` enforces that every pass with `scope.project.type: recent` carries a non-zero `scope.project.window` (missing or zero is a hard load error; no cadence fallback), rejects duplicate pass names within the project's own `passes` list, and reuses `state.ValidateIDComponent` (the same validator the canonical finding ID uses: lowercase kebab grammar `^[a-z0-9]+(?:-[a-z0-9]+)*$`) against `project.name` and each `pass.name`. Also validates each pass's `scope.bok.include` list: non-empty (a BoK-using pass must declare something) and **every entry must contain `/`** (bare terms are reserved for the future semantic-search extension and rejected today; a clear error message points the operator at the deferred capability). Form classification (directory vs file path) happens in `internal/bok/resolve.go`. The "no root-level BoK entries" rule lives in the BoK walker (`internal/bok/resolve.go`) which skips any `.md` directly at the corpus root.
- `internal/config/compose.go` — runs the spec's composition resolution for a single project. Input: the project's parsed config + a profile loader (closure over `<bok.path>`). Output: a `ResolvedProject` carrying the composed pass list and project-level fields. Algorithm exactly mirrors the spec's "Composition Resolution" steps: load each `include_configs` entry, union their pass lists keyed by name (cross-profile name collision → hard error citing both profiles), drop names listed in `disable`, walk the project's own `passes` list with add-or-field-merge semantics (`dd.Merge` for field-merge), final cross-check for duplicate names in the result. Errors carry the file+line that introduced the problem so `otis config check` can point at it.
- `internal/config/load.go` — `LoadGlobal(path)` and `LoadProject(path, profileLoader)` wrappers that orchestrate defaults → `dd.MergeYAMLFile` → `Validate` → `Resolve` → (for project) `Compose`. A `Load(globalPath)` top-level call returns a `ResolvedConfig{Global, Projects map[string]*ResolvedProject}` where each ResolvedProject is the post-composition result. Rejects duplicate `projects[].name` values in the global config as a hard error.
- `AGENTS.md` at repo root pointing at spec + work order, naming the toolchain idioms.
- `README.md` skeleton.

**Verify.** `go build ./...` succeeds, `go vet ./...` clean, `otis config check <example-global>.yaml` validates an example file (ship one as `docs/example/global.yaml`), **`otis version` and `otis serve` both run and log via df/dl**.

---

### Phase 2 — State directory + finding/disposition core

**Goal.** Findings and disposition events round-trip through disk without any reviewer involvement yet.

**Lands.**
- `internal/state/paths.go` — `StateRoot`, `ProjectDir`, `RunDir`, `FindingsDir`, etc.
- `internal/state/store.go` — the per-project mutex registry. `Store` holds `map[string]*sync.RWMutex` guarded by its own top-level mutex; `func (s *Store) Project(name string) *Project` returns a handle that lazily creates the lock on first access. The `Project` handle is the entry point for every mutating op below; nothing outside the state package allocates IDs, appends to `dispositions.jsonl`, writes finding JSON, or renders `backlog.md`. The dispatcher and the API both consume `Project`.
- `internal/state/findings.go` — persisted `Finding` struct mirroring spec section "Persisted Finding Schema" exactly, JSON file IO using atomic temp-file-rename (mirror mercurius `monitor.WriteStatus`). Canonical ID is `<project>/<pass>/<NNNN>`; filename joins the components with `--` (so `baab/vocabulary-sweep/0042` ↔ `baab--vocabulary-sweep--0042.json`). The package exposes `ParseID(string) (FindingID, error)`, `ParseFilename(string) (FindingID, error)`, and `(FindingID).Filename() string` so no other call site reimplements the mapping. **`ValidateIDComponent(string) error`** is exported for reuse by the config loader (per Phase 1) and enforces the canonical grammar `^[a-z0-9]+(?:-[a-z0-9]+)*$` for project names and pass slugs; the sequence component is `^[0-9]{4,}$`. Splitting a filename on `--` yields exactly three components — `<project>`, `<pass>`, `<seq>.json` — making the mapping unambiguous regardless of internal dashes. All write methods take the project's write lock.
- `internal/state/dispositions.go` — append-only `dispositions.jsonl`, event types `finding_created`, `finding_reobserved`, and `disposition_changed`, current-state reducer. Append/reduce both go through the project lock so a reader never sees a half-appended line and two writers can't allocate the same sequence. `finding_reobserved` records that an existing finding was re-surfaced by a later run (carries the run ID); it never changes disposition or any other field on the Finding — its only effect through the reducer is to set `last_run_id`. The reducer also produces the ordered **note history** consumed by `internal/prompt/prompt.go` — every `disposition_changed` event contributes `(disposition_at_event, note_text)` to the finding's history in event order. `finding_created` and `finding_reobserved` do not contribute notes.
- `internal/state/backlog.go` — render `backlog.md` from open findings; called after every event, under the project write lock so the render reflects the just-appended state.
- `otis findings list --project X [--open]` and `otis findings show ID` client subcommands wired against a **local** state dir for now (no HTTP yet) so the feature can be exercised before phase 7.
- A unit test that writes a couple of findings + dispositions through the writers, reads them back, asserts state matches.

**Verify.** Unit tests pass. **Hand-emit two findings into a fixture state dir; `otis findings list` shows them; `otis findings show` renders the detail view; `backlog.md` renders correctly; appending an `accepted` disposition event flips the cache and removes the entry from the backlog**.

---

### Phase 3 — BoK on-disk reader

**Goal.** Read the BoK directly from the sexton-synced filesystem tree at pass time. Resolve a pass's `include` list (directory subtrees + explicit file paths) into a deduplicated, project-filtered slice ready for prompt assembly. No index, no embedding backend.

**Lands.**
- `internal/bok/frontmatter.go` — `dd`-based YAML frontmatter parsing; extract `title`, `tags`, `created`. No `applies_to` field — scope is encoded by location.
- `internal/bok/entry.go` — `Entry` struct (`Relpath string` (extensionless, e.g., `vocabulary/lens-vs-view`), `Title string`, `Tags []string`, `Body string`). `ReadEntry(bokPath, relpath string) (*Entry, error)` reads one entry from disk, appending `.md` at I/O time. The struct is what the prompt assembler embeds.
- `internal/bok/resolve.go` — filesystem walker. Given `(bokPath, include []string, projectName string)`: for each entry in `include`, classify by form — trailing `/` → directory walk under `filepath.Join(bokPath, entry)`, collecting every `.md` not at the corpus root; otherwise → literal file path, read directly. Apply the project-location filter in Go (skip results whose relpath starts with `projects/X/` for `X ≠ projectName`). Deduplicate by relpath. **Reject any `include` entry with no `/`** — those are reserved for the future semantic-search extension; current loader errors with `bare-term entries are reserved for a future capability; use a directory ('vocabulary/') or file path ('vocabulary/lens-vs-view') instead`. Missing file-path entries are hard errors with the pass-config location in the message. The walker also enforces the "no root-level entries" rule: it skips any `.md` directly at the BoK root.
- `otis bok list` (dump every BoK entry's relpath grouped by subtree, useful for hand-verifying what's available) and `otis bok resolve --include <list> --project <name>` (preview the slice a pass would see) subcommands.

**Verify.** Ship a tiny example BoK under `docs/example/bok/` (a couple of vocabulary entries plus a layering entry). **`otis bok list --bok-path docs/example/bok/` enumerates the entries; `otis bok resolve --bok-path docs/example/bok/ --include vocabulary/,naming/lens-vs-view --project baab` returns the unioned slice with the project-location filter applied; supplying a bare-term entry (`--include vocabulary,naming`) errors clearly with the deferred-capability message**.

---

### Phase 4 — Reviewer interface + prompt assembly

**Goal.** A reviewer-shaped subprocess can be invoked against a fully-assembled prompt and return validated output. No dispatcher, no persistence, no worktree — just the prompt-and-reviewer subsystem in isolation, exercised by a fixture-driven test harness.

**Lands.**
- `internal/reviewer/reviewer.go` — `Reviewer` interface (`Review(ctx, Request) (Result, error)`), `Request` carries assembled prompt + schema bytes + working directory + model override, `Result` carries raw output bytes + the parsed findings + the path where artifacts were written.
- `internal/reviewer/dummy/dummy.go` — deterministic reviewer used by tests; reads canned output from disk; mirrors mercurius's dummy.
- `internal/prompt/schema.go` — **reviewer output JSON schema** only (per spec section "Reviewer Output Schema"): the lean shape — optional `id`, required `severity`, `title`, `location`, `bok_refs`, `description`, `suggested_fix`. `top_findings` enforced via `maxItems` on the findings array (mirror mercurius's `schema.ReviewOutputSchemaWithMaxFindings`). Validated via `santhosh-tekuri/jsonschema/v6`. The persisted Finding shape lives in `internal/state/findings.go` (Go struct, not a JSON schema) — the dispatcher's normalize step (Phase 5) constructs it from the validated reviewer output plus dispatcher-owned fields.
- `internal/prompt/prompt.go` — assemble role+goal, BoK slice (the resolved `include` list via `internal/bok/resolve.go` — directory + file-path entries read fresh from disk, deduplicated, project-filtered), project context (non-BoK metadata only: project name, description, primary language from the resolved project config; no BoK content sneaks in through this section), **scope content (manifest + bounded inline)** built by `prompt/scope_content.go`, **prior findings context** (every non-archived finding for the same project+pass with description, location, disposition, and human note — `open`, `accepted`, `deferred`, `rejected` all included; explicit per-disposition handling rules in the prompt copy; consumes the note-history reducer from Phase 2's `internal/state/dispositions.go`), output schema + budget. Mirror mercurius's modular layout from `internal/prompt/prompt.go:29–107`.
- `internal/prompt/scope_content.go` — turns the scope resolver's output (from Phase 5's `dispatcher/scope.go` — but the function signature is settled here as `(scopeKind, files []string, baseSHA string, projectPath string) → ScopeContent`) into the manifest-plus-inline payload defined in the spec. Always emits the manifest (relative paths) and the project's `git rev-parse HEAD`. Inline rules: for `recent`, the caller supplies `{files, baseSHA}`; per file emit `git -C <projectPath> diff <baseSHA>..HEAD -- <file>`. This module never re-derives the base SHA — it consumes the one the caller chose, so manifest and diff are guaranteed to come from the same commit set. For `paths` and `full`, embed file content up to `prompt.per_file_bytes` per file and `prompt.total_scope_bytes` overall, marking truncations and remaining files as manifest-only with a `truncated: N→K bytes` annotation. Both byte caps come from the global config (`internal/config/global.go`); defaults 8 KiB / 256 KiB. Tested in Phase 4 with a fixture worktree path; wired to the live dispatcher in Phase 5.

**Verify.** A fixture harness assembles a prompt against a fixture worktree path, a fixture BoK on disk, and a seeded prior-findings context with all four disposition states. **Confirm the assembled prompt contains every section (role+goal, BoK slice, project context, scope content with manifest, prior findings with note history, output schema, budget); the dummy reviewer round-trips a canned response that validates against the schema; bad output (missing required field, extra `severity` value) is rejected with a clear error**.

---

### Phase 5 — Dispatcher run + worktree + codex adapter + force-run

**Goal.** A single pass fires end-to-end against codex (or the dummy reviewer), produces findings, normalizes them into the persisted Finding shape, writes the frozen run artifacts, and cleans up the worktree. Driven by a force-run subcommand. Still no scheduler.

**Lands.**
- `internal/reviewer/codex/codex.go` — port mercurius's pattern from `internal/reviewer/codex/codexReviewer.go`. Flags: `codex exec -C <project_dir> --ephemeral --skip-git-repo-check --sandbox read-only --output-schema <schema.json> --output-last-message <last.txt> [-m <model>] [extra_args]`. Prompt via stdin. Ephemeral codex home (mirror `prepareCodexHome()`). The `-C` argument is the per-run worktree path, not the upstream clone.
- `internal/dispatcher/worktree.go` — `CreateWorktree(projectPath, capturedSHA, scratchPath) error` and `RemoveWorktree(projectPath, scratchPath) error` wrappers around `git worktree add/remove`. Plus `CaptureHEAD(projectPath) (string, error)` for `git rev-parse HEAD`.
- `internal/dispatcher/scope.go` — resolves a pass's `scope.project` into a concrete file set **plus any auxiliary data the prompt assembler needs to stay consistent** (notably the recent-scope base SHA so the manifest and the inline diff cannot diverge). Operates on the worktree path. `full` walks the project tree honoring the project's ignore rules (use `.gitignore` semantics via `go-git` or shell-out to `git ls-files`). `paths` resolves each entry using the three-rule order from the spec: if it `Stat`s as a directory → recursive expansion via the same `git ls-files` call used by `full` rooted at that directory; else if the entry contains any of `*`, `?`, `[` → `filepath.Glob` (or `doublestar.Glob` if `**` support is wanted); else → treat as a single literal file path. `recent` resolves in **one git pass**: `git -C <worktree> log --first-parent --since=<window-start> --pretty=%H HEAD` (newest-first) returns the in-window first-parent commits; if empty, the recent scope is empty (no files, no base SHA — `scope_content.go` produces no diff and `run.go` short-circuits per spec). Otherwise `C_oldest` is the last line. The base SHA is `C_oldest^1` if it has a parent, else the empty-tree SHA `4b825dc642cb6eb9a060e54bf8d69288fbee4904`. The file list is `git diff --name-only <base>..HEAD`. Both the file list and the base SHA are returned to the prompt assembler so `prompt/scope_content.go` (Phase 4) uses the *same* base when emitting per-file diffs — manifest and diff cannot disagree. `window-start = now - scope.project.window` in UTC; the config loader rejects `type: recent` without an explicit `window` field. Uncommitted working-tree changes are intentionally not included (see spec). `full` and `paths` return just `[]string` (no base SHA); `recent` returns `{files []string, baseSHA string}`.
- `internal/dispatcher/run.go` — one pass execution. **Worktree setup at run start**: `CaptureHEAD(project.path)` → `capturedSHA`, assign the run ID (`<project>/<pass>/<YYYY-MM-DD>/<HHMMSSZ>-<NNN>`; the dispatcher claims the `NNN` sequence inside the per-project critical section), then `CreateWorktree(project.path, capturedSHA, <state_dir>/scratch/<run_id>/)`. The worktree path becomes the working directory for every subsequent operation in this run (scope resolution, file content reads for prompt inlining, the reviewer subprocess's `-C` argument). A `defer` in the goroutine wrapper runs `RemoveWorktree` regardless of how the run exits. Resolve scope using the worktree path. **Empty-recent short-circuit**: if `scope.project.type == recent` and the resolver returned zero files (empty first-parent selection), skip BoK retrieval, prompt assembly, reviewer invocation, and normalization entirely. Write a minimal run directory: `prompt.md` is a one-line header noting "no commits in window," `output.json` is `{}`, `findings.json` is `[]`, `git-head.txt` carries `capturedSHA`, `report.md` is rendered as a no-op report ("no commits landed on first-parent line in the window"). No `finding_*` events appended. No mattermost message posted. `last-run.json` is already written at dispatch start so cadence advances. Worktree is still removed via the defer. Then return. Otherwise proceed normally: retrieve BoK slice, assemble prompt (using Phase 4's `prompt.go`; pass worktree path as the reviewer's working directory), invoke reviewer, validate output against the **reviewer output schema** (lean shape from Phase 4's `schema.go`), **normalize** each reviewer-output entry using the two-branch rule from the spec:
  - **Fresh ID** (reviewer omitted `id` or supplied one that does not match prior-findings context): allocate a new canonical ID, write a new persisted Finding with `created_at = now`, `first_run_id = last_run_id = <current run>`, `disposition: "open"`, append `finding_created` event.
  - **Existing ID** (supplied `id` matches prior-findings context): load the existing Finding, update only `last_run_id`, leave `id` / `created_at` / `first_run_id` / `disposition` untouched, append `finding_reobserved` event.
  
  Then persist run artifacts to `runs/<date>/<pass>/<HHMMSSZ-NNN>/{prompt.md,output.json,findings.json,report.md,git-head.txt}` (output.json is audit-only — the raw reviewer output, unchanged; `findings.json` is the frozen manifest of canonical IDs surfaced by this run with their disposition snapshot at completion time), write/update the persisted Findings into `state/projects/<project>/findings/`, render `report.md` from the persisted Findings *at this moment* and write it once into the run directory (never re-rendered later), render `backlog.md`. The full sequence — normalize, allocate, append events, write files, render — happens inside the per-project write lock so the frozen artifacts and the live state commit together.
- `otis pass run <project>/<pass>` subcommand for force-runs. Force-runs read the resolved config once at invocation time (no dispatcher loop yet); load the global config, resolve the named project's composed config, fire `run.go` synchronously, exit with the run-ID printed.

**Verify.** Configure a tiny test project with a `vocabulary-sweep` pass scoped to `paths: ["sample/"]`. Run `otis pass run testproj/vocabulary-sweep --reviewer dummy`. **Confirm a fresh worktree is created at `<state_dir>/scratch/<run_id>/`, scope and prompt are resolved against it, `runs/.../prompt.md` contains the BoK slice and the prior findings context (with all four disposition states represented when seeded — open, accepted, deferred, rejected); `output.json` validates against the schema; new finding JSON files land under `findings/`; `dispositions.jsonl` grows; `backlog.md` renders; the worktree is removed at run end**. Then run again with `--reviewer codex` against a local codex install to confirm the live adapter works. Force-fire an empty-`recent` pass and confirm the no-op artifacts.

---

### Phase 6 — Scheduler, windows, concurrency caps

**Goal.** Supervisor loop fires due passes inside their window, respecting per-reviewer and global caps. Cadence honored across restarts.

**Lands.**
- `internal/state/lastrun.go` — per-project `projects/<project>/last-run.json` persistence keyed by `<pass>`, atomic temp-rename. Sharded so the existing project write lock fully covers the read-modify-write — no global mutable state file. Written at **dispatch start** (after semaphore acquisition, before the reviewer subprocess is launched). Not updated on completion. Failed runs still consume the cadence cycle; cadence governs firing rhythm, not delivery — retries are the operator's job via force-run. The scheduler's due-list pass reads N project files instead of one global file; cheap, and removes the cross-project locking concern.
- `internal/state/events.go` — `supervisor/events.jsonl` for lifecycle (start, stop, pass dispatch).
- `internal/scheduler/windows.go` — window membership (`time.Now().Local()` clock injected for tests; supervisor-host-local timezone is the contract — see spec). Endpoint parser accepts `HH:MM` with `24:00` as an exclusive end-of-day sentinel (so `00:00-24:00` is a valid full-day window). Cross-midnight windows: when `end < start` (e.g., `22:00-06:00`), membership is `now ≥ start || now < end`; otherwise `now ≥ start && now < end`. Both branches share one helper with a table-driven test covering same-day, cross-midnight, exact-boundary, and full-day (`00:00-24:00`) cases, plus a timezone-injection test that confirms windows are evaluated against local time even when UTC offset differs.
- `internal/scheduler/scheduler.go` — every N seconds (configurable, default 30): build a due-list — `now - last_run >= cadence` AND `now` ∈ pass's reviewer window. Spread firings (jitter or a simple round-robin order) so cadence cohorts don't bunch at startup. No BoK index sync — passes read the BoK fresh from disk via `internal/bok/resolve.go` at prompt-assembly time, so new entries that sexton lands are visible immediately.
- `internal/dispatcher/dispatcher.go` — per-reviewer semaphore + global semaphore plus an in-memory `inFlight map[passKey]*inFlightEntry` guard guarded by its own `sync.Mutex` (separate from the per-project state lock — this guard is dispatcher-internal). `inFlightEntry{state: "queued"|"running", runID string}`. `Enqueue(project, pass, source: scheduled|force)`: lock inFlight; if `(project, pass)` already present, scheduled callers get a no-op return; force-run callers get an `ErrInFlight{State, RunID}` that the REST handler maps to HTTP 409 with body `{state, run_id}` (run_id is `null` while state is `queued`). Else insert a `{state: "queued", runID: ""}` entry, unlock, acquire reviewer semaphore (blocking), acquire global semaphore (blocking). **Post-wait window re-check (scheduled runs only)**: after both semaphores are held, re-evaluate `scheduler.windows.InWindow(reviewer.window, now())`. If the window has closed during the wait, release both semaphores, remove the inFlight entry, and return without allocating a run ID or writing `last-run.json` — the pass stays due. Otherwise (window still open, or force-run source which skips this check) take the project write lock, allocate the run ID, write `last-run.json`, drop the project lock, mutate the inFlight entry to `{state: "running", runID: <id>}`, launch `dispatcher/run.go` in a goroutine. The goroutine wraps the run body in a `defer` that removes the inFlight entry and releases both semaphores — so the entry survives until the run truly completes (success, failure, or panic). This guarantees at most one active run per `(project, pass)`; the prior-findings context cannot be read by two concurrent runs of the same pass. `last-run.json` is still written at dispatch start so failed/crashed runs consume cadence. Force-runs go through the same path with cadence/window eligibility checks skipped at both queue time and post-wait. Mirror mercurius's `executeRoundJob` fire-and-forget shape, but wrap with the caps and the in-flight guard that mercurius doesn't have yet.
- `otis serve` now actually starts the scheduler loop: watches global config for project paths, surfaces sexton-missing as a warning (per spec section 9 — "fail loud, keep running"). **Orphan worktree cleanup at startup**: per supervised project, run `git -C <project.path> worktree prune`; then scan `<state_dir>/scratch/` for directories whose run IDs are not in the (freshly empty) `in_flight` map and `os.RemoveAll` each. Logs each removal at info level. Crash recovery is automatic; no leaked worktrees.
- A `--once` flag on `otis serve` for tests / CI: do one due-list pass and exit.

**Verify.** Two passes configured: a `vocabulary-sweep` on `cadence: 1m` and a no-op pass on `cadence: 5m`. Run `otis serve --once` repeatedly, advancing a clock fixture; **assert that the 1m pass fires every iteration and the 5m pass only fires when due, both staying inside their declared windows, and that concurrency caps gate truly-parallel invocations**.

---

### Phase 7 — HTTPS REST API + bearer auth + workstation client foundation

**Goal.** The endpoints in spec section 8.1 work; auth gates them; clients can be remote. Workstation client foundation (config loader + authed HTTP client) lands here so Phase 8's MCP bridge can use it.

**Lands.**
- `internal/api/auth.go` — token store loaded from `<state_dir>/tokens/` (one file per token; optional label inside; mtime is rotation signal). `Authorize(r *http.Request) (label, ok)`.
- `internal/api/router.go` — stdlib `net/http` + `http.ServeMux` (Go 1.22+ method-aware patterns). No new dependency.
- `internal/client/config.go` — workstation config loader for `~/.config/otis/config.yaml` (supervisor URL + bearer token), `dd`-bound with the same defaults/validate/resolve shape as the server configs. Used by both Phase 8 (MCP bridge) and Phase 9 (CLI commands).
- `internal/client/http.go` — small authed `http.Client` wrapper that adds the bearer token to every request, handles TLS (trusts the supervisor's self-signed cert when configured), and unmarshals JSON responses. One shared dependency for every client subcommand and the MCP bridge.
- `internal/api/handlers.go` — the seven endpoints from spec section 8.1. The runs-report endpoint shape is `GET /projects/{project}/runs/{pass}/{date}/{time_seq}/report` (matches the run-ID layout); finding endpoints split the canonical ID across path segments — `GET /projects/{project}/findings/{pass}/{seq}` and the disposition POST under the same prefix. The path-segment split is what makes both IDs URL-safe without percent-encoding.
- `internal/api/render.go` — the **rendering function** used at run-completion to produce `report.md` from a snapshot of persisted Findings + the run manifest. Called once by `internal/dispatcher/run.go` inside the project write lock; not called by the API. The runs-report endpoint serves the stored `report.md` file directly (no re-render). Raw `output.json` stays in `runs/.../` as audit only and is never the source of truth for what a human or downstream agent reads. Mattermost links (Phase 10) point at the stored markdown via the API endpoint.
- TLS: serve over TLS using a key/cert pair from the global config (`api.tls.cert`, `api.tls.key`); self-signed acceptable for HQ network.
- `otis admin token issue [--label X]` subcommand — generates a random 32-byte token, writes it to the store, prints it once.

**Verify.** `otis serve` starts the API on the configured port. **`curl -H "Authorization: Bearer ..." https://localhost:.../api/v1/projects` returns the configured projects; unauthenticated requests get 401; posting a disposition flips state and re-renders `backlog.md`; `POST /passes/{pass}/run` enqueues a force-run that the dispatcher picks up**.

---

### Phase 8 — MCP server

**Goal.** Digital clients reach the same surface through MCP.

**Lands.**
- `internal/mcp/bridge.go` — `otis mcp` runs as a **stdio bridge**, not as a second supervisor. It speaks MCP to the workstation's MCP client on stdin/stdout and forwards each tool invocation as an authenticated HTTPS request to the running supervisor via the shared `internal/client/{config,http}.go` foundation that landed in Phase 7 — the same code path the `otis findings list / show / accept / defer / reject` CLI commands will use in Phase 9. **The bridge never opens the state store directly; the supervisor process remains the sole writer**, so the per-project locks from Phase 2 / cross-cutting concerns still hold across any number of MCP clients on any number of workstations.
- `internal/mcp/tools.go` — four tools registered via `modelcontextprotocol/go-sdk`: `otis_list_findings`, `otis_get_finding`, `otis_get_report`, `otis_disposition_finding`. Each tool's handler is a small wrapper that builds the matching REST request, sends it through the workstation's authenticated HTTP client, and translates the response back into MCP shape.
- `otis mcp` subcommand wires the bridge to the standard input/output. The supervisor's own HTTPS endpoints (built in Phase 7) are the source of truth; no MCP listener runs inside `otis serve` in the minimal harness.
- Workstation MCP-client config example shipped as `docs/example/mcp.json`.

**Verify.** Configure Claude Code in this repo to point at `otis mcp`. **From a Claude Code session, call `otis_list_findings` with `{"project": "testproj", "disposition": "open"}` and walk through scenario three in the spec end-to-end**.

---

### Phase 9 — Workstation CLI client (full set)

**Goal.** The full `otis` CLI from spec section 8.2 works against the remote supervisor.

**Lands.**
- Client subcommands built on the Phase 7 foundation (`internal/client/{config,http}.go`): `otis findings list`, `otis findings show`, `otis accept`, `otis defer`, `otis reject`, `otis report show <run-id>` (run ID is `<project>/<pass>/<date>/<time_seq>`), `otis projects list`, `otis passes list`, `otis pass run`.
- Same binary as the server (mode is determined by subcommand).
- Pretty-print findings using a small markdown→terminal renderer (or just plain text; whichever is cheaper).

**Verify.** **Run scenario two from the spec end-to-end**: trigger a force-run on the supervisor, see findings created, `otis accept FINDING-ID --note "..."` flips disposition over HTTPS, server log shows the event written, `otis findings list --open` no longer shows that finding.

---

### Phase 10 — Notification + remaining reviewer adapters

**Goal.** Mattermost posting on non-empty runs; claude-code and pi reviewers usable in passes.

**Lands.**
- `internal/notify/notify.go` — `Notifier` interface, `Post(ctx, Notification) error`.
- `internal/notify/mattermost/mm.go` — `MATTERMOST_TOKEN` env, `Notification` struct shaped to render the message in spec section 10. One message per non-empty run; suppress on zero findings. Report links use `notification.report_base_url` from the global config (added to `internal/config/global.go` in this phase); when absent, fall back to deriving from `api.listen` and log a warning that links may not resolve externally.
- Channel resolution: explicit on project, else `#otis-<project-name>`.
- `internal/reviewer/claudecode/cc.go` — `claude -p ... --output-format json --json-schema <path> --bare --allowedTools "Read,Glob,Grep,LS" --permission-mode plan -m <model>`. Prompt via stdin or `-p` arg.
- `internal/reviewer/pi/pi.go` — `pi -p ... --mode json`, configured for read-only tool access via pi's permission-gate extension. Defer to pi's CLI docs for the exact flag set; this adapter is the riskiest because pi's harness conventions are least-settled — wire it cautiously and add adapter-level tests with a fake `pi` binary on `PATH`.
- A `--dry-run` flag on each adapter that prints the assembled invocation without firing it.

**Verify.** Send a finding-producing run; **a single mattermost message lands in the configured channel with the spec's format**. Run a pass against the codex / claude-code / pi reviewers with the same scope and confirm each produces a schema-valid output and a populated `runs/.../report.md`.

---

### Phase 11 — End-to-end demo, seed BoK, doc migration

**Goal.** Move from buildable to demoable, with a story Michael can run on his workstation.

**Lands.**
- `docs/example/` — example global config, example project config, an example tiny BoK (4–6 entries covering vocabulary, layering, cognitive-load — already on disk, no index step required), example mattermost message.
- `README.md` filled out with the demo walkthrough.
- `docs/current/` — first migration of built behavior from `docs/future/` per the design-build pipeline. The spec stays in `docs/future/` until everything described there is built; this phase moves the surfaces that *are* built into `docs/current/`.
- A small smoke-test harness that runs the dummy reviewer end-to-end as part of `go test ./...` to keep the integration glued.

**Verify.** A new collaborator (or a fresh agent), starting from `README.md`, can stand up otis, run a single pass against a test project on a mock BoK, see the finding land in `backlog.md`, accept it via CLI, and watch the next pass not re-surface it. **Scenarios one, two, and three from the spec all play through cleanly.**

---

## Cross-Cutting Concerns Spelled Out

**Sexton handshake.** Otis never syncs or pulls supervised repos — sexton owns that. Otis does invoke local git commands against each project's clone: `rev-parse HEAD` to capture the SHA at run start, `worktree add/remove` per run, and `worktree prune` at supervisor startup. The global config lists project paths; the supervisor reads them as-is. If a path is missing, log a warning and skip that project's passes. The spec is explicit: "fail loud, keep running." Sexton's "watching" state is the implicit handshake — we do not poll it, we just trust that sexton's last sync delivered a reasonable object database. The captured-SHA + worktree mechanism (see Phase 5 and spec "Repository Management") is what makes `git-head.txt` an exact audit-trail commitment regardless of what sexton does to the upstream mid-run.

**Cross-run finding identity.** No content-hash dedupe in the minimal harness (deferred per spec). The mechanism is the **prior findings context** in the prompt (spec section "Reviewer Interface" item 5): every non-archived finding for the same `(project, pass)` is included with its description, location, current disposition, and human note. The prompt copy tells the reviewer how to read each state — reference the existing ID for `open` reoccurrences, do not re-surface `accepted` / `deferred` / `rejected` items unless the code shows the basis has changed. The codex adapter tests exercise all four disposition states.

**Permission discipline.** Reviewer adapters enforce read-only invocation per spec section 5.3. The codex adapter uses `--sandbox read-only`; claude-code uses `--permission-mode plan` and an explicit `--allowedTools` allowlist limited to read tools; pi uses its read-only configuration. The supervisor itself reads code from filesystem paths — it does not exec anything inside the project tree.

**Concurrency caps.** Per-reviewer semaphores from `reviewers.<name>.concurrency_cap`; a global semaphore from `global_concurrency_cap`. Both held while a run is in flight; released when the run goroutine returns. Mercurius's broker doesn't have these yet — this is the place otis genuinely diverges in implementation rather than mirroring.

**State mutation invariant (per-project lock).** Per spec section "State Mutation Invariant," every mutation for a project — ID sequence allocation, `dispositions.jsonl` append, finding-cache write, `backlog.md` render, run-ID `NNN` allocation — happens under one `sync.RWMutex` keyed by project name, owned by `internal/state/store.go`. Dispatcher and API both go through the state package; no call site outside `internal/state/` writes state files directly. Reads acquire the read lock and see a consistent post-write view. Per-project so unrelated projects don't serialize on each other. Cross-process file locking stays deferred (single-supervisor assumption).

**Top-N discipline.** Implemented in the prompt template (per-pass instruction to return only the best N), enforced in the schema (`maxItems: N` on the findings array), validated on output. The default at the project level is 3, override per-pass.

**Logging.** Global `dl.Log()` by default (no preemptive per-subsystem channels — that's not the practice convention). Per-call structured fields via `.With("project", p).With("pass", n)` chains. `--verbose` flips the level to debug via cobra `PersistentPreRun`. Channels introduced lazily if a subsystem ever needs separate output routing.

**Configuration reload.** Mercurius re-reads calibration fields per session. Otis follows the same discipline: at every scheduler tick the supervisor re-resolves each project's composed config — that means re-reading the global config (cheap, mtime-cached), the per-project `otis.yaml`, **and any shared profiles the project's `include_configs` list pulls in from the BoK root**. So editing `<bok.path>/standard.yaml` and waiting one tick is enough; no restart required. The same path catches sexton-landed BoK content updates implicitly because the BoK is read from disk per-pass (there is no index to invalidate). A `SIGHUP` handler that forces an immediate reload is a nice-to-have, not load-bearing for phase 6.

## Risks & Watch-Items

- **Pi adapter is the unknown.** Pi's CLI conventions are not as nailed down as codex's. Phase 10 may surface adapter-shape questions that bubble back to the `Reviewer` interface. Acceptable risk — defer until phase 10, treat as discovery.
- **MCP SDK churn.** `modelcontextprotocol/go-sdk` is recent. Pinning a specific version (matching mercurius's pin where possible) keeps surprises manageable.
- **JSON-schema strictness vs reviewer compliance.** Mercurius's bug fixes for reviewer output drift are valuable prior art; mirror them rather than rediscover. Watch `internal/schema/reviewOutput.go` in mercurius for any patterns worth porting.

## Critical Files (Net-New, Anchored)

| Path | Pattern source |
|---|---|
| `cmd/otis/main.go` | mercurius `cmd/mercurius/main.go` |
| `internal/config/{global,profile,project,compose,load}.go` | mercurius `internal/config/config.go`, lore `internal/config/load.go` |
| `internal/state/{paths,store,findings,dispositions,backlog,runs,lastrun,events,scratch}.go` | mercurius `internal/monitor`, `internal/roundlog` |
| `internal/bok/{entry,resolve,frontmatter}.go` | lore `internal/markdown/frontmatter.go` (frontmatter only; index/embedding pattern deferred) |
| `internal/reviewer/{reviewer,dummy/dummy,codex/codex}.go` | mercurius `internal/reviewer/codex/codexReviewer.go`, mercurius `internal/reviewer/dummy/` |
| `internal/prompt/{prompt,scope_content,schema}.go` | mercurius `internal/prompt/prompt.go`, `internal/schema/reviewOutput.go` |
| `internal/scheduler/{scheduler,windows}.go` | new (no direct anchor; cadence/window combination is otis-specific) |
| `internal/dispatcher/{dispatcher,run,scope,worktree}.go` | mercurius `internal/broker/broker.go` (executeRoundJob + persistSessionLocked) |
| `internal/api/{router,auth,handlers,render}.go` | new (stdlib mux + file-backed bearer tokens) |
| `internal/client/{config,http}.go` | new (workstation foundation shared by CLI commands and MCP bridge) |
| `internal/mcp/{bridge,tools}.go` | mercurius `internal/mcpserver/mcpServer.go` (registration pattern only; otis uses HTTPS forward) |
| `internal/notify/notify.go`, `internal/notify/mattermost/mm.go` | new |

## Verification (End-to-End)

Once phase 11 lands, the following sequence should work cold:

1. `otis serve --config docs/example/global.yaml &` — supervisor starts; scheduler quiet because cadences haven't elapsed.
2. `otis admin token issue --label workstation` — print a token; copy into `~/.config/otis/config.yaml`.
3. `otis pass run testproj/vocabulary-sweep --reviewer codex` — force-fire; codex runs against a clean worktree pinned to HEAD; findings land; mattermost message posts; backlog updates.
4. `otis findings list --project testproj --open` — see the new findings.
5. `otis accept <FINDING-ID> --note "good catch"` — disposition flips.
6. Rerun `otis pass run testproj/vocabulary-sweep` — prompt includes the accepted finding in the prior findings context; the reviewer doesn't re-surface it.
7. From Claude Code, call `otis_list_findings` over MCP — same data, same view.

That's the heartbeat in microcosm. Everything after is calibration of the BoK against real codebases via the harvest practice (out-of-band, per the harvest agent guide).

## Mercurius Review Plan

Once this work order is committed alongside the spec, fire mercurius. Both `docs/future/otis-spec.md` and `docs/future/otis-work-order.md` go into the review session as the artifact set. The `mercurius.yaml` already in the repo is correctly configured (codex reviewer, 6 findings, terse review_context). Drive rounds to convergence — findings stop being load-bearing for implementation — then begin phase 1.

## Deferred (Reaffirmed)

Spec section 13 names the deferrals; this work order does not reintroduce any of them. Specifically out of scope for this build: higher permission gradient levels, event-triggered passes, continuous-mode passes, pass-declared review refs (PR/branch/tag review), AST-level location anchors, content-hash dedupe, operator-action MCP tools, severity-hint BoK frontmatter, per-finding-type permission gradients, in-project BoK augmentation, embedding index + semantic search, and a codified harvest ritual.
