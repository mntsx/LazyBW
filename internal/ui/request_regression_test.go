package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

func TestWordEditingAcrossInputModes(t *testing.T) {
	s := setup(t)
	a := New(s)
	s.Screen = Workspace
	s.Mode = Find
	s.Input, s.Search, s.Caret = "uno dos-tres", "uno dos-tres", len([]rune("uno dos-tres"))
	if err := a.wordEdit(-1, false); err != nil {
		t.Fatal(err)
	}
	if s.Caret != 8 {
		t.Fatalf("expected caret at tres: %d", s.Caret)
	}
	if err := a.wordEdit(-1, true); err != nil {
		t.Fatal(err)
	}
	if s.Input != "uno tres" || s.Search != s.Input {
		t.Fatalf("backward delete: input=%q search=%q", s.Input, s.Search)
	}
	s.Input, s.Search, s.Caret = "área claro", "área claro", 0
	_ = a.wordEdit(1, false)
	if s.Caret != 5 {
		t.Fatalf("next word should begin at column 5, got %d", s.Caret)
	}
	s.Caret = 0
	_ = a.wordEdit(1, true)
	if s.Input != "claro" {
		t.Fatalf("forward deletion should remove first word: %q", s.Input)
	}
	s.Input, s.Search, s.Caret = "área claro", "área claro", 5
	_ = a.wordEdit(-1, false)
	if s.Caret != 0 {
		t.Fatalf("unicode word boundary: %d", s.Caret)
	}
	_ = a.wordEdit(1, true)
	if s.Input != "claro" {
		t.Fatalf("forward delete: %q", s.Input)
	}

	s.Screen = AuthMaster
	s.AuthFocus = 0
	s.Master = "pass word"
	s.Caret = 9
	_ = a.wordEdit(-1, true)
	if s.Master != "pass " || s.Caret != 5 {
		t.Fatalf("auth delete: %q @%d", s.Master, s.Caret)
	}
}

func TestFindPlaceholderAndMagentaRule(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.Mode = Find
	s.Focus = PaneItems
	s.Blink = true
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	searchY := 1 + (44 - 4) - 2
	if got := canvasAt(c, a.itemX+2, searchY+1); got.R != 'F' {
		t.Fatalf("cursor overwrote initial F: %#v", got)
	}
	if got := canvasAt(c, a.itemX+5, searchY); got.FG != CMagenta {
		t.Fatalf("find rule not magenta: %#v", got)
	}
	s.Search = "Ex"
	s.Input = "Ex"
	s.Caret = 2
	c = NewCanvas(120, 44)
	a.mainRender(c)
	// setup() adds Example, which fuzzy-matches "Ex" at its first two positions.
	if cell := canvasAt(c, a.itemX+8, 4); cell.FG != CMagenta {
		t.Fatalf("matched character not magenta: %#v", cell)
	}
}

func TestItemHeadersStateAndCounterAlignment(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.Focus = PaneItems
	a := New(s)
	c := NewCanvas(140, 45)
	a.mainRender(c)
	header, row := stringCellRow(c, 1), stringCellRow(c, 4)
	ownerAt := strings.Index(header, "Owner")

	stateAt := strings.Index(header, "State")
	if ownerAt < 0 || stateAt <= ownerAt {
		t.Fatalf("missing headings: %q", header)
	}
	if absInt((2*utf8.RuneCountInString(row[:strings.Index(row, "Me")])+2)-(2*utf8.RuneCountInString(header[:ownerAt])+5)) > 1 {
		t.Fatalf("owner not centered: heading=%d value=%d", ownerAt, strings.Index(row, "Me"))
	}
	if absInt((2*utf8.RuneCountInString(row[:strings.Index(row, "NA")])+2)-(2*utf8.RuneCountInString(header[:stateAt])+5)) > 1 {
		t.Fatalf("state not centered: heading=%d value=%d", stateAt, strings.Index(row, "NA"))
	}
	if !strings.Contains(row, "1/1") {
		t.Fatal("counter is not on first row")
	}
	if strings.Contains(stringCellRow(c, 5), "1/1") {
		t.Fatal("counter still on subtitle row")
	}

	item := s.CurrentItem()
	item.Raw["name"] = "Example"
	s.Stage.Record("item", item.Raw, vault.TypeNames[item.Type()], s.Originals)
	// Stage a real change to show modified state.
	if err := item.Set([]any{"login", "username"}, "updated"); err != nil {
		t.Fatal(err)
	}
	s.Stage.Record("item", item.Raw, vault.TypeNames[item.Type()], s.Originals)
	c = NewCanvas(140, 45)
	a.mainRender(c)
	if got := canvasAt(c, a.itemX+a.itemW-16, 4); got.R == 'M' && got.FG != CYellow {
		t.Fatalf("wrong modified color: %#v", got)
	}
	s.OpenItem()
	c = NewCanvas(140, 45)
	a.mainRender(c)
	if !strings.Contains(stringCellRow(c, 1), "Ow") || !strings.Contains(stringCellRow(c, 1), "St") {
		t.Fatal("headers not abbreviated when content opens")
	}
}

func TestPopupAndCreateDialogHaveFourSides(t *testing.T) {
	s := setup(t)
	a := New(s)
	c := NewCanvas(120, 40)
	s.Popup = &Popup{Title: "Confirm", Question: "Save changes?", Yes: true}
	a.popupRender(c)
	for _, pos := range [][2]int{{a.dialogX, a.dialogY}, {a.dialogX + a.dialogW - 1, a.dialogY},
		{a.dialogX, a.dialogY + a.dialogH - 1}, {a.dialogX + a.dialogW - 1, a.dialogY + a.dialogH - 1}} {
		if cell := canvasAt(c, pos[0], pos[1]); cell.FG != CBlue {
			t.Fatalf("corner missing: %#v", cell)
		}
	}
	if got := canvasAt(c, a.dialogX, a.dialogY+4); got.R != '┃' {
		t.Fatalf("missing side: %#v", got)
	}
	s.Popup = nil
	s.Mode = DialogCreate
	c = NewCanvas(120, 40)
	a.createRender(c)
	x, y := (120-65)/2, (40-16)/2
	if got := canvasAt(c, x, y+3); got.R != '┃' {
		t.Fatalf("creation border missing: %#v", got)
	}
}

func TestFooterStatusUsesYellowVersionAndGreenSpinner(t *testing.T) {
	s := setup(t)
	s.Version = "2026.9.1"
	a := New(s)
	c := NewCanvas(120, 44)
	a.footer(c, 1, 42, 118)
	row := stringCellRow(c, 42)
	vIndex := strings.Index(row, "2026.9.1")
	if vIndex < 0 || canvasAt(c, utf8.RuneCountInString(row[:vIndex]), 42).FG != CYellow {
		t.Fatalf("missing yellow version: %q", row)
	}
	s.Busy = true
	s.BusyMessage = "Saving changes"
	c = NewCanvas(120, 44)
	a.footer(c, 1, 42, 118)
	row = stringCellRow(c, 42)
	i := strings.Index(row, "Saving changes")
	if i < 0 || canvasAt(c, utf8.RuneCountInString(row[:i]), 42).FG != CGreen {
		t.Fatalf("spinner message not green: %q", row)
	}
}

func absInt(i int) int {
	if i < 0 {
		return -i
	}
	return i
}
