package ui

import "testing"

// TestSelectionEndCapsMatchFullWidthStripe verifies full-width halves and centre,
// without bold modifiers or changes to the side-panel markers.
func TestSelectionEndCapsMatchFullWidthStripe(t *testing.T) {
	s := setup(t)
	s.Focus = PaneItems
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	for _, test := range []struct {
		y    int
		want rune
		bold bool
	}{
		{3, '▄', false},
		{4, selectionStripeCenterMask, false},
		{5, selectionStripeCenterMask, false},
		{6, '▀', false},
	} {
		got := canvasAt(c, a.itemX, test.y)
		if got.R != test.want || got.Bold != test.bold || got.FG != CCyan {
			t.Fatalf("row %d: unexpected stripe cell: %+v", test.y, got)
		}
	}
	s.OpenItem()
	s.Focus = PaneContent
	c = NewCanvas(120, 44)
	a.mainRender(c)
	for _, y := range []int{3, 6} {
		got := canvasAt(c, a.contentX, y)
		if got.Bold || got.FG != CCyan {
			t.Fatalf("content endpoint at %d: %+v", y, got)
		}
	}
}

// TestSidebarStripeRemainsHalfWidth protects other panel indicators.
func TestSidebarStripeRemainsHalfWidth(t *testing.T) {
	s := setup(t)
	s.Focus = PaneVault
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	got := canvasAt(c, 1+selectionStripeInset, a.sideY[0]+2)
	if got.R != '▐' || got.Bold || got.FG != CCyan {
		t.Fatalf("sidebar stripe was modified: %+v", got)
	}
}
