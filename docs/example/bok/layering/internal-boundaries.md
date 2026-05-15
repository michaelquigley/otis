---
title: internal package boundary discipline
tags: [layering]
created: 2026-05-13
---

# internal package boundary discipline

Internal packages should keep dependency direction obvious. Low-level packages
should not reach up into command, API, or orchestration layers.

## guidance

Flag dependencies that make a leaf package know about its caller.
