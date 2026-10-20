package bitwarden

import (
	"os"
	"path/filepath"
	"time"
)

const abandonedSessionAge = 24 * time.Hour

// cleanupAbandonedSessions removes old session directories only after obtaining
// their advisory lock. A running LazyBW process retains that lock for its full
// lifetime, so its directory is never considered abandoned.
func cleanupAbandonedSessions(base string, now time.Time) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < abandonedSessionAge {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		lock, err := lockSession(dir, true)
		if err != nil {
			continue
		}
		_ = os.RemoveAll(dir)
		releaseSessionLock(lock)
	}
}
