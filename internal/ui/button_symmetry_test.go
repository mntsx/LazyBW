package ui

import (
	"testing"
	"unicode/utf8"
)

// assertSymmetricButton checks the label and the accent use identical bounds.
func assertSymmetricButton(t *testing.T, canvas *Canvas, x, y, width int, label string) {
	t.Helper()
	labelWidth := utf8.RuneCountInString(label)
	if leftSpace := (width - labelWidth) / 2; leftSpace != width-labelWidth-leftSpace {
		t.Fatalf("%q has unequal padding in width %d", label, width)
	}
	start := x + (width-labelWidth)/2
	for offset, character := range []rune(label) {
		if cell := canvasAt(canvas, start+offset, y); cell.R != character {
			t.Fatalf("%q at x=%d: got %q, want %q", label, start+offset, cell.R, character)
		}
	}
	for offset := 0; offset < width; offset++ {
		if cell := canvasAt(canvas, x+offset, y+1); cell.R != '▁' {
			t.Fatalf("%q missing underline at x=%d: %+v", label, x+offset, cell)
		}
	}
	if cell := canvasAt(canvas, x+width, y+1); cell.R == '▁' {
		t.Fatalf("%q extends past its right edge at x=%d", label, x+width)
	}
}

// TestSymmetricButtonWidths checks the shared right-edge correction.
func TestSymmetricButtonWidths(t *testing.T) {
	for _, testCase := range []struct {
		label string
		base  int
		want  int
	}{
		{"CANCEL", 13, 12},
		{"YES", 14, 13},
		{"NO", 13, 12},
		{"SUBMIT", 16, 16},
		{"CANCEL", 16, 16},
		{"DISCARD AND QUIT", 20, 20},
		{"AUTH", 12, 12},
		{"QUIT", 12, 12},
	} {
		if got := symmetricButtonWidth(testCase.label, testCase.base); got != testCase.want {
			t.Errorf("%q (%d): width = %d, want %d", testCase.label, testCase.base, got, testCase.want)
		}
	}
}

// TestEveryPopupButtonHasSymmetricPadding covers both standard and quit dialogs.
func TestEveryPopupButtonHasSymmetricPadding(t *testing.T) {
	for _, testCase := range []struct {
		quit        bool
		destructive bool
		yesLabel    string
		noLabel     string
	}{
		{false, false, "YES", "NO"},
		{false, true, "YES", "NO"},
		{true, true, "DISCARD AND QUIT", "CANCEL"},
	} {
		for _, selectedYes := range []bool{true, false} {
			state := setup(t)
			state.Popup = &Popup{
				Title: "Confirm", Question: "Continue?", Quit: testCase.quit,
				Destructive: testCase.destructive, Yes: selectedYes,
			}
			app := New(state)
			canvas := NewCanvas(110, 35)
			app.popupRender(canvas)
			buttonY := app.dialogY + app.dialogH - 4
			yesX := app.dialogX + 4
			yesW := symmetricButtonWidth(testCase.yesLabel,
				max(14, utf8.RuneCountInString(testCase.yesLabel)+4))
			noX := app.dialogX + app.dialogW -
				max(17, utf8.RuneCountInString(testCase.noLabel)+6)
			noW := symmetricButtonWidth(testCase.noLabel,
				max(13, utf8.RuneCountInString(testCase.noLabel)+4))
			assertSymmetricButton(t, canvas, yesX, buttonY, yesW, testCase.yesLabel)
			assertSymmetricButton(t, canvas, noX, buttonY, noW, testCase.noLabel)
		}
	}
}

// TestCreationButtonsHaveSymmetricPadding checks every form submit/cancel pair.
func TestCreationButtonsHaveSymmetricPadding(t *testing.T) {
	for _, mode := range []Mode{DialogName, DialogFolder, DialogField} {
		state := setup(t)
		state.Screen = Workspace
		state.Mode = mode
		app := New(state)
		canvas := NewCanvas(110, 35)
		app.createRender(canvas)
		width := min(65, canvas.W-6)
		x := (canvas.W - width) / 2
		y := max(2, (canvas.H-16)/2) + 10
		assertSymmetricButton(t, canvas, x+9, y, 16, "SUBMIT")
		assertSymmetricButton(t, canvas, x+width-26, y, 16, "CANCEL")
	}
}

// TestAuthenticationButtonsRemainSymmetric verifies compact AUTH/QUIT labels.
func TestAuthenticationButtonsRemainSymmetric(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		state := setup(t)
		state.Screen = screen
		state.Busy = false
		app := New(state)
		canvas := NewCanvas(110, 35)
		app.authRender(canvas)
		layout := authGeometry(canvas.W, canvas.H, screen)
		for _, button := range []struct {
			x     int
			label string
		}{{layout.enterX, "AUTH"}, {layout.quitX, "QUIT"}} {
			if (layout.buttonWidth-utf8.RuneCountInString(button.label))%2 != 0 {
				t.Fatalf("%q not symmetric", button.label)
			}
			start := button.x + (layout.buttonWidth-len(button.label))/2
			for index, char := range button.label {
				if got := canvasAt(canvas, start+index, layout.buttonY).R; got != char {
					t.Fatalf("%q: letter %d = %q", button.label, index, got)
				}
			}
		}
	}
}
