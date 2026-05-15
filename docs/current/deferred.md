# Deferred Work

These capabilities are intentionally not part of the current harness.

- Higher permission levels: proposed diffs, autonomous remediation, and
  continuous autonomous repair.
- Event-triggered passes such as commit, branch, or PR events.
- Continuous-mode passes that immediately restart after each run.
- Pass-declared review refs for PR branches, tags, or arbitrary commits.
- AST-level location anchors beyond file and line ranges.
- Content-hash dedupe for findings. Current identity depends on prior context and
  reviewer-supplied existing IDs.
- Operator-action MCP tools such as force-running a pass or reloading config.
- Expanded BoK frontmatter such as `severity_hint`.
- In-project BoK augmentation under a repository-local `.otis/bok/`.
- Per-finding-type permission gradients.
- Embedding indexes and semantic BoK search. Bare include terms are reserved for
  this future path and rejected today.
- A codified operational harvest ritual beyond the guide in
  `harvest-agent-guide.md`.

## Current Gaps

The completed work order described a future config-reload discipline where the
scheduler would re-resolve global, project, and profile config every tick. The
implemented supervisor currently loads a resolved config at process start and
holds it for the lifetime of the process. Restart the supervisor after config
changes.

The Pi adapter is intentionally thin because the Pi CLI contract is less settled
than Codex and Claude Code. Treat real Pi usage as an adapter-calibration pass.
