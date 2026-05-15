package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"
	"github.com/michaelquigley/otis/internal/bok"
	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/notify"
	"github.com/michaelquigley/otis/internal/notify/mattermost"
	"github.com/michaelquigley/otis/internal/prompt"
	"github.com/michaelquigley/otis/internal/reviewer"
	"github.com/michaelquigley/otis/internal/reviewer/claudecode"
	"github.com/michaelquigley/otis/internal/reviewer/codex"
	"github.com/michaelquigley/otis/internal/reviewer/dummy"
	"github.com/michaelquigley/otis/internal/reviewer/pi"
	"github.com/michaelquigley/otis/internal/state"
)

type RunRequest struct {
	Config           *config.ResolvedConfig
	Store            *state.Store
	ProjectName      string
	PassName         string
	ReviewerOverride string
	DummyOutputPath  string
	RunID            string
	GitHead          string
	Now              time.Time
}

type RunResult struct {
	RunID    string
	RunDir   string
	Findings []*state.Finding
	Noop     bool
}

func Run(ctx context.Context, req RunRequest) (result RunResult, runErr error) {
	if req.Config == nil || req.Config.Global == nil {
		return RunResult{}, fmt.Errorf("config is required")
	}
	project, ok := req.Config.Projects[req.ProjectName]
	if !ok {
		return RunResult{}, fmt.Errorf("project %q is not configured", req.ProjectName)
	}
	pass := findPass(project, req.PassName)
	if pass == nil {
		return RunResult{}, fmt.Errorf("project %q pass %q is not configured", req.ProjectName, req.PassName)
	}
	if req.Now.IsZero() {
		req.Now = time.Now()
	}
	store := req.Store
	if store == nil {
		var err error
		store, err = state.NewStore(req.Config.Global.Storage.StateDir)
		if err != nil {
			return RunResult{}, err
		}
	}
	stateProject := store.Project(req.ProjectName)

	capturedSHA := req.GitHead
	if capturedSHA == "" {
		var err error
		capturedSHA, err = CaptureHEAD(ctx, project.RepoPath)
		if err != nil {
			return RunResult{}, err
		}
	}
	runID := req.RunID
	if runID == "" {
		var err error
		runID, err = stateProject.AllocateRunID(pass.Name, req.Now)
		if err != nil {
			return RunResult{}, err
		}
	}
	parsedRunID, err := state.ParseRunID(runID)
	if err != nil {
		return RunResult{}, err
	}
	result.RunID = runID
	result.RunDir = state.RunDir(store.Root(), parsedRunID.Project, parsedRunID.Pass, parsedRunID.Date, parsedRunID.TimeSeq)
	scratchPath := state.RunScratchDir(store.Root(), runID)
	if err := CreateWorktree(ctx, project.RepoPath, capturedSHA, scratchPath); err != nil {
		return result, err
	}
	defer func() {
		if err := RemoveWorktree(context.Background(), project.RepoPath, scratchPath); err != nil && runErr == nil {
			runErr = err
		}
	}()

	resolvedScope, err := ResolveScope(ctx, scratchPath, pass.Scope.Project, req.Now)
	if err != nil {
		return result, err
	}
	if resolvedScope.Kind == prompt.ScopeRecent && len(resolvedScope.Files) == 0 {
		err := stateProject.RecordNoopRun(state.NoopRunRequest{
			RunID:     runID,
			GitHead:   capturedSHA,
			Prompt:    "# Otis reviewer: no commits in window\n\nNo commits landed on the first-parent line in the configured recent-scope window.\n",
			Report:    "# Otis Run Report\n\nNo commits landed on the first-parent line in the window.\n",
			OutputRaw: json.RawMessage(`{}`),
		})
		result.Noop = true
		return result, err
	}

	bokEntries, err := bok.Resolve(req.Config.Global.Bok.Path, pass.Scope.Bok.Include, req.ProjectName)
	if err != nil {
		return result, err
	}
	contexts, err := stateProject.FindingContexts(pass.Name)
	if err != nil {
		return result, err
	}
	scopeContent, err := prompt.BuildScopeContent(ctx, resolvedScope.Kind, resolvedScope.Files, resolvedScope.BaseSHA, scratchPath, prompt.ScopeOptions{
		PerFileBytes:    req.Config.Global.Prompt.PerFileBytes,
		TotalScopeBytes: req.Config.Global.Prompt.TotalScopeBytes,
	})
	if err != nil {
		return result, err
	}
	built := prompt.Build(prompt.Request{
		Project: prompt.ProjectContext{
			Name:            project.Project.Name,
			Description:     project.Project.Description,
			PrimaryLanguage: project.Project.PrimaryLanguage,
		},
		Pass: prompt.PassContext{
			Name:        pass.Name,
			Description: pass.Description,
		},
		TopFindings:   pass.TopFindings,
		BokEntries:    bokEntries,
		Scope:         scopeContent,
		PriorFindings: prompt.PriorFindingsFromState(contexts),
	})
	reviewerKind := req.ReviewerOverride
	if reviewerKind == "" && pass.Reviewer != nil {
		reviewerKind = pass.Reviewer.Kind
	}
	r, reviewerName, err := buildReviewer(req.Config.Global, pass, reviewerKind, req.DummyOutputPath)
	if err != nil {
		return result, err
	}
	reviewResult, err := r.Review(ctx, reviewer.Request{
		Prompt:     built.Prompt,
		Schema:     built.Schema,
		WorkingDir: scratchPath,
		Model:      reviewerModel(req.Config.Global, pass, reviewerKind),
	})
	if err != nil {
		return result, err
	}
	priorIDs := map[string]struct{}{}
	for _, ctx := range contexts {
		if ctx.Finding != nil {
			priorIDs[ctx.Finding.ID] = struct{}{}
		}
	}
	inputs := make([]state.ReviewerFindingInput, 0, len(reviewResult.Findings))
	for _, finding := range reviewResult.Findings {
		inputs = append(inputs, state.ReviewerFindingInput{
			ID:           finding.ID,
			Severity:     finding.Severity,
			Title:        finding.Title,
			Location:     state.Location{File: finding.Location.File, Lines: finding.Location.Lines},
			BokRefs:      append([]string(nil), finding.BokRefs...),
			Description:  finding.Description,
			SuggestedFix: finding.SuggestedFix,
		})
	}
	persisted, err := stateProject.RecordRunFindings(state.RecordRunRequest{
		RunID:       runID,
		Reviewer:    reviewerName,
		GitHead:     capturedSHA,
		Prompt:      built.Prompt,
		RawOutput:   reviewResult.Raw,
		PriorIDs:    priorIDs,
		Findings:    inputs,
		CompletedAt: time.Now().UTC(),
	})
	if err != nil {
		return result, err
	}
	result.Findings = persisted
	if err := postRunNotification(ctx, req.Config.Global, project, runID, persisted); err != nil {
		return result, err
	}
	return result, nil
}

func findPass(project *config.ResolvedProject, name string) *config.Pass {
	if project == nil {
		return nil
	}
	for _, pass := range project.Passes {
		if pass.Name == name {
			return pass
		}
	}
	return nil
}

func buildReviewer(global *config.GlobalConfig, pass *config.Pass, kind string, dummyOutputPath string) (reviewer.Reviewer, string, error) {
	switch kind {
	case "dummy":
		return dummy.New(dummy.Options{OutputPath: dummyOutputPath}), "dummy", nil
	case "codex":
		cfg := global.Reviewers["codex"]
		if cfg == nil {
			return nil, "", fmt.Errorf("reviewer %q is not configured", kind)
		}
		return codex.New(codex.Options{
			BinaryPath: cfg.Binary,
			Model:      reviewerModel(global, pass, kind),
			DryRun:     cfg.DryRun,
		}), "codex", nil
	case "claude-code":
		cfg := global.Reviewers["claude-code"]
		if cfg == nil {
			return nil, "", fmt.Errorf("reviewer %q is not configured", kind)
		}
		return claudecode.New(claudecode.Options{
			BinaryPath: cfg.Binary,
			Model:      reviewerModel(global, pass, kind),
			DryRun:     cfg.DryRun,
		}), "claude-code", nil
	case "pi":
		cfg := global.Reviewers["pi"]
		if cfg == nil {
			return nil, "", fmt.Errorf("reviewer %q is not configured", kind)
		}
		return pi.New(pi.Options{
			BinaryPath: cfg.Binary,
			Model:      reviewerModel(global, pass, kind),
			DryRun:     cfg.DryRun,
		}), "pi", nil
	case "":
		return nil, "", fmt.Errorf("reviewer is required")
	default:
		return nil, "", fmt.Errorf("reviewer %q is not configured", kind)
	}
}

func postRunNotification(ctx context.Context, global *config.GlobalConfig, project *config.ResolvedProject, runIDValue string, findings []*state.Finding) error {
	if len(findings) == 0 || global == nil || global.Notification == nil || global.Notification.Mattermost == nil {
		return nil
	}
	runID, err := state.ParseRunID(runIDValue)
	if err != nil {
		return err
	}
	mm := global.Notification.Mattermost
	if strings.TrimSpace(mm.URL) == "" {
		return nil
	}
	notifier := mattermost.New(mattermost.Options{
		URL:      mm.URL,
		TokenEnv: mm.TokenEnv,
	})
	return notifier.Post(ctx, notify.Notification{
		Project:   runID.Project,
		Pass:      runID.Pass,
		RunID:     runID.String(),
		Date:      runID.Date,
		Channel:   notificationChannel(runID.Project, project),
		ReportURL: reportURL(global, runID),
		Findings:  findings,
	})
}

func notificationChannel(projectName string, project *config.ResolvedProject) string {
	if project != nil && project.Project != nil && project.Project.Notify != nil {
		if channel := strings.TrimSpace(project.Project.Notify.Mattermost); channel != "" {
			return channel
		}
	}
	return "#otis-" + projectName
}

func reportURL(global *config.GlobalConfig, runID state.RunID) string {
	base := ""
	if global != nil && global.Notification != nil {
		base = strings.TrimRight(strings.TrimSpace(global.Notification.ReportBaseURL), "/")
	}
	if base == "" {
		listen := "127.0.0.1:8443"
		if global != nil && global.API != nil && strings.TrimSpace(global.API.Listen) != "" {
			listen = strings.TrimSpace(global.API.Listen)
		}
		base = "https://" + listen
		dl.Warnf("notification.report_base_url is empty; using %s; links may not resolve externally", base)
	}
	return fmt.Sprintf("%s/api/v1/projects/%s/runs/%s/%s/%s/report",
		base,
		url.PathEscape(runID.Project),
		url.PathEscape(runID.Pass),
		url.PathEscape(runID.Date),
		url.PathEscape(runID.TimeSeq))
}

func reviewerModel(global *config.GlobalConfig, pass *config.Pass, kind string) string {
	if pass != nil && pass.Reviewer != nil && pass.Reviewer.Model != "" {
		return pass.Reviewer.Model
	}
	if global != nil && global.Reviewers != nil && global.Reviewers[kind] != nil {
		return global.Reviewers[kind].DefaultModel
	}
	return ""
}

func WriteDummyOutput(path string, findings []prompt.ReviewerFinding) error {
	raw, err := dd.UnbindJSON(prompt.ReviewerOutput{Findings: findings})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func SplitProjectPass(value string) (string, string, error) {
	projectName, passName, ok := strings.Cut(value, "/")
	if !ok || projectName == "" || passName == "" || strings.Contains(passName, "/") {
		return "", "", fmt.Errorf("pass ref must have form <project>/<pass>")
	}
	return projectName, passName, nil
}
