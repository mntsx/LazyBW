package ui

import (
	"strings"
	"testing"
)

func TestInputLengthCounterUsesUTF8BytesAndThresholdColor(t *testing.T) {
	text, color, ok := inputLengthCounter("", 1000)
	if !ok || text != "UTF-8 bytes 0/687" || color != CBBlack {
		t.Fatalf("empty counter: text=%q color=%d visible=%t", text, color, ok)
	}

	text, color, ok = inputLengthCounter("hé\n🙂", 1000)
	if !ok || text != "UTF-8 bytes 8/687" || color != CBBlack {
		t.Fatalf("normal Unicode counter: text=%q color=%d visible=%t", text, color, ok)
	}
	_, color, ok = inputLengthCounter(strings.Repeat("a", 688), 1000)
	if !ok || color != CRed {
		t.Fatalf("over-limit counter: color=%d visible=%t", color, ok)
	}
	if _, _, ok := inputLengthCounter("value", 0); ok {
		t.Fatal("unlimited/non-text field unexpectedly received a counter")
	}
}

func TestTextEditorCounterUpdatesForInsertionDeletionAndPaste(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 0 // Login username: 1,000 encrypted characters.
	if err := s.BeginTextEdit(); err != nil {
		t.Fatal(err)
	}
	if got := s.InputEncryptedLimit; got != 1000 {
		t.Fatalf("editor encrypted limit: got %d, want 1000", got)
	}

	s.Input = strings.Repeat("a", 686)
	s.Caret = 686
	s.Insert('a')
	text, color, ok := inputLengthCounter(s.Input, s.InputEncryptedLimit)
	if !ok || text != "UTF-8 bytes 687/687" || color != CBBlack {
		t.Fatalf("boundary insertion: text=%q color=%d visible=%t", text, color, ok)
	}

	s.InsertText("é") // Paste must update immediately and counts two UTF-8 bytes.
	text, color, _ = inputLengthCounter(s.Input, s.InputEncryptedLimit)
	if text != "UTF-8 bytes 689/687" || color != CRed {
		t.Fatalf("pasted Unicode text: text=%q color=%d", text, color)
	}

	s.Backspace()
	text, color, _ = inputLengthCounter(s.Input, s.InputEncryptedLimit)
	if text != "UTF-8 bytes 687/687" || color != CBBlack {
		t.Fatalf("deletion did not immediately restore counter: text=%q color=%d", text, color)
	}
}

func TestCreationTextBoxesUseDeclaredLimits(t *testing.T) {
	s := setup(t)
	a := New(s)
	if err := a.char('n'); err != nil {
		t.Fatal(err)
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogName || s.InputEncryptedLimit != 1000 {
		t.Fatalf("item name limit: mode=%d limit=%d", s.Mode, s.InputEncryptedLimit)
	}

	s.Mode = Browse
	s.Focus = PaneFolders
	if err := a.char('n'); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogFolder || s.InputEncryptedLimit != 1000 {
		t.Fatalf("folder name limit: mode=%d limit=%d", s.Mode, s.InputEncryptedLimit)
	}

	a, s = newFieldDialogState(t)
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogField || s.InputEncryptedLimit != 1000 {
		t.Fatalf("custom field name limit: mode=%d limit=%d", s.Mode, s.InputEncryptedLimit)
	}
}

func TestDetailCounterRendersAtTitleRightAndTracksDynamicFields(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 0
	if err := s.BeginTextEdit(); err != nil {
		t.Fatal(err)
	}
	s.Input = strings.Repeat("a", 688)
	a := New(s)
	canvas := NewCanvas(100, 18)
	a.detailPanel(canvas, 0, 0, 100, 10)
	row := stringCellRow(canvas, 0)
	counter := "UTF-8 bytes 688/687"
	counterX := strings.Index(row, counter)
	if counterX < 0 {
		t.Fatalf("missing title-bar counter in %q", row)
	}
	if cell := canvasAt(canvas, counterX, 0); cell.FG != CRed {
		t.Fatalf("over-limit counter color: got %d, want %d", cell.FG, CRed)
	}

	s.ExitEdit()
	login := s.SelectedItem().Raw["login"].(map[string]any)
	login["uris"] = []any{map[string]any{"uri": "https://example.test"}}
	s.loadFields()
	for index, field := range s.Fields {
		if field.Websites {
			s.FieldIndex = index
			break
		}
	}
	if err := s.BeginEditWebsite(0); err != nil {
		t.Fatal(err)
	}
	if got := s.InputEncryptedLimit; got != 10000 {
		t.Fatalf("website editor encrypted limit: got %d, want 10000", got)
	}

	s.ExitEdit()
	s.SelectedItem().Raw["fields"] = []any{map[string]any{"name": "Custom", "type": 1, "value": ""}}
	s.loadFields()
	for index, field := range s.Fields {
		if field.Name == "Custom" {
			s.FieldIndex = index
			break
		}
	}
	if err := s.BeginTextEdit(); err != nil {
		t.Fatal(err)
	}
	if got := s.InputEncryptedLimit; got != 5000 {
		t.Fatalf("custom field editor encrypted limit: got %d, want 5000", got)
	}
}
