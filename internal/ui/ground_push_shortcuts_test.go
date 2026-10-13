package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

func TestGroundItemMarkerDetectsBaselineChange(t *testing.T) {
	s := setup(t)
	s.InitialGrounds = map[string]map[string]any{
		"test": {"name": "old", "favorite": false},
	}
	s.Originals = map[string]map[string]any{
		"test": {"name": "new", "favorite": false},
	}
	if !s.GroundItemChanged("test") {
		t.Fatal("modified ground must produce an item marker")
	}
	s.InitialGrounds["test"]["name"] = "new"
	if s.GroundItemChanged("test") {
		t.Fatal("unchanged ground must not produce an item marker")
	}
}

func TestSyncPushHints(t *testing.T) {
	s := setup(t)
	s.ServerConnected = true
	s.Screen = Workspace
	s.CreateItem(vault.Login, "check")
	a := New(s)
	hints := strings.Join(a.hints(), " ")
	if !strings.Contains(hints, "ctrl+s sync") ||
		!strings.Contains(hints, "ctrl+p push all") ||
		strings.Contains(hints, "ctrl+u") {
		t.Fatalf("incorrect sync or push hint: %q", hints)
	}
	if got := a.keyAction("ctrl+s"); got != "synchronize" {
		t.Fatalf("ctrl+s did not trigger sync: %q", got)
	}
	if got := a.keyAction("ctrl+p"); got != "push" {
		t.Fatalf("ctrl+p did not trigger push: %q", got)
	}
}
