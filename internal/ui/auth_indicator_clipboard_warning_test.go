package ui

import (
	"strings"
	"testing"
	"time"
)

// TestAuthRegionLabelsHaveNoPrefix checks online and offline labels on both forms.
func TestAuthRegionLabelsHaveNoPrefix(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		for _, tc := range []struct {
			server   string
			local    bool
			expected string
		}{
			{server: "us", expected: "US (bitwarden.com)"},
			{server: "eu", expected: "EU (bitwarden.eu)"},
			{server: "local", local: true, expected: "Local"},
		} {
			state := setup(t)
			state.Screen = screen
			state.Server = tc.server
			state.LocalMode = tc.local
			state.Config.Local.VaultPath = "/tmp/bw-vault"
			canvas := NewCanvas(150, 42)
			New(state).authRender(canvas)
			first := stringCellRow(canvas, 1)
			if !strings.Contains(first, tc.expected) ||
				strings.Contains(first, "server:") {
				t.Fatalf("screen=%v server=%s unexpected label %q", screen, tc.server, first)
			}
		}
	}
}

// TestLocalWarningUsesAuthHintBox checks the shared box and warning colors.
func TestLocalWarningUsesAuthHintBox(t *testing.T) {
	state := setup(t)
	state.Screen = AuthMaster
	state.LocalWarning = "Local backup disabled: vault_path is empty"
	canvas := NewCanvas(120, 42)
	New(state).authRender(canvas)
	geometry := authGeometry(canvas.W, canvas.H, AuthMaster)
	warningX := (canvas.W - (len(state.LocalWarning) + 4)) / 2
	cell := canvasAt(canvas, warningX, geometry.tipY)
	if cell.BG != CBlack || cell.R != ' ' {
		t.Fatalf("warning has no upper box border: %+v", cell)
	}
	textCell := canvasAt(canvas, warningX+2, geometry.tipY+1)
	if textCell.FG != CRed || textCell.BG != CBlack || textCell.R != 'L' {
		t.Fatalf("warning text is not red on black: %+v", textCell)
	}
	lower := canvasAt(canvas, warningX, geometry.tipY+2)
	if lower.BG != CBlack {
		t.Fatalf("warning has no lower box border: %+v", lower)
	}

	state.Screen = AuthClient
	canvas = NewCanvas(120, 42)
	New(state).authRender(canvas)
	geometry = authGeometry(canvas.W, canvas.H, AuthClient)
	if !strings.Contains(stringCellRow(canvas, geometry.tipY+1),
		"Hint. Sign in automatically configuring API credentials in settings.json") {
		t.Fatal("shared box changed the original authentication hint")
	}
}

// TestClipboardNoticeHasOneSecondLifetime validates the footer's timer.
func TestClipboardNoticeHasOneSecondLifetime(t *testing.T) {
	state := setup(t)
	state.Screen = Workspace
	state.Notice = "Copied to clipboard"
	state.NoticeUntil = time.Now().Add(time.Second)
	canvas := NewCanvas(120, 44)
	New(state).mainRender(canvas)
	if !strings.Contains(stringCellRow(canvas, 42), "Copied to clipboard") {
		t.Fatal("copied feedback should be visible during its deadline")
	}
	state.NoticeUntil = time.Now().Add(-time.Millisecond)
	canvas = NewCanvas(120, 44)
	New(state).mainRender(canvas)
	if strings.Contains(stringCellRow(canvas, 42), "Copied to clipboard") {
		t.Fatal("copied feedback should disappear after its deadline")
	}
}
