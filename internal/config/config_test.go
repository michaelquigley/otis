package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadComposesExampleConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "docs", "example", "global.yaml"))
	if err != nil {
		t.Fatalf("load example config: %v", err)
	}
	project := cfg.Projects["testproj"]
	if project == nil {
		t.Fatal("expected testproj")
	}
	if len(project.Passes) != 1 {
		t.Fatalf("passes = %d, want 1", len(project.Passes))
	}
	pass := project.Passes[0]
	if pass.Name != "vocabulary-sweep" {
		t.Fatalf("pass name = %q", pass.Name)
	}
	if pass.TopFindings != 2 {
		t.Fatalf("top findings = %d, want 2", pass.TopFindings)
	}
	if cfg.Global.Prompt.PerFileBytes != DefaultPerFileBytes {
		t.Fatalf("per file bytes = %d", cfg.Global.Prompt.PerFileBytes)
	}
}

func TestProjectOverrideMergesInheritedPass(t *testing.T) {
	dir := t.TempDir()
	bokPath := filepath.Join(dir, "bok")
	if err := os.MkdirAll(filepath.Join(bokPath, "projects", "testproj"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(bokPath, "standard.yaml"), `
passes:
  - name: vocabulary-sweep
    description: inherited
    scope:
      project: { type: full }
      bok:
        include: [vocabulary/]
    reviewer: { kind: codex }
    cadence: 24h
    top_findings: 3
`)
	projectPath := filepath.Join(bokPath, "projects", "testproj", "otis.yaml")
	writeFile(t, projectPath, `
include_configs: [standard]
project:
  name: testproj
  top_findings: 3
passes:
  - name: vocabulary-sweep
    top_findings: 5
`)
	project, err := LoadProject(projectPath, ProfileLoaderForBok(bokPath))
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	pass := project.Passes[0]
	if pass.Description != "inherited" {
		t.Fatalf("description = %q", pass.Description)
	}
	if pass.TopFindings != 5 {
		t.Fatalf("top findings = %d", pass.TopFindings)
	}
}

func TestBareTermIncludeRejected(t *testing.T) {
	dir := t.TempDir()
	bokPath := filepath.Join(dir, "bok")
	if err := os.MkdirAll(filepath.Join(bokPath, "projects", "testproj"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(bokPath, "standard.yaml"), `
passes:
  - name: vocabulary-sweep
    scope:
      project: { type: full }
      bok:
        include: [vocabulary]
    reviewer: { kind: codex }
    cadence: 24h
`)
	_, err := LoadProfile(filepath.Join(bokPath, "standard.yaml"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bare-term entries are reserved") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimLeft(body, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}
