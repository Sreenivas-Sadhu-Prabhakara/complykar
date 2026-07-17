// Package store is an in-memory application store guarded by a mutex, with
// JSON snapshot persistence to disk (load on boot, save after every write).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"complykar/internal/notify"
	"complykar/internal/rules"
)

// Filing records that an obligation instance was marked as filed.
type Filing struct {
	ObligationID   string `json:"obligationId"`
	ObligationName string `json:"obligationName"`
	DueDate        string `json:"dueDate"`
	FiledAt        string `json:"filedAt"`
}

// State is the full persisted application state.
type State struct {
	Profile     *rules.Profile     `json:"profile"`
	Obligations []rules.Obligation `json:"obligations"`
	Filings     []Filing           `json:"filings"`
	Outbox      []notify.Message   `json:"outbox"`
}

// Store guards State with a mutex and snapshots it to a JSON file.
type Store struct {
	mu   sync.Mutex
	path string
	st   State
}

// New creates a store that persists to path.
func New(path string) *Store {
	return &Store{path: path}
}

// Load reads the snapshot from disk if present; a missing file is not an error.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &s.st)
}

// save writes the snapshot; the caller must hold the lock.
func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Snapshot returns a copy of the current state safe for concurrent reads.
func (s *Store) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := State{
		Obligations: append([]rules.Obligation(nil), s.st.Obligations...),
		Filings:     append([]Filing(nil), s.st.Filings...),
		Outbox:      append([]notify.Message(nil), s.st.Outbox...),
	}
	if s.st.Profile != nil {
		p := *s.st.Profile
		cp.Profile = &p
	}
	return cp
}

// SetProfile replaces the profile, its computed obligations and the outbox
// (fresh reminders). Filing history is preserved.
func (s *Store) SetProfile(p rules.Profile, obls []rules.Obligation, outbox []notify.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Profile = &p
	s.st.Obligations = obls
	s.st.Outbox = outbox
	return s.save()
}

// MarkFiled appends a filing; duplicate obligation+dueDate pairs are rejected.
func (s *Store) MarkFiled(f Filing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.st.Filings {
		if ex.ObligationID == f.ObligationID && ex.DueDate == f.DueDate {
			return fmt.Errorf("already filed: %s for %s", f.ObligationID, f.DueDate)
		}
	}
	s.st.Filings = append(s.st.Filings, f)
	return s.save()
}

// AppendOutbox adds messages to the outbox log.
func (s *Store) AppendOutbox(msgs ...notify.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Outbox = append(s.st.Outbox, msgs...)
	return s.save()
}
