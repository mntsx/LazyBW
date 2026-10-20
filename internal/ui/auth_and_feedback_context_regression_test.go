package ui

import (
	"strings"
	"testing"
	"time"
)

// TestCipherIndicatorUsesBlueIcon checks both authentication screens' top labels.
func TestCipherIndicatorUsesBlueIcon(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		s.Screen = screen
		canvas := NewCanvas(145, 45)
		a.authRender(canvas)
		label := "AES-256-GCM / " + s.CLI.Cipher.Backend()
		x := canvas.W - 2 - len([]rune("\U000F0780 "+label))
		if got := canvasAt(canvas, x, 1); got.R != '\U000F0780' || got.FG != CBlue {
			t.Fatalf("screen %d: cipher icon is not blue: %+v", screen, got)
		}
		if got := canvasAt(canvas, x+2, 1); got.R != 'A' || got.FG != CWhite {
			t.Fatalf("screen %d: cipher label is not white: %+v", screen, got)
		}
	}
}

// TestBlankMasterNeverCallsUnlock validates the error before touching bw.
func TestBlankMasterNeverCallsUnlock(t *testing.T) {
	s := setup(t)
	s.Screen = AuthMaster
	s.AuthFocus = 2
	s.Busy = false
	s.ClearMaster()
	a := New(s)
	a.submitAuth()
	if s.Notice != "Incorrect master password. Try again" {
		t.Fatalf("unexpected empty-password error: %q", s.Notice)
	}
	if s.Busy || s.Screen != AuthMaster {
		t.Fatal("empty password must not enter the authentication phase")
	}
}

// TestTextEntryNeverRetainsKeyFeedback ensures typed secrets are not copied
// into the TUI's temporary key-monitoring state.
func TestTextEntryNeverRetainsKeyFeedback(t *testing.T) {
	s := setup(t)
	s.Screen = AuthMaster
	s.AuthFocus = 0
	s.Busy = false
	a := New(s)
	if err := a.handleKey("x", func() error { return a.char('x') }); err != nil {
		t.Fatal(err)
	}
	if s.MasterText() != "x" {
		t.Fatal("master-password input was not accepted")
	}
	if s.KeyFeedback != "" || s.KeyFeedbackAction != "" ||
		!s.KeyFeedbackExpires.IsZero() || a.feedbackActive.Load() {
		t.Fatal("text-entry key escaped into feedback monitoring")
	}
}

// TestHintPageKeyDoesNotTriggerFooterFeedback checks silent pagination.
func TestHintPageKeyDoesNotTriggerFooterFeedback(t *testing.T) {
	s := setup(t)
	a := New(s)
	s.CreateItem(1, "pagination-test")
	if len(PaginateHints(a.hints(), 90)) < 2 {
		t.Fatal("test needs multiple footer pages")
	}
	a.reportKey("f", "find")
	beforePage := s.HintPage
	handled, err := a.dispatchConfiguredKey("?")
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("configured next-page key was not handled")
	}
	if !s.KeyFeedbackExpires.IsZero() || a.feedbackActive.Load() {
		t.Fatal("hint pagination left temporary key feedback visible")
	}
	if s.HintPage == beforePage {
		t.Fatal("? must change the footer page immediately")
	}
	if strings.Contains(a.keyAction("?"), "unbound") {
		t.Fatal("hint pagination should remain registered")
	}
}

// TestFeedbackDependsOnCurrentSelection checks inapplicable shortcuts.
func TestFeedbackDependsOnCurrentSelection(t *testing.T) {
	s := setup(t)
	a := New(s)
	s.OpenItem()
	s.Focus = PaneContent
	s.FieldIndex = 1
	if !s.CurrentField().Hidden {
		t.Fatal("test requires a hidden login field")
	}
	if got := a.keyAction("ctrl+r"); got != "unbound" {
		t.Fatalf("workspace ctrl+r should be unbound, got %q", got)
	}
	if err := a.handleKey("ctrl+r", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if s.KeyFeedbackAction != "unbound" || len(s.Revealed) != 0 {
		t.Fatalf("inactive ctrl+r was accepted: %q", s.KeyFeedbackAction)
	}
	if err := a.handleKey("r", func() error { return a.char('r') }); err != nil {
		t.Fatal(err)
	}
	if s.KeyFeedbackAction != "reveal" || !s.Revealed[s.RevealKey()] {
		t.Fatal("content r must reveal the selected secret")
	}
	s.FieldIndex = 0
	if s.CurrentField().Hidden {
		t.Fatal("test requires a visible field")
	}
	if got := a.keyAction("r"); got != "unbound" {
		t.Fatalf("revealing a visible field is inapplicable: %s", got)
	}
	if err := a.handleKey("r", func() error { return a.char('r') }); err != nil {
		t.Fatal(err)
	}
	if s.KeyFeedbackAction != "unbound" {
		t.Fatalf("no-op reveal was reported as %s", s.KeyFeedbackAction)
	}
}

// TestFeedbackSuppressesMovementWithNoResult verifies boundary checks.
func TestFeedbackSuppressesMovementWithNoResult(t *testing.T) {
	s := setup(t)
	a := New(s)
	s.Focus = PaneItems
	s.ItemIndex = 0
	if err := a.handleKey("↑", func() error { return a.arrow(-1, 0) }); err != nil {
		t.Fatal(err)
	}
	if s.KeyFeedbackAction != "unbound" {
		t.Fatalf("navigation at the upper boundary must be unbound, got %s",
			s.KeyFeedbackAction)
	}
	s.KeyFeedbackExpires = time.Time{}
	if got := a.keyAction("ctrl+p"); got != "unbound" {
		t.Fatalf("save without staged changes should be unbound: %q", got)
	}
	s.Focus = PaneTypes
	if got := a.keyAction("f"); got != "unbound" {
		t.Fatalf("find outside items should be unbound: %q", got)
	}
	if a.aliasAvailable("content", "reveal") {
		t.Fatal("custom reveal alias must also honor the selected panel")
	}
}
