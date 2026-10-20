package ui

import (
	"strings"
	"testing"
	"time"
)

// TestFindUpArrowReturnsToItems keeps the typed filter when leaving Find.
func TestFindUpArrowReturnsToItems(t *testing.T) {
	s := setup(t)
	s.Mode, s.Focus = Find, PaneItems
	s.Input, s.Search, s.Caret = "Exam", "Exam", 4
	a := New(s)
	if err := a.arrow(-1, 0); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Browse || s.Focus != PaneItems || s.Search != "Exam" {
		t.Fatalf("up from Find lost Items/filter: %#v", s)
	}
}

// TestTextEditorsNeverShowKeyEcho suppresses both typed and action keys.
func TestTextEditorsNeverShowKeyEcho(t *testing.T) {
	for _, mode := range []Mode{Find, Edit, DialogName} {
		s := setup(t)
		a := New(s)
		s.Mode = mode
		if mode == Edit {
			s.OpenItem()
			if err := s.BeginEdit(); err != nil {
				t.Fatal(err)
			}
		}
		a.reportKey("z", "unbound")
		if err := a.handleKey("x", func() error { return a.char('x') }); err != nil {
			t.Fatal(err)
		}
		if !s.KeyFeedbackExpires.IsZero() {
			t.Fatalf("text entry mode %v kept echo active", mode)
		}
	}
	s := setup(t)
	a := New(s)
	s.Screen = AuthMaster
	s.AuthFocus = 0
	a.reportKey("z", "unbound")
	if err := a.handleKey("a", func() error { return a.char('a') }); err != nil {
		t.Fatal(err)
	}
	if !s.KeyFeedbackExpires.IsZero() {
		t.Fatal("password input showed key feedback")
	}
	s.Screen, s.Mode = Workspace, Browse
	a.reportKey("z", "unbound")
	if s.KeyFeedbackExpires.Before(time.Now()) {
		t.Fatal("ordinary shortcuts lost their feedback")
	}
}

// TestWordEditsAndDeleteForwardUseActiveTextbox covers all three editors.
func TestWordEditsAndDeleteForwardUseActiveTextbox(t *testing.T) {
	for _, mode := range []Mode{Find, Edit, DialogName} {
		s := setup(t)
		a := New(s)
		s.Mode = mode
		s.Input = "first second"
		s.Caret = 5
		if mode == Find {
			s.Search = s.Input
		}
		if err := a.wordEdit(1, false); err != nil {
			t.Fatal(err)
		}
		if s.Caret != 6 {
			t.Fatalf("mode %v next word landed at %d", mode, s.Caret)
		}
		if err := a.wordEdit(1, true); err != nil {
			t.Fatal(err)
		}
		if s.Input != "first " {
			t.Fatalf("mode %v forward word delete = %q", mode, s.Input)
		}
		if mode == Find && s.Search != "first " {
			t.Fatal("Find results did not track word deletion")
		}
		s.Caret = 5
		if err := a.deleteForward(); err != nil {
			t.Fatal(err)
		}
		if s.Input != "first" {
			t.Fatalf("mode %v forward delete = %q", mode, s.Input)
		}
	}
	s := setup(t)
	s.Screen = AuthMaster
	s.AuthFocus = 0
	s.Master = "alpha beta"
	s.Caret = 0
	a := New(s)
	if err := a.wordEdit(1, false); err != nil || s.Caret != 6 {
		t.Fatalf("auth next word: %v @%d", err, s.Caret)
	}
	if err := a.wordEdit(-1, true); err != nil || s.Master != "beta" {
		t.Fatalf("auth delete previous word: %v %q", err, s.Master)
	}
	s.Caret = 0
	if err := a.deleteForward(); err != nil || s.Master != "eta" {
		t.Fatalf("auth forward delete: %v %q", err, s.Master)
	}
}

// TestBoldAndDimOutputPreservesAttributes validates gocui's ANSI SGR order.
func TestMultilineEditorCursorAndViewport(t *testing.T) {
	s := setup(t)
	s.Input = "short\n0123456789\nthird"
	s.Caret = len([]rune("short\n012345"))
	s.CursorLine(-1)
	if s.Caret != 5 {
		t.Fatalf("up moved caret to %d, want 5", s.Caret)
	}
	s.CursorLine(1)
	if s.Caret != len([]rune("short\n01234")) {
		t.Fatalf("down moved caret to %d", s.Caret)
	}
	s.CursorLineBoundary(true)
	if s.Caret != len([]rune("short\n0123456789")) {
		t.Fatalf("end of paragraph = %d", s.Caret)
	}
	s.CursorLineBoundary(false)
	if s.Caret != len([]rune("short\n")) {
		t.Fatalf("start of paragraph = %d", s.Caret)
	}

	rows, line, col := detailDisplayLines(s.Input, len([]rune("short\n012345678")), 4, false)
	if line != 1 || col != 3 || rows[0] != "" || rows[1] != "6789" || rows[2] != "" {
		t.Fatalf("unwrapped viewport = %#v @ %d,%d", rows, line, col)
	}
	rows, line, col = detailDisplayLines(s.Input, len([]rune("short\n012345678")), 4, true)
	if line != 4 || col != 1 || rows[0] != "shor" || rows[1] != "t" || rows[2] != "0123" || rows[3] != "4567" {
		t.Fatalf("wrapped viewport = %#v @ %d,%d", rows, line, col)
	}
}

func TestEditorSelectionHistoryAndBlankLineNavigation(t *testing.T) {
	s := setup(t)
	s.Mode, s.Input, s.Caret, s.SelectionAnchor = Edit, "one\n\ntwo", 0, -1
	s.CursorSelect(3)
	if start, end, ok := s.selection(); !ok || start != 0 || end != 3 {
		t.Fatalf("character selection = %d:%d, %v", start, end, ok)
	}
	s.InsertText("ONE")
	if s.Input != "ONE\n\ntwo" { t.Fatalf("selection replacement = %q", s.Input) }
	s.UndoEdit()
	if s.Input != "one\n\ntwo" || s.Caret != 3 || s.SelectionAnchor != 0 {
		t.Fatalf("undo did not restore selection: %q @%d/%d", s.Input, s.Caret, s.SelectionAnchor)
	}
	s.RedoEdit()
	if s.Input != "ONE\n\ntwo" { t.Fatalf("redo = %q", s.Input) }
	s.Caret, s.SelectionAnchor = 0, -1
	s.CursorBlankLine(1, true)
	if s.Caret != 4 || s.SelectionAnchor != 0 {
		t.Fatalf("next blank line = %d/%d", s.Caret, s.SelectionAnchor)
	}
	s.CursorBlankLine(1, true)
	if s.Caret != len([]rune(s.Input)) { t.Fatalf("missing blank line must go to end: %d", s.Caret) }
}

func TestDetailWrapHintAndDefault(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	a := New(s)
	if s.InputWrap || !strings.Contains(strings.Join(a.hints(), " "), "ctrl+w wrap") {
		t.Fatalf("detail editor must default to unwrapped: wrap=%v hints=%v", s.InputWrap, a.hints())
	}
	s.InputWrap = true
	if !strings.Contains(strings.Join(a.hints(), " "), "ctrl+w unwrap") {
		t.Fatalf("wrapped detail editor must advertise unwrap: %v", a.hints())
	}
}

func TestBoldAndDimOutputPreservesAttributes(t *testing.T) {
	c := NewCanvas(20, 1)
	c.TextBold(0, 0, "Title", CWhite, CBlack)
	c.Text(6, 0, "plain", CWhite, CBlack)
	c.TextDim(12, 0, "value", CWhite, CBlack)
	ansi := c.Render()
	if !strings.Contains(ansi, "\x1b[37m\x1b[40m\x1b[1mTitle") {
		t.Fatalf("bold emitted before colors: %q", ansi)
	}
	if !strings.Contains(ansi, "\x1b[37m\x1b[40m\x1b[2mvalue") {
		t.Fatalf("dim emitted before colors: %q", ansi)
	}
	if !c.Cells[0].Bold || c.Cells[6].Bold || c.Cells[12].Bold || !c.Cells[12].Dim {
		t.Fatal("font weights leaked from labels to values")
	}
}

// TestFieldValuesAreDimWhite checks both plain text and grouped websites.
func TestFieldValuesAreDimWhite(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.Focus = PaneContent
	a := New(s)
	c := NewCanvas(125, 44)
	a.mainRender(c)
	label := canvasAt(c, a.contentX+3, 4)
	value := canvasAt(c, a.contentX+3, 5)
	if !label.Bold || label.Dim || value.Bold || !value.Dim || value.FG != CWhite {
		t.Fatalf("label/value styles: %+v %+v", label, value)
	}
	addTestWebsites(s)
	s.loadFields()
	s.FieldIndex = 3
	c = NewCanvas(125, 44)
	a.mainRender(c)
	if !strings.Contains(c.Render(), "https://first.example") {
		t.Fatal("URI group omitted")
	}
}

// TestNativeCursorNeverChangesText ensures full/partial visibility on typing.
func TestNativeCursorNeverChangesText(t *testing.T) {
	for _, tc := range []struct {
		text   string
		caret  int
		width  int
		window string
		column int
	}{
		{"Hello", 0, 5, "Hello", 0},
		{"Hello", 3, 5, "Hello", 3},
		{"Hello", 5, 5, "ello", 4},
		{"abcdefgh", 8, 5, "efgh", 4},
		{"mañana", 3, 6, "mañana", 3},
	} {
		got, column := visibleText(tc.text, tc.caret, tc.width)
		if got != tc.window || column != tc.column {
			t.Fatalf("visibleText(%q,%d,%d): %q @%d", tc.text,
				tc.caret, tc.width, got, column)
		}
	}
	s := setup(t)
	a := New(s)
	s.Mode = Find
	s.Search, s.Input = "abcde", "abcde"
	s.Caret = 1
	c := NewCanvas(120, 44)
	a.mainRender(c)
	x, y, visible := a.editCaretPoint(c.W, c.H)
	if !visible || y != c.H-4 || x != a.itemX+3 ||
		canvasAt(c, a.itemX+2, y).R != 'a' {
		t.Fatalf("Find caret altered text: (%d,%d) visible=%v", x, y, visible)
	}
	s.Mode, s.Search, s.Input = Browse, "", ""
	s.OpenItem()
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	s.Input, s.Caret = "Hola", 2
	c = NewCanvas(120, 44)
	a.mainRender(c)
	x, y, visible = a.editCaretPoint(c.W, c.H)
	if !visible || x != a.detailX+4 || y != a.detailY+3 ||
		canvasAt(c, a.detailX+2, y).R != 'H' {
		t.Fatalf("Detail caret altered text: (%d,%d) visible=%v", x, y, visible)
	}
}
