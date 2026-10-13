package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awesome-gocui/gocui"
)

// TestQuitIgnoresFeedbackTimeout checks that a pending hint does not defer exit.
func TestQuitIgnoresFeedbackTimeout(t *testing.T) {
	s := setup(t)
	s.Screen = AuthMaster
	s.Busy = false
	a := New(s)
	a.gui = &gocui.Gui{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.cancel = cancel
	a.reportKey("x", "move")
	if !a.feedbackActive.Load() {
		t.Fatal("expected active feedback before quit")
	}
	start := time.Now()
	err := a.handleKey("ctrl+q", func() error { a.requestQuit(); return nil })
	if !errors.Is(err, gocui.ErrQuit) {
		t.Fatalf("quit must propagate directly, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("quit waited for feedback timeout: %v", elapsed)
	}
	if a.feedbackActive.Load() || !s.KeyFeedbackExpires.IsZero() {
		t.Fatal("quit did not clear transient feedback")
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("quit did not cancel active tasks")
	}
}

// TestMouseQuitReturnsImmediateSignal covers the authentication QUIT button.
func TestMouseQuitReturnsImmediateSignal(t *testing.T) {
	s := setup(t)
	s.Screen = AuthMaster
	s.Busy = false
	a := New(s)
	a.gui = &gocui.Gui{}
	g := authGeometry(120, 40, s.Screen)
	err := a.quitResult(a.authClickAt(g.quitX+1, g.buttonY, 120, 40))
	if !errors.Is(err, gocui.ErrQuit) {
		t.Fatalf("mouse QUIT must exit the event loop directly, got: %v", err)
	}
}

// TestQuitConfirmationStillBlocks verifies pending changes are not discarded.
func TestQuitConfirmationStillBlocks(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.Busy = false
	s.CreateItem(1, "Unsaved")
	a := New(s)
	a.gui = &gocui.Gui{}
	err := a.handleKey("ctrl+q", func() error { a.requestQuit(); return nil })
	if err != nil || a.exitRequested.Load() || s.Popup == nil {
		t.Fatalf("quit bypassed pending changes: err=%v, popup=%v", err, s.Popup)
	}
}
