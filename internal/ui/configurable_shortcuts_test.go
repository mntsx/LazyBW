package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

func TestConfiguredHintUsesFirstKeyOfList(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.Focus = PaneContent
	s.Config.Keybinds = map[string]map[string][]string{
		"content": {"switch": {"v", "s", "right", "enter"}},
	}
	index := -1
	item := s.SelectedItem()
	item.Raw["fields"] = []any{map[string]any{
		"name": "Toggle", "type": 2, "value": "false",
	}}
	s.Originals[item.ID()] = vault.Clone(item.Raw)
	s.OpenItem()
	for i, f := range s.Fields {
		if f.Name == "Toggle" {
			index = i
		}
	}
	if index < 0 {
		t.Fatal("missing checkbox")
	}
	s.FieldIndex = index
	hints := strings.Join(New(s).hints(), " ")
	if !strings.Contains(hints, "v switch") || strings.Contains(hints, "right switch") {
		t.Fatalf("first configured key not shown: %q", hints)
	}
}

func TestCheckboxCanToggleWithEnterRightAndS(t *testing.T) {
	s := setup(t)
	item := s.Items[0]
	item.Raw["fields"] = []any{map[string]any{"name": "Toggle", "type": 2, "value": "false"}}
	s.Originals[item.ID()] = vault.Clone(item.Raw)
	s.OpenItem()
	for i, f := range s.Fields {
		if f.Name == "Toggle" {
			s.FieldIndex = i
		}
	}
	a := New(s)
	for _, fn := range []func() error{a.enter, func() error { return a.arrow(0, 1) }, func() error { return a.char('s') }} {
		if err := fn(); err != nil {
			t.Fatal(err)
		}
	}
	if s.CurrentField().Value != "true" || s.Stage.Len() != 1 || s.Mode != Browse {
		t.Fatalf("expected third toggle to restore true: %#v", s.CurrentField())
	}
}

func TestThemeAllowsFullPaletteAndTrueColor(t *testing.T) {
	canvas := NewCanvas(3, 1)
	canvas.Theme(map[string]string{"red": "#ab1290", "bright_green": "ansi256:118", "hint_key": "ansi256:227"})
	canvas.Put(0, 0, 'r', CRed, CDefault)
	canvas.Put(1, 0, 'g', CBGreen, CDefault)
	canvas.Put(2, 0, 'h', CHintKey, CDefault)
	if canvas.Cells[0].FG != (1<<24)|0xab1290 || canvas.Cells[1].FG != 118 ||
		canvas.Cells[2].FG != 227 {
		t.Fatal("custom palette colors were not applied")
	}
}
