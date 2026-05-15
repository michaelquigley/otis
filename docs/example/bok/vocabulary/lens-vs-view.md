---
title: lens vs view vocabulary preference
tags: [vocabulary, naming]
created: 2026-05-13
---

# lens vs view vocabulary preference

Across the codebases, lens is the established term for a perspectival
or filtered view of data. Some subsystems drift into using view for the
same concept.

## guidance

Prefer lens over view in new code for perspectival data surfaces. Flag view
only when it is being used in that lens-like sense.

## why

Vocabulary consistency reduces cognitive load when moving across projects.
