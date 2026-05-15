package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/michaelquigley/otis/internal/client"
	"github.com/michaelquigley/otis/internal/state"
)

type projectsResponse struct {
	Projects []projectItem `dd:"projects"`
}

type projectItem struct {
	Name            string `dd:"name"`
	Description     string `dd:"description,+omitempty"`
	PrimaryLanguage string `dd:"primary_language,+omitempty"`
}

type passesResponse struct {
	Passes []passItem `dd:"passes"`
}

type passItem struct {
	Name        string `dd:"name"`
	Description string `dd:"description,+omitempty"`
	Cadence     string `dd:"cadence,+omitempty"`
	TopFindings int    `dd:"top_findings,+omitempty"`
	Reviewer    string `dd:"reviewer,+omitempty"`
	Model       string `dd:"model,+omitempty"`
	ScopeType   string `dd:"scope_type,+omitempty"`
}

type findingsResponse struct {
	Findings []*state.Finding `dd:"findings"`
}

type dispositionBody struct {
	Disposition string `dd:",+required"`
	Note        string `dd:",+omitempty"`
}

type runResponse struct {
	State   string `dd:"state"`
	Project string `dd:"project,+omitempty"`
	Pass    string `dd:"pass,+omitempty"`
	RunID   string `dd:"run_id,+omitempty"`
}

func newSupervisorClient(configPath string) (*client.Client, error) {
	cfg, err := client.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return client.New(cfg)
}

func apiPath(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return "/api/v1/" + strings.Join(escaped, "/")
}

func findingAPIPath(id state.FindingID) string {
	return fmt.Sprintf("%s/findings/%s/%04d", apiPath("projects", id.Project), url.PathEscape(id.Pass), id.Sequence)
}

func reportAPIPath(id state.RunID) string {
	return fmt.Sprintf("%s/runs/%s/%s/%s/report",
		apiPath("projects", id.Project),
		url.PathEscape(id.Pass),
		url.PathEscape(id.Date),
		url.PathEscape(id.TimeSeq))
}

func splitProjectPass(value string) (string, string, error) {
	projectName, passName, ok := strings.Cut(value, "/")
	if !ok || projectName == "" || passName == "" || strings.Contains(passName, "/") {
		return "", "", fmt.Errorf("pass ref must have form <project>/<pass>")
	}
	if err := state.ValidateIDComponent(projectName); err != nil {
		return "", "", fmt.Errorf("project: %w", err)
	}
	if err := state.ValidateIDComponent(passName); err != nil {
		return "", "", fmt.Errorf("pass: %w", err)
	}
	return projectName, passName, nil
}
