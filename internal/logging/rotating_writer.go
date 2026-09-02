// Package logging provides a rotating file writer for slog output.
//
// The RotatingWriter writes to a log file in a fixed directory (next to the
// app binary / working dir). When the file exceeds maxSize bytes it is renamed
// with a millisecond timestamp and a fresh file is opened. Files older than
// maxAge are deleted on startup and periodically during writes.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RotatingWriter is an io.Writer that rotates log files by size and age.
type RotatingWriter struct {
	mu       sync.Mutex
	dir      string        // log directory (e.g. "logs")
	name     string        // base file name (e.g. "doctor-agent.log")
	maxSize  int64         // max bytes per file before rotation
	maxAge   time.Duration // max age of old log files

	file       *os.File
	fileSize   int64
	lastClean  time.Time
}

// NewRotatingWriter creates a writer that logs to dir/name with the given
// rotation policy. maxSizeMB is the max size in MB per file; maxAgeDays is
// how long old (rotated) log files are kept. The directory is created if
// missing. On startup any existing current file that already exceeds
// maxSize is rotated, and files older than maxAge are deleted.
func NewRotatingWriter(dir, name string, maxSizeMB, maxAgeDays int) (*RotatingWriter, error) {
	w := &RotatingWriter{
		dir:     dir,
		name:    name,
		maxSize: int64(maxSizeMB) * 1024 * 1024,
		maxAge:  time.Duration(maxAgeDays) * 24 * time.Hour,
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating log dir %s: %w", dir, err)
	}
	// Open (or create) the current log file.
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", path, err)
	}
	w.file = f
	if info, err := f.Stat(); err == nil {
		w.fileSize = info.Size()
	}
	// If the existing file already exceeds the limit, rotate it now so
	// we start with a fresh file instead of immediately rotating on the
	// first write.
	if w.fileSize >= w.maxSize {
		w.rotateLocked()
	}
	// Delete old rotated files.
	w.cleanupLocked()
	return w, nil
}

// Write implements io.Writer.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Lazily reopen if the file was closed by a previous rotation.
	if w.file == nil {
		path := filepath.Join(w.dir, w.name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return 0, err
		}
		w.file = f
		if info, err := f.Stat(); err == nil {
			w.fileSize = info.Size()
		}
	}

	n, err := w.file.Write(p)
	if err != nil {
		return n, err
	}
	w.fileSize += int64(n)

	if w.fileSize >= w.maxSize {
		w.rotateLocked()
	}

	// Periodic cleanup (~once per hour) to remove aged files.
	if time.Since(w.lastClean) > time.Hour {
		w.cleanupLocked()
	}
	return n, nil
}

// Close flushes and closes the current log file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// rotateLocked renames the current file to a timestamped backup and opens a
// fresh file. Caller must hold w.mu.
func (w *RotatingWriter) rotateLocked() {
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}
	src := filepath.Join(w.dir, w.name)
	ts := time.Now().Format("20060102-150405.000")
	dst := filepath.Join(w.dir, fmt.Sprintf("%s.%s%s", trimExt(w.name), ts, filepath.Ext(w.name)))
	// Rename; if the destination somehow exists, remove it first.
	os.Remove(dst)
	if err := os.Rename(src, dst); err != nil {
		// If rename fails (e.g. cross-device), fall back to truncate so we
		// at least start fresh instead of growing unbounded.
		_ = os.Truncate(src, 0)
	}
	w.fileSize = 0
}

// cleanupLocked deletes rotated log files older than maxAge. Caller must hold
// w.mu.
func (w *RotatingWriter) cleanupLocked() {
	w.lastClean = time.Now()
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-w.maxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(w.dir, e.Name()))
		}
	}
}

// trimExt removes the file extension from name (e.g. "doctor-agent.log" → "doctor-agent").
func trimExt(name string) string {
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)]
}
