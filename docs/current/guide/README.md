# Otis User Guide

This guide is the friendly path into Otis. It starts with the mental model,
takes you through the demo to confirm the basic loop, and then builds toward
a real configuration and deployment.

The sibling pages in `docs/current/` are the reference layer: terse contract
notes the guide links into when precision matters.

## Audience

You have a codebase (or several) that you want reviewed continuously
against a body of knowledge you control. You are comfortable on the command
line, can edit YAML, and either have one of the supported reviewer CLIs
installed (`codex`, `claude`, `pi`) or are happy to start with the deterministic dummy reviewer that ships with the demo.

## Prerequisites

- Go 1.26 or newer (Otis is built and installed with `go install`).
- `git`, in your PATH; Otis snapshots supervised repos through `git
  worktree`.
- Optional: a reviewer CLI for production use — `codex`, `claude`, or
  `pi`. The quickstart does not require any of these.
- Optional: a Mattermost workspace if you want pass notifications.

## Reading Order

1. [01-concepts.md](01-concepts.md) — vocabulary and mental model.
2. [02-quickstart.md](02-quickstart.md) — run the dummy-reviewer demo end
   to end and watch the loop work.
3. [03-body-of-knowledge.md](03-body-of-knowledge.md) — write the BoK
   entries Otis will review against.
4. [04-configuration.md](04-configuration.md) — global config, project
   config, shared profiles, scopes, cadence and windows.
5. [05-reviewers.md](05-reviewers.md) — choose and wire a real reviewer
   adapter.
6. [06-deployment.md](06-deployment.md) — install, run, secure, and notify.
7. [07-day-to-day.md](07-day-to-day.md) — triage, force-runs, MCP, audit.

If you already understand the moving parts and just want to run something,
jump straight to [02-quickstart.md](02-quickstart.md).

## Reference Layer

- [../configuration.md](../configuration.md) — the configuration contract.
- [../operations.md](../operations.md) — supervisor, dispatch, state,
  notifications.
- [../api.md](../api.md) — HTTP, CLI, and MCP surfaces.
- [../invariants.md](../invariants.md) — operational invariants that should
  survive refactors.
- [../deferred.md](../deferred.md) — capabilities intentionally not built
  yet.
- [../harvest-agent-guide.md](../harvest-agent-guide.md) — the BoK harvest
  practice.

The runnable example used throughout the guide lives in
[`docs/example/`](../../example/).
