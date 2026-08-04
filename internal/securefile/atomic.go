// Package securefile provides private, atomic output files for exports that
// may contain Salesforce data. A write is staged beside the destination and
// only becomes visible after Commit succeeds.
package securefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AtomicFile is an in-progress private file. Call Commit on success or Abort
// on every error path. The temporary file is always created with mode 0600.
type AtomicFile struct {
	file      *os.File
	target    string
	temp      string
	overwrite bool
}

// New creates a private temporary file in the destination directory. When
// overwrite is false, an existing destination is rejected both here and at
// commit time so concurrent writers cannot silently clobber it.
func New(path string, overwrite bool) (*AtomicFile, error) {
	if path == "" {
		return nil, errors.New("output path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	if !overwrite {
		if _, err := os.Lstat(path); err == nil {
			return nil, fmt.Errorf("%s: %w", path, os.ErrExist)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("check output: %w", err)
		}
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return nil, fmt.Errorf("create private temporary output: %w", err)
	}
	return &AtomicFile{file: f, target: path, temp: f.Name(), overwrite: overwrite}, nil
}

func (f *AtomicFile) Write(p []byte) (int, error) {
	if f == nil || f.file == nil {
		return 0, os.ErrClosed
	}
	return f.file.Write(p)
}

// Commit flushes and publishes the completed file. The no-overwrite path uses
// a hard link so destination creation is atomic and fails if another process
// won the race. Temporary and destination are in the same directory.
func (f *AtomicFile) Commit() error {
	if f == nil || f.file == nil {
		return os.ErrClosed
	}
	if err := f.file.Sync(); err != nil {
		_ = f.Abort()
		return fmt.Errorf("sync output: %w", err)
	}
	if err := f.file.Close(); err != nil {
		f.file = nil
		_ = os.Remove(f.temp)
		return fmt.Errorf("close output: %w", err)
	}
	f.file = nil
	if f.overwrite {
		if err := os.Rename(f.temp, f.target); err != nil {
			_ = os.Remove(f.temp)
			return fmt.Errorf("publish output: %w", err)
		}
		return nil
	}
	if err := os.Link(f.temp, f.target); err != nil {
		_ = os.Remove(f.temp)
		return fmt.Errorf("publish output without overwrite: %w", err)
	}
	if err := os.Remove(f.temp); err != nil {
		return fmt.Errorf("remove temporary output: %w", err)
	}
	return nil
}

// Abort closes and removes the unpublished temporary file.
func (f *AtomicFile) Abort() error {
	if f == nil {
		return nil
	}
	var closeErr error
	if f.file != nil {
		closeErr = f.file.Close()
		f.file = nil
	}
	removeErr := os.Remove(f.temp)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(closeErr, removeErr)
}

// Write writes a complete output through an AtomicFile.
func Write(path string, overwrite bool, write func(io.Writer) error) error {
	f, err := New(path, overwrite)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		_ = f.Abort()
		return err
	}
	return f.Commit()
}

func WriteFile(path string, body []byte, overwrite bool) error {
	return Write(path, overwrite, func(w io.Writer) error {
		_, err := w.Write(body)
		return err
	})
}
