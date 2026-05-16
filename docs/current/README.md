# Current Otis Behavior

These pages describe the behavior implemented through the completed
Phase 1-11 work order. They are the docs root for everything *currently
true* about Otis; specs and work orders for future states live in
`docs/future/`.

## Start Here

- [guide/](guide/) — the user-facing walkthrough. Mental model, demo,
  body of knowledge, configuration, reviewers, deployment, and day-to-day operation. New users should read this first.

## Reference

The contract notes below back up the guide; consult them when precision
matters.

- [configuration.md](configuration.md) covers global config, project
  config, shared pass profiles, and body-of-knowledge resolution.
- [operations.md](operations.md) covers the supervisor, scheduling, dispatch, reviewer adapters, state files, notifications, and the demo
  scenarios.
- [api.md](api.md) covers HTTP(S), CLI, and MCP surfaces.
- [invariants.md](invariants.md) captures operational contracts that
  should not drift accidentally.
- [deferred.md](deferred.md) lists intentionally unbuilt capabilities and
  current gaps.
- [harvest-agent-guide.md](harvest-agent-guide.md) orients BoK harvest
  agents.

The runnable example is under [`../example/`](../example/).
