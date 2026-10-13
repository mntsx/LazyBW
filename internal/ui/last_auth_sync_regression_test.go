package ui

import (
	"strings"
	"testing"

	"github.com/awesome-gocui/gocui"

	"github.com/miguelmnts/lazybw-go/internal/config"
)

// TestRepromptToggleDoesNotInterruptOpenContent checks live checkbox changes.
func TestRepromptToggleDoesNotInterruptOpenContent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		present bool
		initial int
	}{
		{name: "missing"},
		{name: "off", present: true, initial: 0},
		{name: "on", present: true, initial: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setup(t)
			defer s.CLI.Cipher.Close()
			item := s.Items[0]
			if tc.present {
				item.Raw["reprompt"] = tc.initial
				s.Originals[item.ID()]["reprompt"] = tc.initial
			}
			s.OpenItem()
			if tc.initial == 1 {
				// Simulate the item having been opened after successful verification.
				s.RepromptApprovedID = item.ID()
				s.OpenItem()
			}
			if s.Focus != PaneContent {
				t.Fatal("item not opened")
			}
			for i, field := range s.Fields {
				if field.Reprompt {
					s.FieldIndex = i
					break
				}
			}
			if err := s.SwitchCheckbox(); err != nil {
				t.Fatal(err)
			}
			app := New(s)
			if err := app.layout(&gocui.Gui{}); err != nil {
				t.Fatal(err)
			}
			if s.Mode == DialogReprompt || s.Focus != PaneContent {
				t.Fatal("toggling reprompt interrupted the open item")
			}
			if s.Stage.Len() != 1 {
				t.Fatal("reprompt change was not staged")
			}
			if err := s.SwitchCheckbox(); err != nil {
				t.Fatal(err)
			}
			if s.Stage.Len() != 0 || s.Stage.FieldKind(item.ID(),
				[]any{"reprompt"}, s.CLI.Cipher) != "" {
				t.Fatal("restoring reprompt left a modified marker")
			}
			s.CloseItem()
			if tc.initial == 1 {
				app.openItem()
				if s.Mode != DialogReprompt || s.Focus == PaneContent {
					t.Fatal("protected item reopened without requesting password")
				}
			}
		})
	}
}

// TestOfflineHidesSynchronizationShortcut checks hints and event feedback.
func TestOfflineHidesSynchronizationShortcut(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.ServerConnected = false
	a := New(s)
	if strings.Contains(strings.Join(a.hints(), " "), "sync") ||
		a.keyAction("ctrl+s") != "unbound" {
		t.Fatal("synchronization shortcut advertised while offline")
	}
	s.ServerConnected = true
	if !strings.Contains(strings.Join(a.hints(), " "), "sync") ||
		a.keyAction("ctrl+s") != "synchronize" {
		t.Fatal("synchronization did not return after connection recovery")
	}
}

// TestMasterScreenSuggestsLocalReplication verifies the disabled feature hint.
func TestMasterScreenSuggestsLocalReplication(t *testing.T) {
	s := setup(t)
	s.Screen = AuthMaster
	s.Config.Local = config.Local{SaveVault: false}
	a := New(s)
	canvas := NewCanvas(120, 44)
	a.authRender(canvas)
	if !strings.Contains(canvas.Render(), "Configure local replication") {
		t.Fatal("missing local replication hint")
	}
	s.Config.Local.SaveVault = true
	canvas = NewCanvas(120, 44)
	a.authRender(canvas)
	if strings.Contains(canvas.Render(), "Configure local replication") {
		t.Fatal("local replication hint appeared when enabled")
	}
}

// TestBlockedRegistrationExplainsTheFailingStep checks safe error presentation.
func TestBlockedRegistrationExplainsTheFailingStep(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.Items[0].Raw["name"] = "Local staged item"
	s.Stage.Record("item", s.Items[0].Raw, "Login", s.Originals)
	a := New(s)
	a.registrationBlocked("bw sync failed (network, exit 1)")
	if s.Popup == nil || !s.Popup.Acknowledge ||
		!strings.Contains(s.Popup.Question, "bw sync") {
		t.Fatal("registration failure lacks persistent diagnostic popup")
	}
	if s.Stage.Len() != 1 || s.Busy {
		t.Fatal("blocked registration lost pending changes or stayed busy")
	}
}
