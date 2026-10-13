package ui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestChangesCounterToTheRightOfKind(t *testing.T) {
	s := setup(t)
	s.Stage.Record("item", map[string]any{"id": "local-new", "name": "First", "type": 1}, "Login", s.Originals)
	it := s.CurrentItem()
	if err := it.Set([]any{"login", "username"}, "changed"); err != nil {
		t.Fatal(err)
	}
	s.Stage.Record("item", it.Raw, "Login", s.Originals)
	s.Focus = PaneChanges
	s.ChangeIndex = 0
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	y := a.sideY[3] + 2
	line := stringCellRow(c, y)
	kx := strings.Index(line, "N")
	cx := strings.Index(line, "1/2")
	if kx < 0 || cx < kx+3 {
		t.Fatalf("kind and counter overlap: %q", line)
	}
	if cell := canvasAt(c, utf8.RuneCountInString(line[:kx]), y); cell.FG != CGreen {
		t.Fatalf("N should be green: %#v", cell)
	}
	y++
	line = stringCellRow(c, y)
	kx = strings.Index(line, "M")
	if kx < 0 || canvasAt(c, utf8.RuneCountInString(line[:kx]), y).FG != CYellow {
		t.Fatalf("M should be yellow: %q", line)
	}
}

func TestPopupKindColorsAndFullWidthButtons(t *testing.T) {
	s := setup(t)
	s.Popup = &Popup{Title: "Changes", Question: "Save?", Yes: true, Lines: []string{"\uf007 New thing  N", "\uf007 Existing thing  M", "\uf007 Deleted thing  D"}}
	a := New(s)
	c := NewCanvas(120, 44)
	a.popupRender(c)
	for idx, expected := range []struct {
		kind  rune
		color int
	}{{'N', CGreen}, {'M', CYellow}, {'D', CRed}} {
		y := a.dialogY + 5 + idx
		line := stringCellRow(c, y)
		k := strings.LastIndex(line, string(expected.kind))
		if k < 0 || canvasAt(c, utf8.RuneCountInString(line[:k]), y).FG != expected.color {
			t.Fatalf("marker at %d not colored: %s", idx, line)
		}
	}
	// Selected YES: green background covers the two-row button.
	for _, y := range []int{a.dialogY + a.dialogH - 4, a.dialogY + a.dialogH - 3} {
		if got := canvasAt(c, a.dialogX+5, y).BG; got != CGreen {
			t.Fatalf("selected button background broken at %d: %d", y, got)
		}
	}
}

func TestButtonStatesSolidAndUnderline(t *testing.T) {
	a := New(setup(t))
	for _, tc := range []struct {
		sel, hover bool
		text, bg   int
		lower      rune
	}{
		{false, false, CGreen, CBlack, '▁'},
		{true, false, CBlack, CGreen, '▁'},
		{false, true, CWhite, CDefault, '▁'},
		{true, true, CWhite, CDefault, '▁'},
	} {
		c := NewCanvas(40, 10)
		a.button(c, 3, 3, 12, "YES", CGreen, tc.sel, tc.hover)
		if got := canvasAt(c, 4, 3); got.BG != tc.bg || got.FG != tc.text {
			t.Fatalf("button top: %+v", got)
		}
		if got := canvasAt(c, 4, 4); got.R != tc.lower {
			t.Fatalf("button underline/filled row: %+v", got)
		}
		accent := CGreen
		if tc.hover {
			accent = CWhite
		}
		if got := canvasAt(c, 4, 4); got.FG != accent || got.BG != tc.bg {
			t.Fatalf("baseline must be colored and flush with button: %+v", got)
		}
		if got := canvasAt(c, 4, 5); got.BG != CDefault {
			t.Fatalf("button paints below its baseline: %+v", got)
		}
	}
}

func TestFindBlinkAndSubtitleForeground(t *testing.T) {
	s := setup(t)
	s.Mode = Find
	s.Focus = PaneItems
	a := New(s)
	for _, blink := range []bool{false, true} {
		s.Blink = blink
		c := NewCanvas(120, 44)
		a.mainRender(c)
		if got := canvasAt(c, a.itemX+2, 1); got.FG != CWhite {
			t.Fatalf("find panel number should be white: %+v", got)
		}
		at := canvasAt(c, a.itemX+5, 44-5)
		expectedColor := CMagenta
		if at.R != '━' || at.FG != expectedColor {
			t.Fatalf("find rule blink %v: %+v", blink, at)
		}
		if got := canvasAt(c, a.itemX+8, 5); got.FG != CSubtitle {
			t.Fatalf("subtitle should stay dim white: %+v", got)
		}
	}
}

func TestItemClickOutsideRenderedRows(t *testing.T) {
	for _, tc := range []struct {
		x, y, rows int
		valid      bool
	}{
		{50, 3, 1, true}, {50, 4, 1, true}, {50, 5, 1, true}, {50, 6, 1, false},
		{50, 2, 1, false}, {50, 7, 1, false}, {50, 42, 1, false},
		{50, 39, 99, false}, {10, 3, 99, false}, {110, 3, 99, false},
	} {
		i, ok := itemHitIndex(tc.x, tc.y, 40, 60, 44, 0, tc.rows)
		if ok != tc.valid {
			t.Fatalf("click %v: index %d valid %v", tc, i, ok)
		}
	}
}

func TestDetailCaretPreservesOriginalText(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 0
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	s.Input = "Hola"
	a := New(s)
	for _, caret := range []int{0, 1, 2, 4} {
		s.Caret = caret
		c := NewCanvas(90, 25)
		a.detailPanel(c, 10, 5, 36, 12)
		row := stringCellRow(c, 8)
		if got := []rune(row)[12:16]; string(got) != "Hola" {
			t.Fatalf("caret %d changed the text: %q", caret, row)
		}
		for x := 12; x <= 16; x++ {
			if cell := canvasAt(c, x, 8); cell.R == '▏' || cell.BG == CBlue {
				t.Fatalf("caret is painted into text cell: %+v", cell)
			}
		}
	}
}

func TestFooterDotSmallGreen(t *testing.T) {
	s := setup(t)
	s.Version = "1.2.3"
	c := NewCanvas(120, 44)
	New(s).footer(c, 1, 42, 118)
	row := stringCellRow(c, 42)
	offset := strings.Index(row, "• Bitwarden CLI ")
	if offset < 0 {
		t.Fatalf("status missing: %s", row)
	}
	idx := utf8.RuneCountInString(row[:offset])
	if got := canvasAt(c, idx, 42); got.FG != CGreen {
		t.Fatalf("dot not green: %+v", got)
	}
}
