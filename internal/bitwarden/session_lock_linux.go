//go:build linux

package bitwarden

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func lockSession(dir string, nonblocking bool) (*os.File, error) {
	file, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	operation := unix.LOCK_EX
	if nonblocking {
		operation |= unix.LOCK_NB
	}
	if err := unix.Flock(int(file.Fd()), operation); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func releaseSessionLock(file *os.File) {
	if file == nil {
		return
	}
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
	_ = file.Close()
}
