package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var errWriteLocked = errors.New("conversation is being saved")

func (c *Cache[T]) lockWrite(ctx context.Context, id string) (*os.File, error) {
	if id == "" {
		return nil, errInvalidID
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dir := filepath.Join(c.dir(), ".locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	// Keep a stable lock file: unlinking it would let other processes lock a
	// different inode while a writer still holds the original file open.
	file, err := os.OpenFile(filepath.Join(dir, id+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open conversation lock: %w", err)
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		err := tryWriteLock(file)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, errWriteLocked) {
			_ = file.Close()
			return nil, fmt.Errorf("lock conversation: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
