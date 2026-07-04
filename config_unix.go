//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// securePermissions checks an existing settings file's owner and permission
// bits before its contents (which may include a plaintext api-key) are read,
// and repairs group/other-accessible permission bits in place. Files owned
// by another user are refused outright rather than silently read, since a
// failed chmod would otherwise leave a false sense of security.
func securePermissions(path string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(stat.Uid) != os.Getuid() {
		return modsError{
			fmt.Errorf("%s is owned by uid %d, not the current user", path, stat.Uid),
			"Refusing to read a settings file owned by another user.",
		}
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o600); err != nil { //nolint:mnd
			return modsError{err, "Could not restrict settings file permissions to 0600."}
		}
	}
	return nil
}
