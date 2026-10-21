package ui

import (
	"strings"
	"testing"
)

func TestDetailMaximizePreservesEditorStateAndGeometry(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 0
	if err := s.BeginTextEdit(); err != nil {
		t.Fatal(err)
	}
	s.Input = "abcdef"
	s.Caret = 4
	s.SelectionAnchor = 1
	a := New(s)

	if handled, err := a.dispatchConfiguredKey("ctrl+e"); err != nil || !handled {
		t.Fatalf("maximize dispatch: handled=%t err=%v", handled, err)
	}
	if !s.DetailMaximized || s.Input != "abcdef" || s.Caret != 4 || s.SelectionAnchor != 1 {
		t.Fatalf("maximize changed editor state: %+v", s)
	}
	if !strings.Contains(strings.Join(a.hints(), " "), "ctrl+e enclose") {
		t.Fatalf("maximize hint = %q", a.hints())
	}

	canvas := NewCanvas(100, 40)
	a.mainRender(canvas)
	a.detailPopupRender(canvas)
	if a.detailW != 58 || a.detailH != 5 || a.detailX != 21 || a.detailY != 17 {
		t.Fatalf("maximized geometry = x:%d y:%d w:%d h:%d", a.detailX, a.detailY, a.detailW, a.detailH)
	}
	if canvas.Cells[16*canvas.W+20].R != '┏' || canvas.Cells[22*canvas.W+79].R != '┛' {
		t.Fatal("maximized editor popup outline was not rendered")
	}

	if handled, err := a.dispatchConfiguredKey("ctrl+e"); err != nil || !handled {
		t.Fatalf("enclose dispatch: handled=%t err=%v", handled, err)
	}
	if s.DetailMaximized || s.Input != "abcdef" || s.Caret != 4 || s.SelectionAnchor != 1 {
		t.Fatalf("enclose changed editor state: %+v", s)
	}
	if !strings.Contains(strings.Join(a.hints(), " "), "ctrl+e expand") {
		t.Fatalf("enclose hint = %q", a.hints())
	}
}

func TestDetailEditorDisablesGlobalSyncAndQuit(t *testing.T) {
	for _, maximized := range []bool{false, true} {
		s := setup(t)
		s.OpenItem()
		if err := s.BeginTextEdit(); err != nil {
			t.Fatal(err)
		}
		s.DetailMaximized = maximized
		a := New(s)
		hints := strings.Join(a.hints(), " ")
		for _, action := range []string{"sync", "quit"} {
			for _, key := range s.Config.Keys("global", action) {
				if strings.Contains(hints, key+" ") {
					t.Fatalf("maximized=%t still displays %s keybind: %q", maximized, action, hints)
				}
			}
		}
		for _, action := range []string{"sync", "quit"} {
			for _, key := range s.Config.Keys("global", action) {
				handled, err := a.dispatchConfiguredKey(key)
				if err != nil || handled {
					t.Fatalf("maximized=%t %s (%s): handled=%t err=%v", maximized, action, key, handled, err)
				}
			}
		}
	}
}

func TestMultilineDetailPopupUsesScreenHeight(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	foundNotes := false
	for index, field := range s.Fields {
		if strings.EqualFold(field.Name, "notes") {
			s.FieldIndex = index
			foundNotes = true
			break
		}
	}
	if !foundNotes {
		t.Fatal("test fixture has no multiline notes field")
	}
	if err := s.BeginTextEdit(); err != nil {
		t.Fatal(err)
	}
	s.DetailMaximized = true
	a := New(s)
	canvas := NewCanvas(100, 40)
	a.detailPopupRender(canvas)
	if a.detailW != 58 || a.detailH != 24 || a.detailY != 8 {
		t.Fatalf("multiline geometry = x:%d y:%d w:%d h:%d", a.detailX, a.detailY, a.detailW, a.detailH)
	}
}
