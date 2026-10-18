package ui

import (
	"strings"
	"testing"

	"github.com/awesome-gocui/gocui"
	"github.com/miguelmnts/lazybw-go/internal/config"
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

func TestConfiguredKeyReplacesBuiltInCharacter(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.Busy = false
	s.Focus = PaneItems
	s.Config.Keybinds = map[string]map[string][]string{
		"items": {"search": {"x"}},
	}
	a := New(s)

	a.editEvent(nil, 0, 'f', gocui.ModNone)
	if s.Mode != Browse {
		t.Fatal("the former built-in search key ignored the configured replacement")
	}
	if s.KeyFeedbackAction != "unbound" {
		t.Fatalf("removed key feedback = %q; want unbound", s.KeyFeedbackAction)
	}
	a.editEvent(nil, 0, 'x', gocui.ModNone)
	if s.Mode != Find {
		t.Fatal("the configured search key did not open Find")
	}
	if s.KeyFeedbackAction != "find" {
		t.Fatalf("configured search feedback = %q; want find", s.KeyFeedbackAction)
	}
}

func TestEveryConfiguredActionHasFeedbackName(t *testing.T) {
	for section, actions := range config.DefaultKeybinds {
		for action := range actions {
			if got := aliasActionName(section, action); got == "unbound" {
				t.Errorf("keybinds.%s.%s has no feedback name", section, action)
			}
		}
	}
}

func TestBusyHintsUseConfiguredGlobalKeys(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.ServerConnected = true
	s.Busy = true
	s.Config.Keybinds = map[string]map[string][]string{
		"global": {"sync": {"x"}},
	}
	hints := strings.Join(New(s).hints(), " ")
	if !strings.Contains(hints, "x sync") || strings.Contains(hints, "ctrl+s sync") {
		t.Fatalf("busy footer ignored configured sync key: %q", hints)
	}
}

func TestFilterSelectionPrefersLeftAndKeepsEnter(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.Busy = false
	s.Focus = PaneTypes
	s.Filters[1] = 2
	a := New(s)

	if handled, err := a.dispatchConfiguredKey("left"); err != nil || !handled {
		t.Fatalf("left filter binding: handled=%t err=%v", handled, err)
	}
	if s.FilterType != 2 {
		t.Fatal("left did not activate the selected filter")
	}
	if s.KeyFeedbackAction != "select" {
		t.Fatalf("left filter feedback = %q; want select", s.KeyFeedbackAction)
	}
	s.Filters[1] = 3
	if handled, err := a.dispatchConfiguredKey("enter"); err != nil || !handled {
		t.Fatalf("enter filter binding: handled=%t err=%v", handled, err)
	}
	if s.FilterType != 3 {
		t.Fatal("enter no longer activates the selected filter")
	}
	if s.KeyFeedbackAction != "select" {
		t.Fatalf("enter filter feedback = %q; want select", s.KeyFeedbackAction)
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
