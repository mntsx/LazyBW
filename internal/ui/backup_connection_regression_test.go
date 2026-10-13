package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miguelmnts/lazybw-go/internal/localvault"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestLatestRegistrationReplacesPendingBackup checks coalescing without CLI calls.
func TestLatestRegistrationReplacesPendingBackup(t *testing.T) {
	s := setup(t)
	s.Config.Local.SaveVault = true
	s.Config.Local.VaultPath = filepath.Join(t.TempDir(), "bw-vault")
	s.Config.Local.VaultEncryption = localvault.AESGCM
	s.LocalSession = &localvault.Session{}
	a := New(s)
	a.backupJobs = make(chan struct{}, 1)
	a.queueBackupAfterRegistration()
	originalDue := a.backupDue
	if count := a.jobsPending.Load(); count != 1 {
		t.Fatalf("expected one pending backup, got %d", count)
	}

	s.Originals["id-1"]["name"] = "newest confirmed value"
	time.Sleep(time.Millisecond)
	a.queueBackupAfterRegistration()
	if a.jobsPending.Load() != 1 || !a.backupDue.After(originalDue) {
		t.Fatal("another registration must replace and postpone the queued backup")
	}
	a.backupMu.Lock()
	a.backupDue = time.Now().Add(-time.Millisecond)
	a.backupMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	job, ok := a.nextBackup(ctx)
	if !ok || len(job.data.Items) == 0 ||
		job.data.Items[0]["name"] != "newest confirmed value" {
		t.Fatalf("coalesced job is stale or missing: %#v", job.data.Items)
	}
	if a.backupPending != nil {
		t.Fatal("there must be no additional pending backup")
	}
}

// TestRegistrationPausesScheduledBackup checks that a pending backup waits.
func TestRegistrationPausesScheduledBackup(t *testing.T) {
	s := setup(t)
	s.Config.Local.SaveVault = true
	s.Config.Local.VaultPath = filepath.Join(t.TempDir(), "bw-vault")
	s.Config.Local.VaultEncryption = localvault.AESGCM
	s.LocalSession = &localvault.Session{}
	a := New(s)
	a.backupJobs = make(chan struct{}, 1)
	a.setRegistrationInProgress(true)
	a.queueBackup()
	result := make(chan backupJob, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		job, ok := a.nextBackup(ctx)
		if ok {
			result <- job
		}
	}()
	select {
	case <-result:
		t.Fatal("a backup started during registration")
	case <-time.After(20 * time.Millisecond):
	}
	a.setRegistrationInProgress(false)
	select {
	case <-result:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("backup did not resume after registration")
	}
}

// TestLostConnectionBlocksNewRegistrations checks both UI and action guards.
func TestLostConnectionBlocksNewRegistrations(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.Server = "us"
	a := New(s)
	a.applyConnectionResult("eu", false)
	if !s.ServerConnected {
		t.Fatal("stale region incorrectly changed connection status")
	}
	firstFailure := time.Now()
	a.applyConnectionResultAt("us", false, firstFailure)
	if !s.ServerConnected {
		t.Fatal("one failed probe must not immediately report an outage")
	}
	a.applyConnectionResultAt("us", false, firstFailure.Add(connectionLossThreshold))
	if s.ServerConnected {
		t.Fatal("sustained connection loss was not reported")
	}
	s.CreateItem(vault.Login, "pending")
	a.confirmStage(true, true)
	if s.Popup != nil {
		t.Fatal("registration confirmation was available while offline")
	}
	a.saveChanges(s.Stage.Changes())
	if s.Busy {
		t.Fatal("offline action unexpectedly started a remote registration")
	}
	canvas := NewCanvas(130, 44)
	a.mainRender(canvas)
	if !strings.Contains(stringCellRow(canvas, 42), "Server connexion lost") {
		t.Fatal("missing connection loss indicator")
	}
	a.applyConnectionResult("us", true)
	if !s.ServerConnected {
		t.Fatal("reconnection was not detected")
	}
	a.confirmStage(true, true)
	if s.Popup == nil {
		t.Fatal("registration was not restored after reconnection")
	}
}

// TestSuccessfulBackupNoticeIsBlue keeps backup and registration colors distinct.
func TestSuccessfulBackupNoticeIsBlue(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.SetWorkspaceNotice("Local vault backed up", CBlue)
	a := New(s)
	canvas := NewCanvas(130, 44)
	a.mainRender(canvas)
	footer := stringCellRow(canvas, 42)
	idx := strings.Index(footer, s.Notice)
	if idx < 0 || canvasAt(canvas, idx, 42).FG != CBlue {
		t.Fatal("local backup success must use normal blue")
	}
	s.SetWorkspaceNotice("Edits pushed", CGreen)
	canvas = NewCanvas(130, 44)
	a.mainRender(canvas)
	footer = stringCellRow(canvas, 42)
	idx = strings.Index(footer, s.Notice)
	if idx < 0 || canvasAt(canvas, idx, 42).FG != CGreen {
		t.Fatal("remote registration success must remain green")
	}
}

// TestOfflineRegistrationHintsAreUnavailable avoids advertising disabled actions.
func TestOfflineRegistrationHintsAreUnavailable(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.ServerConnected = false
	s.CreateItem(vault.Login, "pending")
	s.Focus = PaneChanges
	a := New(s)
	hints := strings.Join(a.hints(), " ")
	if strings.Contains(hints, "push") || a.keyAction("ctrl+p") != "unbound" ||
		a.keyAction("s") != "unbound" || a.keyAction("enter") != "unbound" {
		t.Fatalf("remote registration still available offline: %q", hints)
	}
	s.ServerConnected = true
	if !strings.Contains(strings.Join(a.hints(), " "), "push") {
		t.Fatal("registration hints did not return after reconnection")
	}
}
