package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/dispatcher"
	"github.com/michaelquigley/otis/internal/state"
)

type dispositionRequest struct {
	Disposition string
	Note        string `dd:",+omitempty"`
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	names := make([]string, 0, len(s.cfg.Projects))
	for name := range s.cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)
	projects := make([]map[string]any, 0, len(names))
	for _, name := range names {
		project := s.cfg.Projects[name]
		item := map[string]any{"name": name}
		if project != nil && project.Project != nil {
			item["description"] = project.Project.Description
			item["primary_language"] = project.Project.PrimaryLanguage
		}
		projects = append(projects, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) handlePasses(w http.ResponseWriter, r *http.Request) {
	project, ok := s.project(w, r.PathValue("project"))
	if !ok {
		return
	}
	passes := make([]map[string]any, 0, len(project.Passes))
	for _, pass := range project.Passes {
		item := map[string]any{
			"name":         pass.Name,
			"description":  pass.Description,
			"cadence":      pass.Cadence,
			"top_findings": pass.TopFindings,
		}
		if pass.Reviewer != nil {
			item["reviewer"] = pass.Reviewer.Kind
			item["model"] = pass.Reviewer.Model
		}
		if pass.Scope != nil && pass.Scope.Project != nil {
			item["scope_type"] = pass.Scope.Project.Type
		}
		passes = append(passes, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"passes": passes})
}

func (s *Server) handleFindings(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	if _, ok := s.project(w, projectName); !ok {
		return
	}
	findings, err := s.store.Project(projectName).ListFindings(state.FindingFilter{
		Pass:        r.URL.Query().Get("pass"),
		Disposition: r.URL.Query().Get("disposition"),
		OpenOnly:    r.URL.Query().Get("open") == "true",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(findings))
	for _, finding := range findings {
		data, err := dd.Unbind(finding)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, data)
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": items})
}

func (s *Server) handleFinding(w http.ResponseWriter, r *http.Request) {
	finding, ok := s.finding(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, finding)
}

func (s *Server) handleDisposition(w http.ResponseWriter, r *http.Request) {
	id, ok := findingIDFromRequest(w, r)
	if !ok {
		return
	}
	var req dispositionRequest
	if err := dd.BindJSONReader(&req, r.Body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	actor := authLabel(r.Context())
	if actor == "" {
		actor = "api"
	}
	finding, err := s.store.Project(id.Project).ChangeDisposition(id.String(), req.Disposition, actor, req.Note)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, finding)
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	if _, ok := s.project(w, projectName); !ok {
		return
	}
	runID := fmt.Sprintf("%s/%s/%s/%s", projectName, r.PathValue("pass"), r.PathValue("date"), r.PathValue("time_seq"))
	parsed, err := state.ParseRunID(runID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, err := os.ReadFile(runReportPath(s.store.Root(), parsed))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *Server) handleRunPass(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	passName := r.PathValue("pass")
	if _, ok := s.project(w, projectName); !ok {
		return
	}
	handle, err := s.dispatcher.Enqueue(r.Context(), dispatcher.EnqueueRequest{
		ProjectName: projectName,
		PassName:    passName,
		Source:      dispatcher.SourceForce,
	})
	if err != nil {
		var inFlight *dispatcher.ErrInFlight
		if errors.As(err, &inFlight) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"state":  inFlight.State,
				"run_id": inFlight.RunID,
			})
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if handle == nil {
		writeJSON(w, http.StatusAccepted, map[string]any{"state": "skipped"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"state":   dispatcher.InFlightQueued,
		"project": projectName,
		"pass":    passName,
	})
}

func (s *Server) project(w http.ResponseWriter, name string) (*config.ResolvedProject, bool) {
	project := s.cfg.Projects[name]
	if project == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("project %q not found", name))
		return nil, false
	}
	return project, true
}

func (s *Server) finding(w http.ResponseWriter, r *http.Request) (*state.Finding, bool) {
	id, ok := findingIDFromRequest(w, r)
	if !ok {
		return nil, false
	}
	finding, err := s.store.Project(id.Project).GetFinding(id.String())
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "finding not found")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	return finding, true
}

func findingIDFromRequest(w http.ResponseWriter, r *http.Request) (state.FindingID, bool) {
	id, err := state.ParseID(strings.Join([]string{r.PathValue("project"), r.PathValue("pass"), r.PathValue("seq")}, "/"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return state.FindingID{}, false
	}
	return id, true
}
