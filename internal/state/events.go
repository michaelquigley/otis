package state

import (
	"errors"
	"os"
	"time"
)

const (
	EventSupervisorStarted = "supervisor_started"
	EventSupervisorStopped = "supervisor_stopped"
	EventPassDispatched    = "pass_dispatched"
)

type SupervisorEvent struct {
	Type    string
	Project string `dd:",+omitempty"`
	Pass    string `dd:",+omitempty"`
	RunID   string `dd:",+omitempty"`
	At      time.Time
	Message string `dd:",+omitempty"`
}

// AppendSupervisorEvent appends one supervisor lifecycle event.
func (s *Store) AppendSupervisorEvent(event SupervisorEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if event.Type == "" {
		return errors.New("supervisor event type is required")
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	} else {
		event.At = event.At.UTC()
	}
	raw, err := marshalDDJSONLine(event)
	if err != nil {
		return err
	}
	path := SupervisorEventsPath(s.root)
	if err := os.MkdirAll(SupervisorDir(s.root), 0o700); err != nil {
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
