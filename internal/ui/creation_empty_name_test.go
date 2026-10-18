package ui

import (
	"strings"
	"testing"
)

var emptyNameCases = []struct {
	name string
	mode Mode
}{
	{name: "item", mode: DialogName},
	{name: "field", mode: DialogField},
	{name: "folder", mode: DialogFolder},
}

func assertEmptyNameRejected(t *testing.T, s *State, mode Mode) {
	t.Helper()
	if s.Mode != mode {
		t.Fatalf("empty input closed mode %v as %v", mode, s.Mode)
	}
	if s.FormError != creationNameError(mode) {
		t.Fatalf("error = %q, want %q", s.FormError, creationNameError(mode))
	}
	if s.Stage.Len() != 0 {
		t.Fatal("empty input staged an entity")
	}
}

func TestEmptyCreationNamesKeepFormOpenFromKeyboard(t *testing.T) {
	for _, testCase := range emptyNameCases {
		for _, focus := range []int{0, 1} {
			t.Run(testCase.name, func(t *testing.T) {
				s := setup(t)
				defer s.CLI.Cipher.Close()
				s.Mode = testCase.mode
				s.FormFocus = focus
				s.Input = "  \t  "
				s.CustomType = 0
				a := New(s)

				if err := a.enter(); err != nil {
					t.Fatal(err)
				}
				assertEmptyNameRejected(t, s, testCase.mode)
				if s.Input != "  \t  " {
					t.Fatalf("empty input was changed: %q", s.Input)
				}
			})
		}
	}
}

func TestCreationNameErrorRestoresArrowNavigation(t *testing.T) {
	for _, testCase := range emptyNameCases {
		t.Run(testCase.name, func(t *testing.T) {
			s := setup(t)
			defer s.CLI.Cipher.Close()
			s.Mode = testCase.mode
			s.FormFocus = 1
			a := New(s)

			if err := a.enter(); err != nil {
				t.Fatal(err)
			}
			assertEmptyNameRejected(t, s, testCase.mode)
			if handled, err := a.dispatchConfiguredKey("up"); err != nil || !handled {
				t.Fatalf("up binding: handled=%t err=%v", handled, err)
			}
			if s.FormFocus != 0 {
				t.Fatalf("up did not return to textbox: focus=%d", s.FormFocus)
			}
			if err := a.char('x'); err != nil {
				t.Fatal(err)
			}
			if s.Input != "x" || s.FormError != "" {
				t.Fatalf("textbox did not resume editing: input=%q error=%q", s.Input, s.FormError)
			}
		})
	}
}

func TestCreationTextboxIsLockedWhileValidationIsPending(t *testing.T) {
	for _, busy := range []bool{false, true} {
		s := setup(t)
		s.Mode = DialogName
		s.FormFocus = 0
		s.Input = "name"
		s.Caret = len(s.Input)
		s.Busy = busy
		a := New(s)
		if !busy {
			a.submitFlash = submitFlashDialog
		}

		if err := a.char('x'); err != nil {
			t.Fatal(err)
		}
		if err := a.backspace(); err != nil {
			t.Fatal(err)
		}
		if err := a.deleteForward(); err != nil {
			t.Fatal(err)
		}
		if err := a.wordEdit(-1, true); err != nil {
			t.Fatal(err)
		}
		if err := a.arrow(1, 0); err != nil {
			t.Fatal(err)
		}
		if s.Input != "name" || s.Caret != len(s.Input) || s.FormFocus != 0 {
			t.Fatalf("pending validation changed form: input=%q caret=%d focus=%d",
				s.Input, s.Caret, s.FormFocus)
		}
		s.CLI.Cipher.Close()
	}
}

func TestEmptyCreationNamesKeepFormOpenFromMouse(t *testing.T) {
	for _, testCase := range emptyNameCases {
		t.Run(testCase.name, func(t *testing.T) {
			s := setup(t)
			defer s.CLI.Cipher.Close()
			s.Mode = testCase.mode
			s.Input = ""
			s.CustomType = 0
			a := New(s)
			canvas := NewCanvas(100, 35)
			a.mainRender(canvas)

			width := min(65, canvas.W-6)
			x := (canvas.W-width)/2 + 9
			y := max(2, (canvas.H-16)/2) + 10
			if err := a.clickAt(x, y); err != nil {
				t.Fatal(err)
			}
			assertEmptyNameRejected(t, s, testCase.mode)

			dialog := NewCanvas(100, 35)
			a.createRender(dialog)
			message := creationNameError(testCase.mode)
			if !strings.Contains(dialog.Render(), message) {
				t.Fatalf("validation message is not visible: %q", dialog.Render())
			}
			dialogX := (dialog.W - width) / 2
			dialogY := max(2, (dialog.H-16)/2)
			messageX := dialogX + (width-len([]rune(message)))/2
			for offset, character := range []rune(message) {
				cell := canvasAt(dialog, messageX+offset, dialogY+7)
				if cell.R != character || cell.FG != CRed {
					t.Fatalf("message is not centered one row above: x=%d cell=%#v", messageX+offset, cell)
				}
			}
		})
	}
}
