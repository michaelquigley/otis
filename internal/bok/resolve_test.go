package bok

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadEntryParsesFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeBokFile(t, root, "vocabulary/lens-vs-view.md", `
---
title: lens vs view
tags: [vocabulary, naming]
created: 2026-05-13
---

# lens vs view

Prefer lens for perspectival data surfaces.
`)

	entry, err := ReadEntry(root, "vocabulary/lens-vs-view")
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if entry.Relpath != "vocabulary/lens-vs-view" {
		t.Fatalf("relpath = %q", entry.Relpath)
	}
	if entry.Title != "lens vs view" {
		t.Fatalf("title = %q", entry.Title)
	}
	if !reflect.DeepEqual(entry.Tags, []string{"vocabulary", "naming"}) {
		t.Fatalf("tags = %#v", entry.Tags)
	}
	if !strings.Contains(entry.Body, "Prefer lens") {
		t.Fatalf("body = %q", entry.Body)
	}
}

func TestResolveIncludesDirectoriesFilesDedupAndProjectFilter(t *testing.T) {
	root := t.TempDir()
	writeBokFile(t, root, "README.md", "# readme\n")
	writeBokFile(t, root, "vocabulary/lens-vs-view.md", "---\ntitle: lens vs view\n---\n\nbody\n")
	writeBokFile(t, root, "vocabulary/library-overloads.md", "---\ntitle: library overloads\n---\n\nbody\n")
	writeBokFile(t, root, "naming/lens-vs-view.md", "---\ntitle: naming lens\n---\n\nbody\n")
	writeBokFile(t, root, "layering/internal-boundaries.md", "---\ntitle: internal boundaries\n---\n\nbody\n")
	writeBokFile(t, root, "projects/baab/established-conventions.md", "---\ntitle: baab conventions\n---\n\nbody\n")
	writeBokFile(t, root, "projects/lore/established-conventions.md", "---\ntitle: lore conventions\n---\n\nbody\n")

	got, err := ResolveRelpaths(root, []string{"vocabulary/", "naming/lens-vs-view", "projects/"}, "baab")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{
		"naming/lens-vs-view",
		"projects/baab/established-conventions",
		"vocabulary/lens-vs-view",
		"vocabulary/library-overloads",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relpaths = %#v, want %#v", got, want)
	}
}

func TestResolveRejectsBareTerm(t *testing.T) {
	_, err := ResolveRelpaths(t.TempDir(), []string{"vocabulary"}, "baab")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), BareTermError) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListRelpathsSkipsRootMarkdownAndYaml(t *testing.T) {
	root := t.TempDir()
	writeBokFile(t, root, "README.md", "# readme\n")
	writeBokFile(t, root, "standard.yaml", "passes: []\n")
	writeBokFile(t, root, "vocabulary/lens-vs-view.md", "---\ntitle: lens\n---\n\nbody\n")
	writeBokFile(t, root, "projects/testproj/otis.yaml", "project: {}\n")
	writeBokFile(t, root, "projects/testproj/conventions.md", "---\ntitle: conventions\n---\n\nbody\n")

	got, err := ListRelpaths(root)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []string{"projects/testproj/conventions", "vocabulary/lens-vs-view"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relpaths = %#v, want %#v", got, want)
	}
}

func writeBokFile(t *testing.T, root string, relpath string, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relpath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(body, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}
