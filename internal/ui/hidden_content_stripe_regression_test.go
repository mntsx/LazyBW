package ui

import "testing"

// TestHiddenContentDoesNotRetainGreenItemStripe checks focus changes from an
// opened item to a filter without discarding the remembered item selection.
func TestHiddenContentDoesNotRetainGreenItemStripe(t *testing.T) {
	for _, tc := range []struct {
		name string
		pane Pane
		key  rune
	}{
		{name: "vault", pane: PaneVault, key: '1'},
		{name: "types", pane: PaneTypes, key: '2'},
		{name: "folders", pane: PaneFolders, key: '3'},
		{name: "changes", pane: PaneChanges, key: '4'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setup(t)
			s.OpenItem()
			selectedID := s.SelectedID
			a := New(s)
			before := NewCanvas(120, 44)
			a.mainRender(before)
			if got := canvasAt(before, a.itemX, 3); got.FG != CGreen {
				t.Fatalf("opened item stripe should be green: %+v", got)
			}
			if err := a.char(tc.key); err != nil {
				t.Fatal(err)
			}
			if s.Focus != tc.pane {
				t.Fatalf("focus = %v, want %v", s.Focus, tc.pane)
			}
			if s.SelectedID != selectedID {
				t.Fatal("changing panels must not discard the item selection")
			}
			c := NewCanvas(120, 44)
			a.mainRender(c)
			if a.contentX != 0 {
				t.Fatal("content panel should have been hidden")
			}
			for y := 3; y <= 6; y++ {
				if got := canvasAt(c, a.itemX, y); got.FG == CGreen {
					t.Fatalf("hidden content left green item stripe at y=%d: %+v", y, got)
				}
			}
		})
	}
}

// TestClickingFilterHidesContentStripe checks the mouse route to filter focus.
func TestClickingFilterHidesContentStripe(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	selectedID := s.SelectedID
	a := New(s)
	a.mainRender(NewCanvas(120, 44))
	if err := a.clickAt(5, a.sideY[int(PaneTypes)-1]+2); err != nil {
		t.Fatal(err)
	}
	if s.Focus != PaneTypes {
		t.Fatalf("mouse focus = %v, want Types", s.Focus)
	}
	if s.SelectedID != selectedID {
		t.Fatal("mouse click unexpectedly discarded the item selection")
	}
	c := NewCanvas(120, 44)
	a.mainRender(c)
	if a.contentX != 0 || canvasAt(c, a.itemX, 3).FG == CGreen {
		t.Fatal("clicking filter must hide Content and its green item stripe")
	}
}
