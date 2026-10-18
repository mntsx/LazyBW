package ui

import (
	"context"
	"time"

	"github.com/awesome-gocui/gocui"
	"github.com/miguelmnts/lazybw-go/internal/localvault"
	"github.com/miguelmnts/lazybw-go/internal/network"
	"github.com/miguelmnts/lazybw-go/internal/replica"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// backupJob contains an immutable snapshot of confirmed remote data.
type backupJob struct {
	data      localvault.Snapshot
	region    string
	path      string
	algorithm string
	session   *localvault.Session
}

// checkOfflineStartup retains the configured region even when it is unreachable.
func (a *App) checkOfflineStartup(ctx context.Context) {
	cfg := a.S.Config
	region := a.S.Server
	url := cfg.Network.USPingURL
	if region == "eu" {
		url = cfg.Network.EUPingURL
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Second)
	available := network.Reachable(probeCtx, url)
	cancel()
	if ctx.Err() != nil || a.gui == nil {
		return
	}
	if available {
		a.startup(ctx, a.gui, true)
		return
	}
	a.gui.Update(func(*gocui.Gui) error {
		a.S.ServerConnected = false
		a.S.Screen = AuthClient
		a.S.Busy = false
		a.S.AuthFocus = 2
		a.S.Notice = "Server connection lost"
		return nil
	})
}

func (a *App) switchToLocal() {
	s := a.S
	s.Server = "local"
	s.LocalMode = true
	s.Screen = AuthMaster
	s.Mode = Browse
	s.Busy = false
	s.AuthFocus = 0
	s.Caret = 0
	s.ClearMaster()
	s.Notice = ""
	s.LocalWarning = ""
	if h, err := localvault.Inspect(s.Config.Local.VaultPath); err == nil {
		s.LocalOrigin = h.Origin
	}
}

func (a *App) openServerPicker() {
	s := a.S
	s.Mode = DialogServer
	s.ServerReady = [2]bool{}
	s.ServerMoved = false
	s.ServerChoice = -1
	if localvault.Available(s.Config.Local.VaultPath) {
		s.ServerChoice = 2
	}
	if a.pickerCancel != nil {
		a.pickerCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.pickerCancel = cancel
	if a.gui == nil {
		return
	}
	desired := s.Server
	if desired == "local" {
		desired = s.Config.Authentication.DefaultServer
	}
	for index, endpoint := range []string{s.Config.Network.USPingURL, s.Config.Network.EUPingURL} {
		go func(i int, url string) {
			const interval = 2500 * time.Millisecond
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			var failedSince time.Time
			for {
				started := time.Now()
				probe, done := context.WithTimeout(ctx, interval)
				good := network.Reachable(probe, url)
				done()
				if ctx.Err() != nil {
					return
				}
				now := time.Now()
				if good {
					failedSince = time.Time{}
				} else if failedSince.IsZero() {
					failedSince = started
				}
				// The server is unavailable after 2.5 consecutive seconds of
				// failed probes. A successful probe restores it immediately.
				ready := good
				outageConfirmed := !good && now.Sub(failedSince) >= interval
				a.gui.Update(func(*gocui.Gui) error {
					if a.S.Mode != DialogServer {
						return nil
					}
					if good || outageConfirmed {
						a.S.ServerReady[i] = ready
					}
					if good && !a.S.ServerMoved &&
						desired == []string{"us", "eu"}[i] {
						a.S.ServerChoice = i
					}
					return nil
				})
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}(index, endpoint)
	}
}

func (a *App) closeServerPicker() {
	if a.pickerCancel != nil {
		a.pickerCancel()
		a.pickerCancel = nil
	}
	a.S.Mode = Browse
}

func (a *App) selectServerChoice() {
	s := a.S
	choice := s.ServerChoice
	if choice == 2 {
		if !localvault.Available(s.Config.Local.VaultPath) {
			return
		}
		a.closeServerPicker()
		a.switchToLocal()
		return
	}
	if choice < 0 || choice > 1 || !s.ServerReady[choice] {
		return
	}
	region := []string{"us", "eu"}[choice]
	a.closeServerPicker()
	// The picker has just independently verified this endpoint.
	s.ServerConnected = true
	a.connectionFailureSince = time.Time{}
	a.connectionFailureRegion = ""
	if region == s.Server && s.Screen != AuthOffline {
		if s.Notice == "Server connection lost" {
			s.Notice = ""
		}
		return
	}
	s.LocalMode = false
	s.Server = region
	s.AuthID, s.AuthSecret = "", ""
	s.ClearMaster()
	s.AuthFocus, s.Caret, s.AuthVisible = 0, 0, false
	s.Notice = ""
	s.Screen = AuthChecking
	s.Busy = true
	s.BusyMessage = "Checking Bitwarden session"
	if a.gui != nil {
		go a.startup(context.Background(), a.gui, true)
	}
}

func (a *App) localWarning() string {
	cfg := a.S.Config.Local
	if !cfg.SaveVault {
		return ""
	}
	if err := localvault.Validate(cfg.VaultPath, cfg.VaultEncryption); err != nil {
		return "Local backup disabled: " + err.Error()
	}
	return ""
}

// confirmedSnapshot excludes staged changes and all organization-owned items.
func (a *App) confirmedSnapshot() localvault.Snapshot {
	s := a.S
	data := localvault.Snapshot{Items: []map[string]any{}, Folders: []map[string]any{}}
	for _, item := range s.Originals {
		if org := item["organizationId"]; org != nil && org != "" {
			continue
		}
		data.Items = append(data.Items, vault.Clone(item))
	}
	for _, folder := range s.FolderOriginals {
		data.Folders = append(data.Folders, vault.Clone(folder))
	}
	return data
}

func (a *App) backupAllowed() bool {
	s := a.S
	return !s.LocalMode && s.Config.Local.SaveVault && s.LocalSession != nil &&
		a.localWarning() == ""
}

// queueBackup schedules a snapshot without accessing Bitwarden again.
func (a *App) queueBackup() {
	a.queueBackupAt(0)
}

// prepareBackup snapshots only confirmed data before any background work begins.
func (a *App) prepareBackup() backupJob {
	cfg := a.S.Config.Local
	return backupJob{data: a.confirmedSnapshot(), region: a.S.Server,
		path: cfg.VaultPath, algorithm: cfg.VaultEncryption,
		session: a.S.LocalSession}
}

// queueBackupAfterRegistration delays the backup until the registration notice expires.
// A later registration replaces and moves the pending backup to the end of the queue.
func (a *App) queueBackupAfterRegistration() {
	a.queueBackupAt(time.Second)
}

// queueBackupAt keeps at most one pending backup, always using the latest
// confirmed snapshot. A backup already being written is never interrupted.
func (a *App) queueBackupAt(delay time.Duration) {
	if !a.backupAllowed() || a.backupJobs == nil {
		return
	}
	job := a.prepareBackup()
	a.backupMu.Lock()
	if a.backupPending == nil {
		a.jobsPending.Add(1)
	}
	a.backupPending = &job
	a.backupDue = time.Now().Add(delay)
	a.backupMu.Unlock()
	select {
	case a.backupJobs <- struct{}{}:
	default:
	}
}

// setRegistrationInProgress defers a queued backup until the current remote
// registration batch is committed (or finishes with an error).
func (a *App) setRegistrationInProgress(active bool) {
	a.backupMu.Lock()
	a.registrationInProgress = active
	a.backupMu.Unlock()
	if !active && a.backupJobs != nil {
		select {
		case a.backupJobs <- struct{}{}:
		default:
		}
	}
}

// nextBackup waits until the newest pending backup is due. Replacements reset
// its deadline without adding another job or an additional CLI operation.
func (a *App) nextBackup(ctx context.Context) (backupJob, bool) {
	for {
		a.backupMu.Lock()
		if a.backupPending != nil && !a.registrationInProgress &&
			!time.Now().Before(a.backupDue) {
			job := *a.backupPending
			a.backupPending = nil
			a.backupMu.Unlock()
			return job, true
		}
		var delay time.Duration
		hasPending := a.backupPending != nil && !a.registrationInProgress
		if hasPending {
			delay = time.Until(a.backupDue)
		}
		a.backupMu.Unlock()
		if !hasPending {
			select {
			case <-ctx.Done():
				return backupJob{}, false
			case <-a.backupJobs:
			}
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return backupJob{}, false
		case <-a.backupJobs:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (a *App) backupWorker(ctx context.Context) {
	for {
		job, ok := a.nextBackup(ctx)
		if !ok {
			return
		}
		// A pending registration confirmation has finished before this spinner.
		a.gui.Update(func(*gocui.Gui) error {
			if a.S.Notice == "Edits pushed" {
				a.S.ClearWorkspaceNotice()
			}
			a.S.BackupBusy = true
			a.S.BusyMessage = "Encrypting local vault"
			return nil
		})
		started := time.Now()
		jobCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := replica.Write(jobCtx, job.data, job.region, job.path,
			job.algorithm, job.session, a.S.CLI.Cipher)
		// Keep the spinner visible for fast snapshots.
		if wait := 180*time.Millisecond - time.Since(started); wait > 0 {
			time.Sleep(wait)
		}
		cancel()
		a.gui.Update(func(*gocui.Gui) error {
			a.jobsPending.Add(-1)
			a.S.BackupBusy = false
			if err != nil {
				a.S.SetWorkspaceNotice("Local backup failed", CRed)
			} else {
				a.S.SetWorkspaceNotice("Local vault backed up", CBlue)
			}
			return a.quitResult(nil)
		})
	}
}

func (a *App) offlineRender(c *Canvas) {
	title := "Unable to connect to either Bitwarden server"
	c.Text(max(0, (c.W-len(title))/2), c.H/2-2, title, CRed, CDefault)
	w := 12
	x := (c.W - w) / 2
	a.authButton(c, x, c.H/2+1, w, "QUIT", CRed, false, false)
	authHints(c, c.H-3, joinConfiguredHints(
		a.configuredHint("authentication_offline", "server", "server"),
		a.configuredHint("authentication_offline", "quit", "quit")))
	drawMainBorders(c, nil, nil, -1)
}
