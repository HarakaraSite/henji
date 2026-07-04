//go:build windows

package main

import "os"

// securePermissions is a no-op on Windows: file access there is governed by
// ACLs, not Unix mode bits, and ACL-based enforcement isn't implemented yet.
func securePermissions(path string, info os.FileInfo) error {
	return nil
}
