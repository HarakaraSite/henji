package cache

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConversationLockAcrossProcesses(t *testing.T) {
	if dir := os.Getenv("HENJI_TEST_LOCK_DIR"); dir != "" {
		c, err := NewConversations(dir)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		lock, err := c.Lock(ctx, os.Getenv("HENJI_TEST_LOCK_ID"))
		if os.Getenv("HENJI_TEST_LOCK_BUSY") == "1" {
			require.ErrorIs(t, err, context.DeadlineExceeded)
		} else {
			require.NoError(t, err)
			require.NoError(t, lock.Close())
		}
		return
	}
	dir := t.TempDir()
	c, err := NewConversations(dir)
	require.NoError(t, err)
	lock, err := c.Lock(context.Background(), "same")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lock.Close() })
	child := func(id string, busy bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConversationLockAcrossProcesses$")
		cmd.Env = append(os.Environ(), "HENJI_TEST_LOCK_DIR="+dir, "HENJI_TEST_LOCK_ID="+id, "HENJI_TEST_LOCK_BUSY=0")
		if busy {
			cmd.Env = append(cmd.Env, "HENJI_TEST_LOCK_BUSY=1")
		}
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	child("same", true)
	child("different", false)
	require.NoError(t, lock.Close())
	child("same", false)

	// Cancellation should stop a waiting writer without removing the stable
	// lock file or releasing another process's lock.
	lock, err = c.Lock(context.Background(), "same")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Lock(ctx, "same")
	require.True(t, errors.Is(err, context.Canceled))
}
