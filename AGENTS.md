# Otis

Otis is a continuous code-quality agent. The Phase 1-11 work order is complete;
current behavior and operating contracts live in `docs/current/`.

## Current Implementation Rules

- Use Go 1.26+.
- Use `github.com/spf13/cobra` for the CLI.
- Use `github.com/michaelquigley/df/dl` directly for logging; initialize it in `cmd/otis/main.go` with trim prefix `github.com/michaelquigley/otis/`.
- Use `github.com/michaelquigley/df/dd` for YAML binding and struct/map merge behavior.
- Prefer defaults through constructors, then `dd.MergeYAMLFile`, then `Validate()`, then `Resolve()`.
- Keep config loading side-effect-free; commands that need writable directories should ensure them explicitly in the later phase that writes.
- Follow the Mercurius idioms in `../mercurius`: cobra root shape, reviewer adapter boundaries, schema validation outside adapters, immutable logs, and atomic writes.

## Documentation

- `docs/current/` is the docs root for everything currently true about Otis.
  - `docs/current/guide/` is the user-facing walkthrough (concepts, quickstart,
    BoK, configuration, reviewers, deployment, day-to-day operation).
  - The sibling notes (`configuration.md`, `operations.md`, `api.md`,
    `invariants.md`, `deferred.md`, `harvest-agent-guide.md`) are the
    reference layer; keep guide prose and reference contracts in sync.
- `docs/future/` is reserved for specs and work orders describing future states.
- `docs/example/` contains the runnable demo BoK, project, and config.
- Keep `README.md` concise and point to `docs/current/guide/` and the demo.
