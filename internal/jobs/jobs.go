// Package jobs owns JOBS_DIR: one subdir per batch conversion holding inputs,
// outputs, per-input logs, and a status.json. It backs the results view,
// download-all zip, stored logs, retry, and the TTL sweep.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileState is the lifecycle state of one input within a batch.
type FileState string

const (
	StatePending FileState = "pending"
	StateRunning FileState = "running"
	StateDone    FileState = "done"
	StateFailed  FileState = "failed"
)

// FileResult is one input's outcome within a batch.
type FileResult struct {
	Input      string           `json:"input"` // original filename
	State      FileState        `json:"state"`
	Outputs    []string         `json:"outputs"` // output basenames under <job>/out/<input>/
	Sizes      map[string]int64 `json:"sizes"`   // basename -> bytes
	LogLines   int              `json:"log_lines"`
	FirstError string           `json:"first_error"` // first line starting "ERR", else ""
	Err        string           `json:"err"`         // short message, "" on success
	Millis     int64            `json:"millis"`
}

// Status is the whole batch, persisted as <job>/status.json.
type Status struct {
	ID     string       `json:"id"`
	Preset string       `json:"preset"` // preset id used ("" = Defaults)
	Format string       `json:"format"`
	Files  []FileResult `json:"files"`
	Done   bool         `json:"done"` // all files terminal
}

// Store owns JOBS_DIR. One subdir per job:
// <id>/in/, <id>/out/<input>/, <id>/<input>.log, <id>/status.json
type Store struct {
	Dir string
	TTL time.Duration
}

// NewStore returns a Store rooted at dir with the given sweep TTL.
func NewStore(dir string, ttl time.Duration) *Store { return &Store{Dir: dir, TTL: ttl} }

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) root(id string) string       { return filepath.Join(s.Dir, id) }
func (s *Store) statusPath(id string) string { return filepath.Join(s.root(id), "status.json") }

// Create makes the job tree and writes the initial status.
func (s *Store) Create(preset, format string) (string, error) {
	id := newID()
	for _, d := range []string{filepath.Join(s.root(id), "in"), filepath.Join(s.root(id), "out")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}
	if err := s.Save(id, &Status{ID: id, Preset: preset, Format: format}); err != nil {
		return "", err
	}
	return id, nil
}

// InputPath is <id>/in/<input>. filepath.Base neutralizes any path components.
func (s *Store) InputPath(id, input string) string {
	return filepath.Join(s.root(id), "in", filepath.Base(input))
}

// OutDir is <id>/out/<input>.
func (s *Store) OutDir(id, input string) string {
	return filepath.Join(s.root(id), "out", filepath.Base(input))
}

// LogPath is <id>/<input>.log.
func (s *Store) LogPath(id, input string) string {
	return filepath.Join(s.root(id), filepath.Base(input)+".log")
}

// Load reads and decodes status.json.
func (s *Store) Load(id string) (*Status, error) {
	b, err := os.ReadFile(s.statusPath(id))
	if err != nil {
		return nil, err
	}
	var st Status
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// Save writes status.json atomically (temp file + rename).
func (s *Store) Save(id string, st *Status) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	final := s.statusPath(id)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// OutputFile returns the safe-joined path to one output, rejecting names that
// contain a path separator or "..".
func (s *Store) OutputFile(id, input, base string) (string, error) {
	if err := safeName(input); err != nil {
		return "", err
	}
	if err := safeName(base); err != nil {
		return "", err
	}
	return filepath.Join(s.root(id), "out", input, base), nil
}

func safeName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid name %q", name)
	}
	return nil
}
