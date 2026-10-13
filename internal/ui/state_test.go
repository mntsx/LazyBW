package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

func setup(t *testing.T) *State {
	t.Helper()
	c, err := bitwarden.NewClient(nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewState(config.Config{Colors: config.DefaultColors}, c)
	it := vault.NewItem(vault.Login, "Example", "id-1", c.Cipher)
	_ = it.Set([]any{"login", "password"}, "secret")
	it.Raw["login"].(map[string]any)["username"] = "alice"
	s.Loaded([]*vault.Item{it}, nil)
	return s
}
func TestOpenEditStageDiscard(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	if s.Focus != PaneContent || len(s.Fields) < 2 {
		t.Fatal("open")
	}
	s.FieldIndex = 0
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	s.Insert('!')
	if err := s.CommitEdit(); err != nil {
		t.Fatal(err)
	}
	if s.Stage.Len() != 1 || s.FieldStatus(s.Fields[0]) != vault.Modified || s.FieldStatus(s.Fields[1]) != "" {
		t.Fatal("wrong staging field markers")
	}
	s.Restore(s.Stage.Changes()[0])
	if s.Stage.Len() != 0 || s.Items[0].Raw["login"].(map[string]any)["username"] != "alice" {
		t.Fatal("discard")
	}
}
func TestHiddenFieldNotKeptInPreview(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 1
	f := s.Fields[1]
	if !f.Hidden || vault.Mask(f.Value) != "••••••" {
		t.Fatal("missing mask")
	}
	s.Reveal()
	if !s.Revealed[s.RevealKey()] {
		t.Fatal("reveal only selected")
	}
	s.FieldIndex = 0
	if s.Revealed[s.RevealKey()] {
		t.Fatal("other field revealed")
	}
}
func TestFuzzyFiltersOnlyNames(t *testing.T) {
	s := setup(t)
	s.Search = "alice"
	if len(s.Filtered()) != 0 {
		t.Fatal("matched username rather than name")
	}
	s.Search = "exm"
	if len(s.Filtered()) != 1 {
		t.Fatal("name fuzzy matching")
	}
}
func TestCreateSortedAndIcon(t *testing.T) {
	s := setup(t)
	s.CreateItem(vault.Card, "aaa")
	if s.Items[0].Name() != "aaa" || vault.TypeIcons[s.Items[0].Type()] == "" {
		t.Fatal("creation not sorted/icon missing")
	}
	if s.Stage.Len() != 1 || s.Stage.Changes()[0].Kind != vault.New {
		t.Fatal("staging new")
	}
}
func TestEditCursorBoundary(t *testing.T) {
	s := setup(t)
	s.Input = "abc"
	s.Caret = 0
	s.Cursor(-1)
	if s.Caret != 0 {
		t.Fatal("cursor went below zero")
	}
	s.Insert('Z')
	if s.Input != "Zabc" || s.Caret != 1 {
		t.Fatal("did not insert before first char")
	}
	s.Caret = 3
	s.Backspace()
	if s.Input != "Zac" {
		t.Fatal(s.Input)
	}
	s.Caret = 3
	s.DeleteForward()
	if s.Input != "Zac" {
		t.Fatal("EOF delete")
	}
}
func TestStageSnapshotNotPlainSecret(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 1
	_ = s.BeginEdit()
	s.Input = "other-secret"
	_ = s.CommitEdit()
	c := s.Stage.Changes()[0]
	if strings.Contains(c.After["login"].(map[string]any)["password"].(*vault.Sealed).String(), "other-secret") {
		t.Fatal("plaintext leaked")
	}
}
func TestQuitConfirmationDefaultsCancel(t *testing.T) {
	s := setup(t)
	s.CreateItem(1, "new")
	a := New(s)
	a.requestQuit()
	if s.Popup == nil || s.Popup.Yes || !s.Popup.Quit {
		t.Fatal("quit confirmation did not default to Cancel")
	}
}
func TestChangePopupDefaults(t *testing.T) {
	s := setup(t)
	s.CreateItem(1, "new")
	a := New(s)
	a.confirmStage(false, true)
	if s.Popup == nil || s.Popup.Yes || !s.Popup.Destructive {
		t.Fatal("discard should default No")
	}
	s.Popup = nil
	a.confirmStage(true, true)
	if s.Popup == nil || !s.Popup.Yes {
		t.Fatal("save should default Yes")
	}
}
func TestCanvasColorsAndStyle(t *testing.T) {
	c := NewCanvas(16, 2)
	c.Theme(map[string]string{"title_background": "#112233"})
	c.Put(0, 0, 'x', CWhite, CTitle)
	s := c.Render()
	if !strings.Contains(s, "48;2;17;34;51m") {
		t.Fatal("truecolor title background not rendered")
	}
}
func TestPanelMarkers(t *testing.T) {
	s := setup(t)
	s.CreateItem(1, "new")
	a := New(s)
	c := NewCanvas(124, 38)
	a.mainRender(c)
	output := c.Render()
	if !strings.Contains(output, "Edits") || !strings.Contains(output, "Name") || !strings.Contains(output, "Owner") {
		t.Fatal("missing panels")
	}
}
func TestHintsWithin75Percent(t *testing.T) {
	hints := []string{"ctrl+q quit", "ctrl+p push", "ctrl+d discard", "e edit", "f find", "r reveal", "n new", "d delete", "c copy", "? ..."}
	pages := PaginateHints(hints, 44)
	if len(pages) < 2 {
		t.Fatal("expected multiple hint pages")
	}
	if pages[0][len(pages[0])-1] != "? ..." {
		t.Fatal("page-one navigation hint")
	}
	for j, page := range pages {
		width := 0
		for _, hint := range page {
			width += len([]rune(hint)) + 3
		}
		if width > 44 {
			t.Fatalf("page %d exceeds width: %d", j, width)
		}
		if j > 0 && page[0] != "? ..." {
			t.Fatal("next page must start with navigation hint")
		}
	}
}
