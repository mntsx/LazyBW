package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

func newFieldDialogState(t *testing.T) (*App, *State) {
	t.Helper()
	s := setup(t)
	s.OpenItem()
	a := New(s)
	if err := a.char('n'); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogFieldType || s.CustomType != 0 || s.Focus != PaneContent {
		t.Fatalf("expected first phase: mode=%v, type=%v, focus=%v", s.Mode, s.CustomType, s.Focus)
	}
	return a, s
}

func TestCreateFieldTwoPhases(t *testing.T) {
	expected := []struct {
		label, icon string
		bwType      int
		initial     string
	}{
		{"Text", "\U000F0284", 0, ""},
		{"Hidden", "\U000F07F5", 1, ""},
		{"Checkbox", "\U000F0135", 2, "false"},
		{"Linked", "\U000F0339", 3, ""},
	}
	if len(customFieldOptions) != len(expected) {
		t.Fatalf("expected %d field types, got %d", len(expected), len(customFieldOptions))
	}
	for index, want := range expected {
		t.Run(want.label, func(t *testing.T) {
			a, s := newFieldDialogState(t)
			for j := 0; j < index; j++ {
				if err := a.arrow(1, 0); err != nil {
					t.Fatal(err)
				}
			}
			if s.CustomType != index {
				t.Fatalf("wrong selected index %d", s.CustomType)
			}
			if customFieldOptions[index].Icon != want.icon || customFieldOptions[index].Name != want.label {
				t.Fatal("wrong label or icon")
			}
			if err := a.enter(); err != nil {
				t.Fatal(err)
			}
			if s.Mode != DialogField || s.FormFocus != 0 {
				t.Fatal("did not open name entry")
			}
			for _, r := range "Sample " + want.label {
				if err := a.char(r); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.enter(); err != nil {
				t.Fatal(err)
			}
			if want.bwType == 3 {
				if s.Mode != DialogLinkedChoice {
					t.Fatal("linked field must request a target")
				}
				if err := a.enter(); err != nil {
					t.Fatal(err)
				}
			}
			if s.Mode != Browse || s.Stage.Len() != 1 {
				t.Fatal("not staged after creation")
			}
			arr := s.SelectedItem().Raw["fields"].([]any)
			if len(arr) != 1 {
				t.Fatalf("expected one field, got %d", len(arr))
			}
			field := arr[0].(map[string]any)
			if field["type"] != want.bwType || (want.bwType != 3 && field["value"] != want.initial) {
				t.Fatalf("wrong wire representation: %#v", field)
			}
			if field["name"] != "Sample "+want.label {
				t.Fatalf("wrong field name: %#v", field)
			}
			var got *vault.Field
			for i := range s.Fields {
				if s.Fields[i].Name == "Sample "+want.label {
					got = &s.Fields[i]
					break
				}
			}
			if got == nil || !got.Custom || got.Hidden != (index == 1) {
				t.Fatalf("wrong field state: %#v", got)
			}
			if last := s.Fields[len(s.Fields)-1]; !last.Folders {
				t.Fatalf("Folders must remain the final field: %#v", last)
			}
			if s.Stage.Changes()[0].Kind != vault.Modified {
				t.Fatal("existing item must be modified")
			}
		})
	}
}

func TestCreateFieldCancelAndEscape(t *testing.T) {
	a, s := newFieldDialogState(t)
	for i := 0; i < len(customFieldOptions)+2; i++ {
		_ = a.arrow(1, 0)
	}
	if s.CustomType != len(fieldOptions(s.SelectedItem())) {
		t.Fatal("did not reach cancel")
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Browse || s.Stage.Len() != 0 {
		t.Fatal("cancel staged a change")
	}
	a, s = newFieldDialogState(t)
	_ = a.enter()
	if s.Mode != DialogField {
		t.Fatal("second phase expected")
	}
	_ = a.escape()
	if s.Mode != Browse || s.Stage.Len() != 0 {
		t.Fatal("escape staged a change")
	}
	a, s = newFieldDialogState(t)
	_ = a.char('d') // no delete-field while the modal list is open
	if s.Mode != DialogFieldType || s.Popup != nil {
		t.Fatal("modal leaked keyboard shortcut")
	}
}

func TestCreateFieldPopupRendering(t *testing.T) {
	a, _ := newFieldDialogState(t)
	c := NewCanvas(100, 40)
	a.createRender(c)
	output := c.Render()
	for _, option := range customFieldOptions {
		if !strings.Contains(output, option.Name) || !strings.Contains(output, option.Icon) {
			t.Fatalf("missing option %s", option.Name)
		}
	}
	x := (c.W - 65) / 2
	y := (c.H - 16) / 2
	if c.Cells[(y+3)*c.W+x+4].BG != CBlack {
		t.Fatal("first choice is not highlighted")
	}
	for i := 0; i < len(fieldOptions(a.S.SelectedItem())); i++ {
		_ = a.arrow(1, 0)
	}
	c = NewCanvas(100, 40)
	a.createRender(c)
	if c.Cells[(y+3+len(fieldOptions(a.S.SelectedItem())))*c.W+x+4].BG != CRed {
		t.Fatal("cancel row must have red background when selected")
	}
	_ = a.arrow(-1, 0) // Website: edits a URI rather than a custom field.
	_ = a.arrow(-1, 0) // Checkbox: opens the regular name input.
	_ = a.enter()
	c = NewCanvas(100, 40)
	a.createRender(c)
	if !strings.Contains(c.Render(), "ield name") || a.S.Mode != DialogField {
		t.Fatal("missing second-phase name input")
	}
}

func TestCreateFieldNameValidationAndCancelButton(t *testing.T) {
	a, s := newFieldDialogState(t)
	_ = a.enter() // Text -> name entry
	_ = a.enter() // Blank name must not create a field
	if s.Stage.Len() != 0 || len(s.SelectedItem().Raw["fields"].([]any)) != 0 {
		t.Fatal("blank field name staged a field")
	}
	a, s = newFieldDialogState(t)
	_ = a.enter()
	_ = a.arrow(1, 0) // textbox -> ENTER
	_ = a.arrow(0, 1) // ENTER -> CANCEL
	if s.FormFocus != 2 {
		t.Fatal("cancel button must be reachable")
	}
	_ = a.enter()
	if s.Mode != Browse || s.Stage.Len() != 0 {
		t.Fatal("cancel button staged a change")
	}
}
