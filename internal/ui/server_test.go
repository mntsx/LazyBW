package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/config"
)

func TestServerSelectorAndAuthScreen(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		s := setup(t)
		s.Screen, s.Mode, s.AuthFocus = screen, Browse, 0
		s.Config.Authentication.DefaultServer = "eu"
		s.Server = "eu"
		a := New(s)
		c := NewCanvas(100, 32)
		a.authRender(c)
		row := stringCellRow(c, 1)
		if !strings.Contains(row, "EU (bitwarden.eu)") || strings.Contains(row, "server:") {
			t.Fatalf("no server indicator: %q", row)
		}
		if !strings.Contains(stringCellRow(c, authGeometry(100, 32, screen).footerY), "ctrl+s server") {
			t.Fatal("server shortcut not advertised")
		}
		s.Mode = DialogServer
		s.ServerReady = [2]bool{true, true}
		s.ServerChoice = 1
		if _, _, visible := a.editCaretPoint(100, 32); visible {
			t.Fatal("auth textbox cursor visible over selector")
		}
		oldID := s.AuthID
		if err := a.char('s'); err != nil || s.AuthID != oldID {
			t.Fatal("characters must not enter auth textbox through selector")
		}
		a.serverPickerRender(c)
		_, dy, _, _ := serverPickerGeometry(100, 32)
		if !strings.Contains(stringCellRow(c, dy+3), "United States") ||
			!strings.Contains(stringCellRow(c, dy+4), "European Union") {
			t.Fatal("popup missing region options")
		}
		if strings.Contains(stringCellRow(c, dy+5), "Cancel") {
			t.Fatal("selector has a Cancel option")
		}
		if err := a.escape(); err != nil || s.Mode != Browse || s.Server != "eu" {
			t.Fatal("escape should cancel selection")
		}
		s.Mode = DialogServer
		_ = a.arrow(-1, 0)
		if s.ServerChoice != 0 {
			t.Fatal("up should select US")
		}
		if err := a.enter(); err != nil || s.Server != "us" || s.Screen != AuthChecking {
			t.Fatalf("server selection not applied: %s %v", s.Server, err)
		}
	}
}

func TestDefaultServerIsNotChangedBySelection(t *testing.T) {
	cfg := config.Config{Authentication: config.Authentication{DefaultServer: "eu"}}
	s := NewState(cfg, nil)
	a := New(s)
	s.Screen = AuthMaster
	s.Mode = DialogServer
	s.ServerReady = [2]bool{true, true}
	s.ServerChoice = 0
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Config.Authentication.DefaultServer != "eu" || s.Server != "us" {
		t.Fatal("selected server must not change saved default")
	}
}
