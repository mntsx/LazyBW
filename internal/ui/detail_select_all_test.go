package ui

import (
	"strings"
	"testing"
)

func TestDetailSelectAllUsesConfiguredKeyWithoutFooterHint(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 0
	if err := s.BeginTextEdit(); err != nil {
		t.Fatal(err)
	}
	s.Input = "uno dos"
	s.Caret = 3

	defaultApp := New(s)
	if handled, err := defaultApp.dispatchConfiguredKey("ctrl+a"); err != nil || !handled {
		t.Fatalf("default select-all key: handled=%t err=%v", handled, err)
	}
	start, end, selected := s.selection()
	if !selected || start != 0 || end != len([]rune(s.Input)) {
		t.Fatalf("default select all = (%d, %d, %t), want entire input", start, end, selected)
	}

	s.Caret = 3
	s.SelectionAnchor = -1
	s.Config.Keybinds = map[string]map[string][]string{
		"detail": {"select_all": {"ctrl+g"}},
	}
	a := New(s)

	if handled, err := a.dispatchConfiguredKey("ctrl+a"); err != nil || handled {
		t.Fatalf("default select-all key ignored configured replacement: handled=%t err=%v", handled, err)
	}
	if handled, err := a.dispatchConfiguredKey("ctrl+g"); err != nil || !handled {
		t.Fatalf("configured select-all key: handled=%t err=%v", handled, err)
	}
	start, end, selected = s.selection()
	if !selected || start != 0 || end != len([]rune(s.Input)) {
		t.Fatalf("select all = (%d, %d, %t), want entire input", start, end, selected)
	}
	if strings.Contains(strings.Join(a.hints(), " "), "select all") {
		t.Fatal("select-all must not be displayed in the detail footer")
	}
}

func TestDetailCtrlNPagesHintsWithoutChangingInput(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 0
	if err := s.BeginTextEdit(); err != nil {
		t.Fatal(err)
	}
	s.Input = "unchanged"
	a := New(s)

	if handled, err := a.dispatchConfiguredKey("ctrl+n"); err != nil || !handled {
		t.Fatalf("configured next-page key: handled=%t err=%v", handled, err)
	}
	if s.Input != "unchanged" {
		t.Fatalf("ctrl+n changed editor input to %q", s.Input)
	}
}

func TestDetailRevealUsesConfiguredBinding(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 1
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	before := s.Input
	a := New(s)

	if handled, err := a.dispatchConfiguredKey("ctrl+r"); err != nil || !handled {
		t.Fatalf("configured detail reveal: handled=%t err=%v", handled, err)
	}
	if !s.AuthVisible || s.Input != before {
		t.Fatalf("detail reveal changed input or did not reveal: visible=%t input=%q", s.AuthVisible, s.Input)
	}
}

func TestContentNewFieldHintUsesConfiguredBinding(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.Focus = PaneContent
	s.Config.Keybinds = map[string]map[string][]string{
		"content": {"new_field": {"x"}},
	}

	hints := strings.Join(New(s).hints(), " ")
	if !strings.Contains(hints, "x new") || strings.Contains(hints, "n new") {
		t.Fatalf("content new-field hint ignored configured key: %q", hints)
	}
}

func TestDetailEditingKeysUseConfiguredBindings(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 0
	if err := s.BeginTextEdit(); err != nil {
		t.Fatal(err)
	}
	s.Input = "abc"
	s.Caret = 2
	s.Config.Keybinds = map[string]map[string][]string{
		"detail": {"cursor_left": {"h"}},
	}
	a := New(s)

	if handled, err := a.dispatchConfiguredKey("left"); err != nil || handled {
		t.Fatalf("former cursor key ignored configured replacement: handled=%t err=%v", handled, err)
	}
	if handled, err := a.dispatchConfiguredKey("h"); err != nil || !handled {
		t.Fatalf("configured cursor key: handled=%t err=%v", handled, err)
	}
	if s.Caret != 1 {
		t.Fatalf("configured cursor key moved to %d, want 1", s.Caret)
	}
}
