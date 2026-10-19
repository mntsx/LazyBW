//go:build linux

package config

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// acquireConfigLock serializes configuration completion across LazyBW
// processes without creating a persistent lock file in the user's directory.
func acquireConfigLock(path string) (func(), error) {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(directory.Fd()), unix.LOCK_EX); err != nil {
		_ = directory.Close()
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(directory.Fd()), unix.LOCK_UN)
		_ = directory.Close()
	}, nil
}
