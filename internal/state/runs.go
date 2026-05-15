package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RunID is the parsed canonical run id.
type RunID struct {
	Project string
	Pass    string
	Date    string
	TimeSeq string
}

type ReviewerFindingInput struct {
	ID           string
	Severity     string
	Title        string
	Location     Location
	BokRefs      []string
	Description  string
	SuggestedFix string
}

type RecordRunRequest struct {
	RunID       string
	Reviewer    string
	GitHead     string
	Prompt      string
	RawOutput   json.RawMessage
	PriorIDs    map[string]struct{}
	Findings    []ReviewerFindingInput
	CompletedAt time.Time
}

type NoopRunRequest struct {
	RunID     string
	GitHead   string
	Prompt    string
	Report    string
	OutputRaw json.RawMessage
}

type RunFindingSnapshot struct {
	ID          string
	Severity    string
	Title       string
	Disposition string
}

// String renders the canonical run id.
func (id RunID) String() string {
	return fmt.Sprintf("%s/%s/%s/%s", id.Project, id.Pass, id.Date, id.TimeSeq)
}

// ParseRunID parses a canonical run id.
func ParseRunID(value string) (RunID, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 4 {
		return RunID{}, fmt.Errorf("run id must have form <project>/<pass>/<YYYY-MM-DD>/<HHMMSSZ-NNN>")
	}
	if err := ValidateIDComponent(parts[0]); err != nil {
		return RunID{}, fmt.Errorf("project: %w", err)
	}
	if err := ValidateIDComponent(parts[1]); err != nil {
		return RunID{}, fmt.Errorf("pass: %w", err)
	}
	if _, err := time.Parse("2006-01-02", parts[2]); err != nil {
		return RunID{}, fmt.Errorf("date: %w", err)
	}
	if len(parts[3]) != len("150405Z-001") || parts[3][6:] == "" || !strings.Contains(parts[3], "Z-") {
		return RunID{}, fmt.Errorf("time sequence must have form HHMMSSZ-NNN")
	}
	return RunID{Project: parts[0], Pass: parts[1], Date: parts[2], TimeSeq: parts[3]}, nil
}

// AllocateRunID allocates a same-second run sequence and creates the run directory.
func (p *Project) AllocateRunID(pass string, at time.Time) (string, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.allocateRunIDLocked(pass, at)
}

func (p *Project) allocateRunIDLocked(pass string, at time.Time) (string, error) {
	if err := ValidateIDComponent(pass); err != nil {
		return "", fmt.Errorf("pass: %w", err)
	}
	at = at.UTC()
	date := at.Format("2006-01-02")
	prefix := at.Format("150405Z")
	base := filepath.Join(RunsDir(p.root, p.name), date, pass)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	maxSeq := 0
	entries, err := os.ReadDir(base)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix+"-") {
				continue
			}
			var seq int
			if _, err := fmt.Sscanf(entry.Name(), prefix+"-%03d", &seq); err == nil && seq > maxSeq {
				maxSeq = seq
			}
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	timeSeq := fmt.Sprintf("%s-%03d", prefix, maxSeq+1)
	runDir := RunDir(p.root, p.name, pass, date, timeSeq)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return "", err
	}
	return RunID{Project: p.name, Pass: pass, Date: date, TimeSeq: timeSeq}.String(), nil
}

// RecordNoopRun writes a documented no-op run artifact set.
func (p *Project) RecordNoopRun(req NoopRunRequest) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	id, err := p.requireProjectRunID(req.RunID)
	if err != nil {
		return err
	}
	output := req.OutputRaw
	if len(output) == 0 {
		output = json.RawMessage(`{}`)
	}
	report := req.Report
	if strings.TrimSpace(report) == "" {
		report = "# Otis Run Report\n\nNo commits landed on the first-parent line in the window.\n"
	}
	return p.writeRunArtifactsLocked(id, req.Prompt, output, []RunFindingSnapshot{}, report, req.GitHead)
}

// RecordRunFindings normalizes reviewer findings and writes frozen run artifacts.
func (p *Project) RecordRunFindings(req RecordRunRequest) ([]*Finding, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	runID, err := p.requireProjectRunID(req.RunID)
	if err != nil {
		return nil, err
	}
	if err := p.ensureProjectDirsLocked(); err != nil {
		return nil, err
	}
	now := req.CompletedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	persisted := make([]*Finding, 0, len(req.Findings))
	for _, input := range req.Findings {
		if _, ok := req.PriorIDs[input.ID]; input.ID != "" && ok {
			finding, parsed, err := p.readProjectFindingLocked(input.ID)
			if err != nil {
				return nil, err
			}
			finding.LastRunID = req.RunID
			if err := p.writeFindingWithIDLocked(parsed, finding); err != nil {
				return nil, err
			}
			if err := p.appendDispositionEventLocked(DispositionEvent{
				Type:      EventFindingReobserved,
				FindingID: finding.ID,
				RunID:     req.RunID,
				At:        now,
			}); err != nil {
				return nil, err
			}
			persisted = append(persisted, cloneFinding(finding))
			continue
		}
		seq, err := p.nextFindingSequenceLocked(runID.Pass)
		if err != nil {
			return nil, err
		}
		id := FindingID{Project: p.name, Pass: runID.Pass, Sequence: seq}
		finding := &Finding{
			ID:           id.String(),
			Project:      p.name,
			Pass:         runID.Pass,
			Reviewer:     req.Reviewer,
			FirstRunID:   req.RunID,
			LastRunID:    req.RunID,
			CreatedAt:    now,
			Severity:     input.Severity,
			Title:        input.Title,
			Location:     input.Location,
			BokRefs:      append([]string(nil), input.BokRefs...),
			Description:  input.Description,
			SuggestedFix: input.SuggestedFix,
			Disposition:  DispositionOpen,
		}
		if err := validateFinding(finding); err != nil {
			return nil, err
		}
		if err := p.writeFindingLocked(finding); err != nil {
			return nil, err
		}
		if err := p.appendDispositionEventLocked(DispositionEvent{
			Type:      EventFindingCreated,
			FindingID: finding.ID,
			RunID:     req.RunID,
			At:        now,
		}); err != nil {
			return nil, err
		}
		persisted = append(persisted, cloneFinding(finding))
	}
	snapshots := snapshotsFromFindings(persisted)
	report := renderRunReport(req.RunID, persisted)
	if err := p.writeRunArtifactsLocked(runID, req.Prompt, req.RawOutput, snapshots, report, req.GitHead); err != nil {
		return nil, err
	}
	if err := p.renderBacklogLocked(); err != nil {
		return nil, err
	}
	return persisted, nil
}

func (p *Project) requireProjectRunID(value string) (RunID, error) {
	id, err := ParseRunID(value)
	if err != nil {
		return RunID{}, err
	}
	if id.Project != p.name {
		return RunID{}, fmt.Errorf("run %q belongs to project %q, not %q", value, id.Project, p.name)
	}
	return id, nil
}

func (p *Project) writeRunArtifactsLocked(id RunID, prompt string, output json.RawMessage, snapshots []RunFindingSnapshot, report string, gitHead string) error {
	runDir := RunDir(p.root, id.Project, id.Pass, id.Date, id.TimeSeq)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return err
	}
	if err := atomicWriteFile(filepath.Join(runDir, "prompt.md"), []byte(prompt), 0o600); err != nil {
		return err
	}
	if len(output) == 0 {
		output = json.RawMessage(`{}`)
	}
	if !json.Valid(output) {
		output = json.RawMessage(fmt.Sprintf("%q", string(output)))
	}
	if err := atomicWriteFile(filepath.Join(runDir, "output.json"), appendPrettyJSON(output), 0o600); err != nil {
		return err
	}
	rawFindings, err := marshalDDJSONSlice(snapshots)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(filepath.Join(runDir, "findings.json"), rawFindings, 0o600); err != nil {
		return err
	}
	if err := atomicWriteFile(filepath.Join(runDir, "report.md"), []byte(report), 0o600); err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(runDir, "git-head.txt"), []byte(strings.TrimSpace(gitHead)+"\n"), 0o600)
}

func snapshotsFromFindings(findings []*Finding) []RunFindingSnapshot {
	snapshots := make([]RunFindingSnapshot, 0, len(findings))
	for _, finding := range findings {
		snapshots = append(snapshots, RunFindingSnapshot{
			ID:          finding.ID,
			Severity:    finding.Severity,
			Title:       finding.Title,
			Disposition: finding.Disposition,
		})
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].ID < snapshots[j].ID })
	return snapshots
}

func renderRunReport(runID string, findings []*Finding) string {
	var b strings.Builder
	b.WriteString("# Otis Run Report\n\n")
	b.WriteString(fmt.Sprintf("Run: `%s`\n\n", runID))
	if len(findings) == 0 {
		b.WriteString("No findings.\n")
		return b.String()
	}
	for _, finding := range findings {
		b.WriteString(fmt.Sprintf("## %s\n\n", finding.ID))
		b.WriteString(fmt.Sprintf("- Severity: `%s`\n", finding.Severity))
		b.WriteString(fmt.Sprintf("- Disposition: `%s`\n", finding.Disposition))
		b.WriteString(fmt.Sprintf("- Location: `%s:%s`\n\n", finding.Location.File, finding.Location.Lines))
		b.WriteString(fmt.Sprintf("### %s\n\n", finding.Title))
		b.WriteString(strings.TrimSpace(finding.Description))
		b.WriteString("\n\n")
		if strings.TrimSpace(finding.SuggestedFix) != "" {
			b.WriteString("Suggested fix: ")
			b.WriteString(strings.TrimSpace(finding.SuggestedFix))
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func appendPrettyJSON(raw json.RawMessage) []byte {
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return append(append([]byte(nil), raw...), '\n')
	}
	out.WriteByte('\n')
	return out.Bytes()
}
