//go:build !windows

package cache

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryWriteLock(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
		return errWriteLocked
	}
	return err
}
