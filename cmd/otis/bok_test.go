package main

import (
	"strings"
	"testing"
)

func TestBokCommands(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root+"/vocabulary/lens-vs-view.md", `
---
title: lens vs view
tags: [vocabulary]
---

body
`)
	writeTestFile(t, root+"/naming/lens-vs-view.md", `
---
title: naming lens
tags: [naming]
---

body
`)
	writeTestFile(t, root+"/projects/baab/conventions.md", `
---
title: baab conventions
tags: [project]
---

body
`)
	writeTestFile(t, root+"/projects/lore/conventions.md", `
---
title: lore conventions
tags: [project]
---

body
`)
	writeTestFile(t, root+"/README.md", "# readme\n")

	listOut := runCommand(t, "bok", "list", "--bok-path", root)
	for _, want := range []string{"vocabulary/", "vocabulary/lens-vs-view", "projects/baab/conventions"} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("list output missing %q:\n%s", want, listOut)
		}
	}
	if strings.Contains(listOut, "README") {
		t.Fatalf("root README appeared in list:\n%s", listOut)
	}

	resolveOut := runCommand(t, "bok", "resolve", "--bok-path", root, "--include", "vocabulary/,naming/lens-vs-view,projects/", "--project", "baab")
	for _, want := range []string{"vocabulary/lens-vs-view", "naming/lens-vs-view", "projects/baab/conventions"} {
		if !strings.Contains(resolveOut, want) {
			t.Fatalf("resolve output missing %q:\n%s", want, resolveOut)
		}
	}
	if strings.Contains(resolveOut, "projects/lore/conventions") {
		t.Fatalf("foreign project entry appeared in resolve output:\n%s", resolveOut)
	}
}

func TestBokResolveBareTermError(t *testing.T) {
	root := t.TempDir()
	cmd := newRootCommand()
	cmd.SetArgs([]string{"bok", "resolve", "--bok-path", root, "--include", "vocabulary,naming", "--project", "baab"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected bare-term error")
	}
	if !strings.Contains(err.Error(), "bare-term entries are reserved") {
		t.Fatalf("unexpected error: %v", err)
	}
}
