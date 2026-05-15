# Otis

Otis is being built phase-by-phase from `docs/future/otis-work-order.md`, with `docs/future/otis-spec.md` as the behavioral contract. Stop at each phase boundary for human review and manual commit before continuing.

## Current Implementation Rules

- Use Go 1.26+.
- Use `github.com/spf13/cobra` for the CLI.
- Use `github.com/michaelquigley/df/dl` directly for logging; initialize it in `cmd/otis/main.go` with trim prefix `github.com/michaelquigley/otis/`.
- Use `github.com/michaelquigley/df/dd` for YAML binding and struct/map merge behavior.
- Prefer defaults through constructors, then `dd.MergeYAMLFile`, then `Validate()`, then `Resolve()`.
- Keep config loading side-effect-free; commands that need writable directories should ensure them explicitly in the later phase that writes.
- Follow the Mercurius idioms in `../mercurius`: cobra root shape, reviewer adapter boundaries, schema validation outside adapters, immutable logs, and atomic writes.

## Documentation

- `docs/future/` contains planned behavior.
- `docs/current/` should describe only implemented behavior once later phases create it.
- Keep `README.md` concise and point to the design/work-order documents.
