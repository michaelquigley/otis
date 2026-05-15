# Otis

Otis is a continuous code-quality agent. The current repository is in the first implementation phase: command scaffolding and configuration loading.

The design contract lives in `docs/future/otis-spec.md`; the phased build order lives in `docs/future/otis-work-order.md`.

## Phase 1 Commands

```bash
go run ./cmd/otis version
go run ./cmd/otis config check docs/example/global.yaml
go run ./cmd/otis --config docs/example/global.yaml serve
```

The `serve` command is a Phase 1 stub. It loads the composed global/project configuration and logs where the supervisor will start in later phases.
