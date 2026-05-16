# 3. Body of Knowledge

The body of knowledge (BoK) is the part of Otis you author. Without a real
BoK, Otis is a generic smell-hunter; with a calibrated one, it becomes the
access pattern for your team's accumulated architectural perspective.

This chapter covers BoK structure, entry format, include syntax, project
scoping, and the relationship between BoK content and BoK configuration.
For the editorial *practice* of harvesting a BoK from an existing
codebase, see [../harvest-agent-guide.md](../harvest-agent-guide.md).

## What Lives in a BoK

A BoK is a directory tree on disk. Otis reads it directly at pass time —
no embedding index, no cache to refresh.

Two things live in the BoK root:

1. **Knowledge entries**, organized under category subtrees.
2. **Shared pass profiles**, as root-level YAML files like
   `standard.yaml`.

Knowledge entries are markdown files under category subtrees such as
`vocabulary/`, `layering/`, and `cognitive-load/`. Root-level markdown is
not treated as BoK content. The example BoK from the demo shows the
shape:

```
bok/
├── cognitive-load/
│   └── repeated-context-switches.md
├── layering/
│   └── internal-boundaries.md
├── naming/
│   └── lens-vs-view.md
├── projects/
│   ├── baab/
│   │   └── established-conventions.md
│   └── testproj/
│       ├── established-conventions.md
│       └── otis.yaml
├── standard.yaml
└── vocabulary/
    ├── lens-vs-view.md
    └── library-overloads.md
```

`projects/<name>/` is reserved for project-scoped guidance. Entries under
that subtree are only ever included when resolving for project `<name>`.
A per-project `otis.yaml` lives there too — see
[04-configuration.md](04-configuration.md).

Category names are not enforced. Pick the categories that make sense for
your practice. The calibration target is reducing cognitive load when
reading code, not generic correctness.

## Writing an Entry

An entry is a markdown file with optional YAML frontmatter and prose body.
The example BoK uses a simple shape:

```markdown
---
title: lens vs view vocabulary preference
tags: [vocabulary, naming]
created: 2026-05-13
---

# lens vs view vocabulary preference

Across the codebases, lens is the established term for a perspectival or
filtered view of data. Some subsystems drift into using view for the same
concept.

## guidance

Prefer lens over view in new code for perspectival data surfaces. Flag
view only when it is being used in that lens-like sense.

## why

Vocabulary consistency reduces cognitive load when moving across projects.
```

What works well in practice:

- A short body that names the concept.
- A `## guidance` section the reviewer can act on.
- A `## why` section so the entry survives editorial review later.
- Examples or anti-examples if they help a reviewer pattern-match.

What to avoid:

- Generic catalog entries copied from style guides.
- Style or formatting concerns that linters already enforce.
- Rules the codebase happens to exhibit but you do not actually endorse.

See [../harvest-agent-guide.md](../harvest-agent-guide.md) for a longer
treatment of what is worth writing down.

## BoK Includes

A pass declares the slice of the BoK it cares about via
`scope.bok.include`. The include syntax is deliberately small:

- **Trailing-slash directory** — `vocabulary/` includes every entry under
  that directory.
- **Explicit file path without `.md`** — `vocabulary/lens-vs-view` includes
  that one entry.

The demo's `standard.yaml` profile uses the directory form:

```yaml
passes:
  - name: vocabulary-sweep
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

Bare terms (no slash, no path) are rejected today. They are reserved for a
future semantic-search extension and currently fail validation rather than
silently match.

Entries under `projects/<name>/` are auto-scoped: they are included only
when resolving for project `<name>`, regardless of what your include list
says. You do not need to add `projects/testproj/` to a testproj include —
it happens automatically.

## Shared Pass Profiles

A *profile* is a root-level YAML file in the BoK that declares one or more
passes. Projects pull profiles in with `include_configs`:

```yaml
# projects/testproj/otis.yaml
include_configs:
  - standard

project:
  name: testproj
  description: small demonstration project for the Otis harness
  primary_language: go

passes:
  - name: vocabulary-sweep
    top_findings: 2
```

Composition rules:

- Profiles are loaded in the order listed.
- A pass name may appear in only one profile — cross-profile collisions
  are rejected.
- A project's `passes[]` entries are merged onto profile passes by name,
  so a project can override `top_findings`, swap reviewer kind, or tighten
  scope without re-declaring the whole pass.
- A project can disable inherited passes with `disable`.

The discipline this encourages: keep the *shape* of common passes
(scope, BoK slice, cadence) in a shared profile, and let each project
tune only what is project-specific.

## Where to Put a BoK

The BoK directory is named in the global config's `bok.path`. In the demo
it sits next to the global config; in production it is usually a
checked-out repository (`otis-bok` is a common name) that lives on the
supervisor host. Otis never modifies the BoK directory; it only reads
from it. You version, review, and deploy BoK changes the same way you do
any other repo, and restart the supervisor after a change because config
reload is not implemented today (see [../deferred.md](../deferred.md)).

---

Next: [04-configuration.md](04-configuration.md) — wire global config,
project config, passes, scopes, cadence, and windows together.
