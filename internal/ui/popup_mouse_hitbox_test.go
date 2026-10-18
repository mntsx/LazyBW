package ui

import "testing"

func TestCreationNameDialogsIgnoreBlankButtonRowAreas(t *testing.T) {
	for _, mode := range []Mode{DialogName, DialogField, DialogFolder} {
		for _, area := range []string{"left margin", "center gap", "right margin"} {
			t.Run(area, func(t *testing.T) {
				s := setup(t)
				defer s.CLI.Cipher.Close()
				s.Mode = mode
				s.Input = "Valid name"
				s.CustomType = 0
				a := New(s)
				canvas := NewCanvas(100, 35)
				a.mainRender(canvas)

				width := min(65, canvas.W-6)
				dialogX := (canvas.W - width) / 2
				buttonY := max(2, (canvas.H-16)/2) + 10
				x := dialogX + 1
				switch area {
				case "center gap":
					x = dialogX + width/2
				case "right margin":
					x = dialogX + width - 2
				}
				if err := a.clickAt(x, buttonY); err != nil {
					t.Fatal(err)
				}
				if s.Mode != mode || s.Input != "Valid name" || s.Stage.Len() != 0 {
					t.Fatalf("blank click changed dialog: mode=%v input=%q staged=%d", s.Mode, s.Input, s.Stage.Len())
				}
			})
		}
	}
}

func TestConfirmationPopupsIgnoreBlankButtonRowAreas(t *testing.T) {
	for _, quit := range []bool{false, true} {
		s := setup(t)
		defer s.CLI.Cipher.Close()
		called := false
		popup := &Popup{
			Title: "Confirm", Question: "Continue?", Quit: quit,
			Action: func(bool) { called = true },
		}
		s.Popup = popup
		a := New(s)
		a.popupRender(NewCanvas(100, 35))
		buttonY := a.dialogY + a.dialogH - 4

		if err := a.clickAt(a.dialogX+a.dialogW/2, buttonY); err != nil {
			t.Fatal(err)
		}
		if s.Popup != popup || called {
			t.Fatal("clicking the gap between popup buttons closed the popup")
		}
	}
}

func TestSelectionPopupsIgnoreBlankHorizontalAreas(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.Mode = DialogCreate
	a := New(s)
	canvas := NewCanvas(100, 35)
	a.mainRender(canvas)

	width := min(65, canvas.W-6)
	dialogX := (canvas.W - width) / 2
	rowY := max(2, (canvas.H-16)/2) + 3
	if err := a.clickAt(dialogX+1, rowY); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogCreate || s.Stage.Len() != 0 {
		t.Fatal("clicking beside an option activated it")
	}
}
