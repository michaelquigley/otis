package state

import (
	"bufio"
	"encoding/json"
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
	Type        string    `json:"type"`
	FindingID   string    `json:"finding_id"`
	RunID       string    `json:"run_id,omitempty"`
	Disposition string    `json:"disposition,omitempty"`
	Actor       string    `json:"actor,omitempty"`
	Note        string    `json:"note,omitempty"`
	At          time.Time `json:"at"`
}

type NoteHistoryEntry struct {
	Disposition string    `json:"disposition"`
	Note        string    `json:"note"`
	Actor       string    `json:"actor"`
	At          time.Time `json:"at"`
}

type ReducedFindingState struct {
	FindingID   string
	LastRunID   string
	Disposition string
	Notes       []NoteHistoryEntry
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
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
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
		if err := json.Unmarshal(line, &event); err != nil {
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
