package ui

import (
	"strings"
	"testing"
	"time"
)

// TestPopupContentSpacing validates the last item and pagination margins.
func TestPopupContentSpacing(t *testing.T) {
	for _, number := range []int{2, 9, 10, 11, 23} {
		state := setup(t)
		lines := make([]string, number)
		for idx := range lines {
			lines[idx] = "Changed item"
		}
		state.Popup = &Popup{Title: "Register changes?", Question: "Register?", Lines: lines}
		app := New(state)
		canvas := NewCanvas(120, 48)
		app.popupRender(canvas)
		buttonY := app.dialogY + app.dialogH - 4
		if number <= 10 {
			itemY := app.dialogY + 5 + number - 1
			if buttonY-itemY < 2 {
				t.Fatalf("%d items: no empty row above buttons", number)
			}
			continue
		}
		paginationY := app.dialogY + app.dialogH - 6
		lastItemY := app.dialogY + 14
		if paginationY-lastItemY < 2 || buttonY-paginationY < 2 {
			t.Fatalf("%d items: content/pagination/button spacing collapsed", number)
		}
		if !strings.Contains(stringCellRow(canvas, paginationY), "page 1/") {
			t.Fatalf("%d items: missing pagination", number)
		}
	}
}

// TestConnectionLossRequiresSustainedFailures verifies the 2.5-second threshold.
func TestConnectionLossRequiresSustainedFailures(t *testing.T) {
	state := setup(t)
	state.Screen = Workspace
	state.Server = "us"
	app := New(state)
	started := time.Now()
	app.applyConnectionResultAt("us", false, started)
	app.applyConnectionResultAt("us", false, started.Add(2*time.Second))
	if !state.ServerConnected {
		t.Fatal("outage reported before 2.5 seconds")
	}
	app.applyConnectionResultAt("us", true, started.Add(4500*time.Millisecond))
	app.applyConnectionResultAt("us", false, started.Add(6*time.Second))
	if !state.ServerConnected {
		t.Fatal("a recovery should reset consecutive-failure counting")
	}
	app.applyConnectionResultAt("us", false, started.Add(11*time.Second))
	if state.ServerConnected {
		t.Fatal("sustained failures were not reported")
	}
	app.applyConnectionResultAt("us", true, started.Add(12*time.Second))
	if !state.ServerConnected {
		t.Fatal("recovery should clear the alert immediately")
	}
	app.applyConnectionResultAt("eu", false, started.Add(20*time.Second))
	if !state.ServerConnected {
		t.Fatal("ignored stale region should not change availability")
	}
}

// TestTemporaryWorkspaceNoticeHasGuaranteedExpiry checks colored notices and fallback.
func TestTemporaryWorkspaceNoticeHasGuaranteedExpiry(t *testing.T) {
	state := setup(t)
	state.Screen = Workspace
	app := New(state)
	for _, test := range []struct {
		message string
		color   int
	}{
		{"Edits pushed", CGreen},
		{"Edits push failed", CRed},
		{"Edits discarded", CWhite},
		{"Copied to clipboard", CWhite},
		{"Local vault backed up", CBlue},
		{"Local backup failed", CRed},
	} {
		state.SetWorkspaceNotice(test.message, test.color)
		duration := time.Until(state.NoticeUntil)
		if duration <= 0 || duration > time.Second {
			t.Fatalf("%q duration outside one second", test.message)
		}
		canvas := NewCanvas(130, 44)
		app.mainRender(canvas)
		row := stringCellRow(canvas, 42)
		idx := strings.Index(row, test.message)
		if idx < 0 || canvasAt(canvas, idx, 42).FG != test.color {
			t.Fatalf("notice %q is missing or has the wrong color", test.message)
		}
		state.NoticeUntil = time.Now().Add(-time.Millisecond)
		canvas = NewCanvas(130, 44)
		app.mainRender(canvas)
		if strings.Contains(stringCellRow(canvas, 42), test.message) {
			t.Fatalf("expired footer notice remained visible: %q", test.message)
		}
	}
	state.Notice = "Staged new Login"
	state.NoticeUntil = time.Time{}
	canvas := NewCanvas(130, 44)
	app.mainRender(canvas)
	if strings.Contains(stringCellRow(canvas, 42), state.Notice) {
		t.Fatal("untimed status must never remain permanently in footer")
	}
}

// TestCreatingItemsNeverShowsPermanentStageNotice enforces the stage-only behavior.
func TestCreatingItemsNeverShowsPermanentStageNotice(t *testing.T) {
	state := setup(t)
	state.Screen = Workspace
	state.CreateItem(1, "created")
	if state.Notice != "" {
		t.Fatalf("unexpected stage message: %q", state.Notice)
	}
	state.CreateFolder("new folder")
	if state.Notice != "" {
		t.Fatalf("unexpected folder stage message: %q", state.Notice)
	}
}

// TestRegistrationFailureNoticePrecedesBackup protects the red one-second alert.
func TestRegistrationFailureNoticePrecedesBackup(t *testing.T) {
	state := setup(t)
	state.Screen = Workspace
	state.Config.Local.SaveVault = true
	state.Config.Local.VaultPath = t.TempDir() + "/bw-vault"
	state.Config.Local.VaultEncryption = "aes-256-gcm"
	// Validate backup scheduling separately: the alert must last one second
	// even when background operations are enabled.
	app := New(state)
	app.registrationFailed()
	if state.Notice != "Edits push failed" || state.NoticeColor != CRed {
		t.Fatalf("registration error notice: %q, color %d", state.Notice, state.NoticeColor)
	}
	duration := time.Until(state.NoticeUntil)
	if duration <= 0 || duration > time.Second {
		t.Fatalf("registration failure timeout is not one second: %v", duration)
	}
	if state.Busy || state.Popup != nil {
		t.Fatal("failed registration did not clear its UI state")
	}
}
