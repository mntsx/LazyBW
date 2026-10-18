package ui

import "testing"

// TestAllButtonsHoverUsesWhite checks the shared hover palette regardless of hue.
func TestAllButtonsHoverUsesWhite(t *testing.T) {
	app := New(setup(t))
	for _, color := range []int{CGreen, CRed, CBlue, CYellow, CMagenta} {
		for _, selected := range []bool{false, true} {
			canvas := NewCanvas(28, 12)
			app.button(canvas, 3, 4, 12, "TEST", color, selected, true)
			for x := 3; x < 15; x++ {
				body := canvasAt(canvas, x, 4)
				accent := canvasAt(canvas, x, 5)
				if body.FG != CWhite || body.BG != CDefault ||
					accent.R != buttonAccentGlyph || accent.FG != CWhite ||
					accent.BG != CDefault {
					t.Fatalf("color=%d selected=%t: body=%+v accent=%+v",
						color, selected, body, accent)
				}
			}
		}
	}
}

// TestAuthButtonHoverClearsTopQuarter checks the upper glyph also uses the base.
func TestAuthButtonHoverClearsTopQuarter(t *testing.T) {
	app := New(setup(t))
	for _, color := range []int{CGreen, CRed, CBlue} {
		for _, selected := range []bool{false, true} {
			canvas := NewCanvas(28, 12)
			app.authButton(canvas, 3, 5, 12, "AUTH", color, selected, true)
			for x := 3; x < 15; x++ {
				top := canvasAt(canvas, x, 4)
				body := canvasAt(canvas, x, 5)
				accent := canvasAt(canvas, x, 6)
				if top.R != ' ' || top.FG != CDefault || top.BG != CDefault ||
					body.FG != CWhite || body.BG != CDefault ||
					accent.R != buttonAccentGlyph || accent.FG != CWhite ||
					accent.BG != CDefault {
					t.Fatalf("color=%d selected=%t: top=%+v body=%+v accent=%+v",
						color, selected, top, body, accent)
				}
			}
		}
	}
}

// TestKeyboardSubmitTargetsOnlyTextBoxesWithButtons checks flash eligibility.
func TestKeyboardSubmitTargetsOnlyTextBoxesWithButtons(t *testing.T) {
	state := setup(t)
	app := New(state)
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		state.Screen = screen
		state.Popup = nil
		state.Busy = false
		for _, focus := range []int{0, 1, 2, 3} {
			state.AuthFocus = focus
			want := submitFlashNone
			if focus < 2 {
				want = submitFlashAuth
			}
			if got := app.keyboardSubmitTarget(); got != want {
				t.Fatalf("screen=%v auth focus=%d: got %v, want %v",
					screen, focus, got, want)
			}
		}
	}
	state.Screen = Workspace
	for _, mode := range []Mode{DialogName, DialogField, DialogFolder, Find, Edit, Browse} {
		state.Mode = mode
		for _, focus := range []int{0, 1, 2} {
			state.FormFocus = focus
			want := submitFlashNone
			if focus == 0 && (mode == DialogName || mode == DialogField ||
				mode == DialogFolder) {
				want = submitFlashDialog
			}
			if got := app.keyboardSubmitTarget(); got != want {
				t.Fatalf("mode=%v form focus=%d: got %v, want %v",
					mode, focus, got, want)
			}
		}
	}
	state.Mode = DialogName
	state.FormFocus = 0
	state.Busy = true
	if got := app.keyboardSubmitTarget(); got != submitFlashNone {
		t.Fatalf("busy form should not flash: %v", got)
	}
}

// TestKeyboardFlashRendersSelectedButton tests the flash overlay on both forms.
func TestKeyboardFlashRendersSelectedButton(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		state := setup(t)
		state.Screen = screen
		state.AuthFocus = 0
		state.Busy = false
		app := New(state)
		app.submitFlash = submitFlashAuth
		canvas := NewCanvas(100, 35)
		app.authRender(canvas)
		layout := authGeometry(canvas.W, canvas.H, screen)
		body := canvasAt(canvas, layout.enterX, layout.buttonY)
		accent := canvasAt(canvas, layout.enterX, layout.buttonY+1)
		if body.BG != CGreen || accent.FG != CGreen {
			t.Fatalf("screen=%v auth flash missing: body=%+v accent=%+v",
				screen, body, accent)
		}
		if state.AuthFocus != 0 || state.Busy {
			t.Fatal("flash must not trigger authentication or change textbox focus")
		}
	}

	state := setup(t)
	state.Screen = Workspace
	state.Mode = DialogName
	state.FormFocus = 0
	app := New(state)
	app.submitFlash = submitFlashDialog
	canvas := NewCanvas(100, 35)
	app.createRender(canvas)
	y := max(2, (canvas.H-16)/2) + 10
	x := (canvas.W-min(65, canvas.W-6))/2 + 9
	if body := canvasAt(canvas, x, y); body.BG != CBlue {
		t.Fatalf("dialog flash missing: %+v", body)
	}
	if state.Mode != DialogName || state.FormFocus != 0 {
		t.Fatal("dialog flash must leave the input focused until the action runs")
	}
}
