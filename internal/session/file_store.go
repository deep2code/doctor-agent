package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// sessionIDPattern allows only safe filename characters for session IDs.
// conversation_id is attacker-controlled in the HTTP API, so any path
// traversal must be rejected before it reaches the filesystem.
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// ValidID reports whether id is a safe session identifier (used both for
// filenames and as an API input guard).
func ValidID(id string) bool {
	if !sessionIDPattern.MatchString(id) {
		return false
	}
	// Reject any ".." sequence even though the ".json" suffix already
	// neutralizes bare ".." — defense in depth.
	return !strings.Contains(id, "..")
}

// FileStore persists sessions as individual JSON files under a directory
// (one file per session ID). It is a simple, dependency-free snapshot store:
// each Save rewrites the session file atomically (write-temp-then-rename).
type FileStore struct {
	dir string
	mu  sync.Mutex
}

// NewFileStore creates a store rooted at dir, creating it if needed.
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("session store dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating session dir: %w", err)
	}
	return &FileStore{dir: dir}, nil
}

// path resolves a session file path, rejecting IDs that could escape the
// store directory (defense in depth on top of ValidID).
func (fs *FileStore) path(id string) (string, error) {
	if !ValidID(id) {
		return "", fmt.Errorf("invalid session id %q", id)
	}
	p := filepath.Join(fs.dir, id+".json")
	base := filepath.Clean(fs.dir) + string(filepath.Separator)
	if !strings.HasPrefix(filepath.Clean(p), base) {
		return "", fmt.Errorf("session id %q escapes store dir", id)
	}
	return p, nil
}

// Save writes a snapshot of the session. The caller should hold the session
// lock or otherwise ensure the snapshot is consistent.
func (fs *FileStore) Save(s *Session) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	path, err := fs.path(s.ID)
	if err != nil {
		return fmt.Errorf("session path: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write session temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename session file: %w", err)
	}
	return nil
}

// Load reads a session snapshot from disk. Returns nil, nil when the file
// does not exist (caller treats it as a fresh session).
func (fs *FileStore) Load(id string) (*Session, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	path, err := fs.path(id)
	if err != nil {
		return nil, fmt.Errorf("session path: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}
	s := &Session{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("unmarshal session %s: %w", id, err)
	}
	return s, nil
}

// List returns all persisted session IDs (sorted).
func (fs *FileStore) List() ([]string, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return nil, fmt.Errorf("read session dir: %w", err)
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		ids = append(ids, e.Name()[:len(e.Name())-len(".json")])
	}
	sort.Strings(ids)
	return ids, nil
}

// Delete removes a persisted session file (no-op if absent).
func (fs *FileStore) Delete(id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	path, err := fs.path(id)
	if err != nil {
		return fmt.Errorf("session path: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete session file: %w", err)
	}
	return nil
}
