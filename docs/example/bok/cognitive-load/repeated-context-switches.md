---
title: repeated context switches
tags: [cognitive-load]
---

# Repeated Context Switches

Prefer code paths where a reviewer can keep one working model in mind. A flow that
requires bouncing between command parsing, config composition, state mutation, and
transport details should have a small boundary object or helper that names the
operation.

This is not a license to hide simple code. Add the boundary only when it removes
real reading cost from a workflow people revisit often.
