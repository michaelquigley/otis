package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DispositionOpen     = "open"
	DispositionAccepted = "accepted"
	DispositionDeferred = "deferred"
	DispositionRejected = "rejected"
)

type Location struct {
	File  string
	Lines string
}

// Finding is the persisted finding schema.
type Finding struct {
	ID           string
	Project      string
	Pass         string
	Reviewer     string
	FirstRunID   string
	LastRunID    string
	CreatedAt    time.Time
	Severity     string
	Title        string
	Location     Location
	BokRefs      []string
	Description  string
	SuggestedFix string
	Disposition  string
}

// FindingID is the parsed canonical finding id.
type FindingID struct {
	Project  string
	Pass     string
	Sequence int
}

// CreateFindingRequest contains dispatcher-owned data for a fresh finding.
type CreateFindingRequest struct {
	Pass         string
	Reviewer     string
	RunID        string
	Severity     string
	Title        string
	Location     Location
	BokRefs      []string
	Description  string
	SuggestedFix string
}

type FindingFilter struct {
	Pass        string
	Disposition string
	OpenOnly    bool
}

// String renders the canonical finding id.
func (id FindingID) String() string {
	return fmt.Sprintf("%s/%s/%04d", id.Project, id.Pass, id.Sequence)
}

// Filename maps the canonical id to its persisted filename.
func (id FindingID) Filename() string {
	return fmt.Sprintf("%s--%s--%04d.json", id.Project, id.Pass, id.Sequence)
}

// ParseID parses a canonical finding id.
func ParseID(value string) (FindingID, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return FindingID{}, fmt.Errorf("finding id must have form <project>/<pass>/<NNNN>")
	}
	if err := ValidateIDComponent(parts[0]); err != nil {
		return FindingID{}, fmt.Errorf("project: %w", err)
	}
	if err := ValidateIDComponent(parts[1]); err != nil {
		return FindingID{}, fmt.Errorf("pass: %w", err)
	}
	seq, err := parseSequence(parts[2])
	if err != nil {
		return FindingID{}, err
	}
	return FindingID{Project: parts[0], Pass: parts[1], Sequence: seq}, nil
}

// ParseFilename parses a persisted finding filename.
func ParseFilename(value string) (FindingID, error) {
	parts := strings.Split(value, "--")
	if len(parts) != 3 {
		return FindingID{}, fmt.Errorf("finding filename must have form <project>--<pass>--<NNNN>.json")
	}
	if filepath.Ext(parts[2]) != ".json" {
		return FindingID{}, fmt.Errorf("finding filename must end in .json")
	}
	return ParseID(fmt.Sprintf("%s/%s/%s", parts[0], parts[1], strings.TrimSuffix(parts[2], ".json")))
}

// CreateFinding allocates and persists a new open finding.
func (p *Project) CreateFinding(req CreateFindingRequest) (*Finding, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if err := p.ensureProjectDirsLocked(); err != nil {
		return nil, err
	}
	seq, err := p.nextFindingSequenceLocked(req.Pass)
	if err != nil {
		return nil, err
	}
	id := FindingID{Project: p.name, Pass: req.Pass, Sequence: seq}
	now := time.Now().UTC()
	finding := &Finding{
		ID:           id.String(),
		Project:      p.name,
		Pass:         req.Pass,
		Reviewer:     req.Reviewer,
		FirstRunID:   req.RunID,
		LastRunID:    req.RunID,
		CreatedAt:    now,
		Severity:     req.Severity,
		Title:        req.Title,
		Location:     req.Location,
		BokRefs:      append([]string(nil), req.BokRefs...),
		Description:  req.Description,
		SuggestedFix: req.SuggestedFix,
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
	if err := p.renderBacklogLocked(); err != nil {
		return nil, err
	}
	return cloneFinding(finding), nil
}

// ReobserveFinding records a reobserved finding without changing disposition.
func (p *Project) ReobserveFinding(idValue string, runID string) (*Finding, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	finding, parsed, err := p.readProjectFindingLocked(idValue)
	if err != nil {
		return nil, err
	}
	finding.LastRunID = runID
	if err := p.writeFindingWithIDLocked(parsed, finding); err != nil {
		return nil, err
	}
	if err := p.appendDispositionEventLocked(DispositionEvent{
		Type:      EventFindingReobserved,
		FindingID: finding.ID,
		RunID:     runID,
		At:        time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	if err := p.renderBacklogLocked(); err != nil {
		return nil, err
	}
	return cloneFinding(finding), nil
}

// ChangeDisposition appends a disposition event and updates the finding cache.
func (p *Project) ChangeDisposition(idValue string, disposition string, actor string, note string) (*Finding, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if err := ValidateDisposition(disposition); err != nil {
		return nil, err
	}
	finding, parsed, err := p.readProjectFindingLocked(idValue)
	if err != nil {
		return nil, err
	}
	finding.Disposition = disposition
	if err := p.writeFindingWithIDLocked(parsed, finding); err != nil {
		return nil, err
	}
	if err := p.appendDispositionEventLocked(DispositionEvent{
		Type:        EventDispositionChanged,
		FindingID:   finding.ID,
		Disposition: disposition,
		Actor:       actor,
		Note:        note,
		At:          time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	if err := p.renderBacklogLocked(); err != nil {
		return nil, err
	}
	return cloneFinding(finding), nil
}

// GetFinding reads one finding by canonical id.
func (p *Project) GetFinding(idValue string) (*Finding, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	finding, _, err := p.readProjectFindingLocked(idValue)
	if err != nil {
		return nil, err
	}
	return cloneFinding(finding), nil
}

// ListFindings reads findings for this project.
func (p *Project) ListFindings(filter FindingFilter) ([]*Finding, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	findings, err := p.listFindingsLocked(filter)
	if err != nil {
		return nil, err
	}
	return findings, nil
}

func (p *Project) listFindingsLocked(filter FindingFilter) ([]*Finding, error) {
	entries, err := os.ReadDir(FindingsDir(p.root, p.name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	findings := make([]*Finding, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		id, err := ParseFilename(entry.Name())
		if err != nil {
			continue
		}
		if id.Project != p.name {
			continue
		}
		finding, err := p.readFindingWithIDLocked(id)
		if err != nil {
			return nil, err
		}
		if filter.Pass != "" && finding.Pass != filter.Pass {
			continue
		}
		if filter.OpenOnly && finding.Disposition != DispositionOpen {
			continue
		}
		if filter.Disposition != "" && finding.Disposition != filter.Disposition {
			continue
		}
		findings = append(findings, finding)
	}
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].ID < findings[j].ID
	})
	return findings, nil
}

func (p *Project) readProjectFindingLocked(idValue string) (*Finding, FindingID, error) {
	id, err := ParseID(idValue)
	if err != nil {
		return nil, FindingID{}, err
	}
	if id.Project != p.name {
		return nil, FindingID{}, fmt.Errorf("finding %q belongs to project %q, not %q", idValue, id.Project, p.name)
	}
	finding, err := p.readFindingWithIDLocked(id)
	return finding, id, err
}

func (p *Project) readFindingWithIDLocked(id FindingID) (*Finding, error) {
	raw, err := os.ReadFile(filepath.Join(FindingsDir(p.root, p.name), id.Filename()))
	if err != nil {
		return nil, err
	}
	var finding Finding
	if err := bindDDJSON(&finding, raw); err != nil {
		return nil, err
	}
	return &finding, nil
}

func (p *Project) writeFindingLocked(finding *Finding) error {
	id, err := ParseID(finding.ID)
	if err != nil {
		return err
	}
	return p.writeFindingWithIDLocked(id, finding)
}

func (p *Project) writeFindingWithIDLocked(id FindingID, finding *Finding) error {
	if id.Project != p.name {
		return fmt.Errorf("finding %q belongs to project %q, not %q", finding.ID, id.Project, p.name)
	}
	raw, err := marshalDDJSON(finding)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(FindingsDir(p.root, p.name), id.Filename()), raw, 0o600)
}

func (p *Project) nextFindingSequenceLocked(pass string) (int, error) {
	if err := ValidateIDComponent(pass); err != nil {
		return 0, fmt.Errorf("pass: %w", err)
	}
	entries, err := os.ReadDir(FindingsDir(p.root, p.name))
	if errors.Is(err, os.ErrNotExist) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	maxSeq := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		id, err := ParseFilename(entry.Name())
		if err != nil {
			continue
		}
		if id.Project == p.name && id.Pass == pass && id.Sequence > maxSeq {
			maxSeq = id.Sequence
		}
	}
	return maxSeq + 1, nil
}

func (p *Project) ensureProjectDirsLocked() error {
	for _, dir := range []string{
		ProjectDir(p.root, p.name),
		FindingsDir(p.root, p.name),
		RunsDir(p.root, p.name),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func validateFinding(finding *Finding) error {
	id, err := ParseID(finding.ID)
	if err != nil {
		return err
	}
	if finding.Project != id.Project {
		return fmt.Errorf("finding.project must match id project")
	}
	if finding.Pass != id.Pass {
		return fmt.Errorf("finding.pass must match id pass")
	}
	if finding.Reviewer == "" {
		return fmt.Errorf("finding.reviewer is required")
	}
	if finding.FirstRunID == "" {
		return fmt.Errorf("finding.first_run_id is required")
	}
	if finding.LastRunID == "" {
		return fmt.Errorf("finding.last_run_id is required")
	}
	if finding.Severity == "" {
		return fmt.Errorf("finding.severity is required")
	}
	if finding.Title == "" {
		return fmt.Errorf("finding.title is required")
	}
	return ValidateDisposition(finding.Disposition)
}

func ValidateDisposition(value string) error {
	switch value {
	case DispositionOpen, DispositionAccepted, DispositionDeferred, DispositionRejected:
		return nil
	default:
		return fmt.Errorf("disposition must be one of open, accepted, deferred, rejected")
	}
}

func parseSequence(value string) (int, error) {
	if len(value) < 4 {
		return 0, fmt.Errorf("sequence must be at least four digits")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("sequence must contain only digits")
		}
	}
	seq, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return seq, nil
}

func cloneFinding(finding *Finding) *Finding {
	if finding == nil {
		return nil
	}
	out := *finding
	out.BokRefs = append([]string(nil), finding.BokRefs...)
	return &out
}

func atomicWriteFile(path string, raw []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
