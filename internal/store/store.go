// Package store remembers which jobs have already been sent.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Seen maps job ID to the time it was first seen.
type Seen map[string]time.Time

// Load reads the file at path. A missing file is an empty set.
func Load(path string) (Seen, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Seen{}, nil
	}
	if err != nil {
		return nil, err
	}
	s := Seen{}
	return s, json.Unmarshal(data, &s)
}

// Prune drops entries older than maxAge so the file stays small.
// A job still open after that is sent again, which is a useful reminder.
func (s Seen) Prune(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	for id, t := range s {
		if t.Before(cutoff) {
			delete(s, id)
		}
	}
}

// Save writes s to path atomically.
func (s Seen) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
