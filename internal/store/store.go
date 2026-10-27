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

// Since counts entries first seen after t.
func (s Seen) Since(t time.Time) int {
	n := 0
	for _, seen := range s {
		if seen.After(t) {
			n++
		}
	}
	return n
}

// Save writes s to path atomically.
func (s Seen) Save(path string) error {
	return writeJSON(path, s)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
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

// Detail is a cached job description, so title-only sources are opened once per job.
type Detail struct {
	Text    string    `json:"text"`
	Fetched time.Time `json:"fetched"`
}

// Details maps job ID to its cached description.
type Details map[string]Detail

// LoadDetails reads the cache at path. A missing file is an empty cache.
func LoadDetails(path string) (Details, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Details{}, nil
	}
	if err != nil {
		return nil, err
	}
	d := Details{}
	return d, json.Unmarshal(data, &d)
}

// Prune drops descriptions fetched before now-maxAge.
func (d Details) Prune(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	for id, v := range d {
		if v.Fetched.Before(cutoff) {
			delete(d, id)
		}
	}
}

// Save writes d to path atomically.
func (d Details) Save(path string) error {
	return writeJSON(path, d)
}
