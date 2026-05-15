package state

import (
	"bufio"
	"errors"
	"os"
	"time"
)

const (
	EventFindingCreated     = "finding_created"
	EventFindingReobserved  = "finding_reobserved"
	EventDispositionChanged = "disposition_changed"
	DefaultDispositionActor = "human"
)

// DispositionEvent is one append-only finding lifecycle event.
type DispositionEvent struct {
	Type        string
	FindingID   string
	RunID       string `dd:",+omitempty"`
	Disposition string `dd:",+omitempty"`
	Actor       string `dd:",+omitempty"`
	Note        string `dd:",+omitempty"`
	At          time.Time
}

type NoteHistoryEntry struct {
	Disposition string
	Note        string
	Actor       string
	At          time.Time
}

type ReducedFindingState struct {
	FindingID   string
	LastRunID   string
	Disposition string
	Notes       []NoteHistoryEntry
}

type FindingContext struct {
	Finding *Finding
	Notes   []NoteHistoryEntry
}

// FindingContexts returns findings for a pass with reduced note history.
func (p *Project) FindingContexts(pass string) ([]FindingContext, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	findings, err := p.listFindingsLocked(FindingFilter{Pass: pass})
	if err != nil {
		return nil, err
	}
	events, err := p.readDispositionEventsLocked()
	if err != nil {
		return nil, err
	}
	reduced := reduceDispositionEvents(events)
	contexts := make([]FindingContext, 0, len(findings))
	for _, finding := range findings {
		state := reduced[finding.ID]
		contexts = append(contexts, FindingContext{
			Finding: cloneFinding(finding),
			Notes:   append([]NoteHistoryEntry(nil), state.Notes...),
		})
	}
	return contexts, nil
}

// ReduceDispositionEvents returns the current event-reduced state.
func (p *Project) ReduceDispositionEvents() (map[string]ReducedFindingState, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	events, err := p.readDispositionEventsLocked()
	if err != nil {
		return nil, err
	}
	return reduceDispositionEvents(events), nil
}

func (p *Project) appendDispositionEventLocked(event DispositionEvent) error {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	if event.Type == "" {
		return errors.New("disposition event type is required")
	}
	if _, err := ParseID(event.FindingID); err != nil {
		return err
	}
	if event.Type == EventDispositionChanged {
		if err := ValidateDisposition(event.Disposition); err != nil {
			return err
		}
		if event.Actor == "" {
			event.Actor = DefaultDispositionActor
		}
	}
	raw, err := marshalDDJSONLine(event)
	if err != nil {
		return err
	}
	path := DispositionsPath(p.root, p.name)
	if err := os.MkdirAll(ProjectDir(p.root, p.name), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(raw)
	return err
}

func (p *Project) readDispositionEventsLocked() ([]DispositionEvent, error) {
	file, err := os.Open(DispositionsPath(p.root, p.name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var events []DispositionEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event DispositionEvent
		if err := bindDDJSON(&event, line); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func reduceDispositionEvents(events []DispositionEvent) map[string]ReducedFindingState {
	out := map[string]ReducedFindingState{}
	for _, event := range events {
		state := out[event.FindingID]
		state.FindingID = event.FindingID
		switch event.Type {
		case EventFindingCreated, EventFindingReobserved:
			if event.RunID != "" {
				state.LastRunID = event.RunID
			}
			if state.Disposition == "" {
				state.Disposition = DispositionOpen
			}
		case EventDispositionChanged:
			state.Disposition = event.Disposition
			if event.Note != "" {
				state.Notes = append(state.Notes, NoteHistoryEntry{
					Disposition: event.Disposition,
					Note:        event.Note,
					Actor:       event.Actor,
					At:          event.At,
				})
			}
		}
		out[event.FindingID] = state
	}
	return out
}
