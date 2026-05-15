package state

import (
	"errors"
	"os"
	"time"
)

type LastRunRecord struct {
	RunID string
	At    time.Time
}

// LastRuns reads the per-pass last dispatch timestamps for this project.
func (p *Project) LastRuns() (map[string]LastRunRecord, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.readLastRunsLocked()
}

// LastRun reads the last dispatch timestamp for one pass.
func (p *Project) LastRun(pass string) (LastRunRecord, bool, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	records, err := p.readLastRunsLocked()
	if err != nil {
		return LastRunRecord{}, false, err
	}
	record, ok := records[pass]
	return record, ok, nil
}

// RecordLastRun writes the last dispatch timestamp for one pass.
func (p *Project) RecordLastRun(pass string, runID string, at time.Time) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.recordLastRunLocked(pass, runID, at)
}

// AllocateRunStart allocates a run id and records last-run in one project lock.
func (p *Project) AllocateRunStart(pass string, at time.Time) (string, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	runID, err := p.allocateRunIDLocked(pass, at)
	if err != nil {
		return "", err
	}
	if err := p.recordLastRunLocked(pass, runID, at); err != nil {
		return "", err
	}
	return runID, nil
}

func (p *Project) readLastRunsLocked() (map[string]LastRunRecord, error) {
	raw, err := os.ReadFile(LastRunPath(p.root, p.name))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]LastRunRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	records, err := bindDDJSONMap[LastRunRecord](raw)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (p *Project) recordLastRunLocked(pass string, runID string, at time.Time) error {
	if err := ValidateIDComponent(pass); err != nil {
		return err
	}
	if _, err := ParseRunID(runID); err != nil {
		return err
	}
	records, err := p.readLastRunsLocked()
	if err != nil {
		return err
	}
	if records == nil {
		records = map[string]LastRunRecord{}
	}
	if at.IsZero() {
		at = time.Now()
	}
	records[pass] = LastRunRecord{
		RunID: runID,
		At:    at.UTC(),
	}
	raw, err := marshalDDJSONMap(records)
	if err != nil {
		return err
	}
	return atomicWriteFile(LastRunPath(p.root, p.name), raw, 0o600)
}
