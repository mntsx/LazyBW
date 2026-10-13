package ui

import (
	"testing"
	"time"
)

// TestOfflineConnectionChecksPreserveQuitFocus prevents periodic probes from
// overriding the user's button selection on either authentication screen.
func TestOfflineConnectionChecksPreserveQuitFocus(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		state := setup(t)
		state.Screen = screen
		state.Server = "us"
		state.ServerConnected = true
		app := New(state)
		started := time.Now()
		app.applyConnectionResultAt("us", false, started)
		app.applyConnectionResultAt("us", false,
			started.Add(connectionLossThreshold))
		if state.AuthFocus != 2 {
			t.Fatalf("first outage must select access-local button: %d", state.AuthFocus)
		}
		state.AuthFocus = 3
		app.applyConnectionResultAt("us", false,
			started.Add(2*connectionLossThreshold))
		if state.AuthFocus != 3 {
			t.Fatalf("probe reset QUIT selection on screen %v", screen)
		}
	}
}

// TestAllVaultsDisplayedFirst verifies the displayed order is independent of
// the internal filter encoding.
func TestAllVaultsDisplayedFirst(t *testing.T) {
	state := setup(t)
	state.Focus = PaneVault
	app := New(state)
	canvas := NewCanvas(120, 44)
	app.mainRender(canvas)
	first := canvasAt(canvas, 1+selectionStripeInset, app.sideY[0]+2)
	if first.R != selectionStripeGlyph || first.FG != CCyan {
		t.Fatalf("first vault entry must be selected: %+v", first)
	}
}
