// Package cache provides a simple in-file cache implementation.
package cache

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Type represents the type of cache being used.
type Type string

// Cache types for different purposes.
const (
	ConversationCache Type = "conversations"
	TemporaryCache    Type = "temp"
)

const cacheExt = ".gob"

var errInvalidID = errors.New("invalid id")

// Cache is a generic cache implementation that stores data in files.
type Cache[T any] struct {
	baseDir string
	cType   Type
}

// New creates a new cache instance with the specified base directory and cache type.
func New[T any](baseDir string, cacheType Type) (*Cache[T], error) {
	dir := filepath.Join(baseDir, string(cacheType))
	if err := os.MkdirAll(dir, os.ModePerm); err != nil { //nolint:gosec
		return nil, fmt.Errorf("create cache directory: %w", err)
	}
	return &Cache[T]{
		baseDir: baseDir,
		cType:   cacheType,
	}, nil
}

func (c *Cache[T]) dir() string {
	return filepath.Join(c.baseDir, string(c.cType))
}

func (c *Cache[T]) Read(id string, readFn func(io.Reader) error) error {
	if id == "" {
		return fmt.Errorf("read: %w", errInvalidID)
	}
	file, err := os.Open(filepath.Join(c.dir(), id+cacheExt))
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	defer file.Close() //nolint:errcheck

	if err := readFn(file); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	return nil
}

func (c *Cache[T]) Write(id string, writeFn func(io.Writer) error) error {
	return c.WriteWithCommit(id, writeFn, nil)
}

// WriteWithCommit replaces a body only after its complete contents are staged.
// If commitFn fails, the previous body is restored, or a new body is removed.
func (c *Cache[T]) WriteWithCommit(id string, writeFn func(io.Writer) error, commitFn func() error) error {
	if id == "" {
		return fmt.Errorf("write: %w", errInvalidID)
	}
	staged, err := c.stageWrite(writeFn)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(staged) }()

	target := filepath.Join(c.dir(), id+cacheExt)
	var backup string
	if commitFn != nil {
		previous, err := os.Open(target)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read previous body: %w", err)
		}
		if err == nil {
			backup, err = c.stageWrite(func(w io.Writer) error {
				_, err := io.Copy(w, previous)
				return err
			})
			_ = previous.Close()
			if err != nil {
				return fmt.Errorf("back up previous body: %w", err)
			}
		}
	}
	keepBackup := false
	defer func() {
		if backup != "" && !keepBackup {
			_ = os.Remove(backup)
		}
	}()

	if err := os.Rename(staged, target); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	if commitFn != nil {
		if err := commitFn(); err != nil {
			var restoreErr error
			if backup != "" {
				restoreErr = os.Rename(backup, target)
				if restoreErr != nil {
					// Do not remove the surviving original if restoring it fails.
					keepBackup = true
					restoreErr = fmt.Errorf("restore previous body (backup retained at %s): %w", backup, restoreErr)
				}
			} else {
				restoreErr = os.Remove(target)
				if restoreErr != nil {
					restoreErr = fmt.Errorf("remove unsaved body: %w", restoreErr)
				}
			}
			return fmt.Errorf("commit: %w", errors.Join(err, restoreErr))
		}
	}
	return nil
}

func (c *Cache[T]) stageWrite(writeFn func(io.Writer) error) (name string, err error) {
	file, err := os.CreateTemp(c.dir(), ".henji-cache-*")
	if err != nil {
		return "", fmt.Errorf("stage write: %w", err)
	}
	name = file.Name()
	defer func() {
		_ = file.Close()
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if err := writeFn(file); err != nil {
		return name, fmt.Errorf("stage write: %w", err)
	}
	if err := file.Sync(); err != nil {
		return name, fmt.Errorf("sync staged body: %w", err)
	}
	if err := file.Close(); err != nil {
		return name, fmt.Errorf("close staged body: %w", err)
	}
	return name, nil
}

// Delete removes a cached item by its ID.
func (c *Cache[T]) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("delete: %w", errInvalidID)
	}
	if err := os.Remove(filepath.Join(c.dir(), id+cacheExt)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}
