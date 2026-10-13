package ui

import "testing"

// Selection lines use the original position and a single-cell-wide glyph.
// Shared half-height padding spans this column without quarter-cell notches.
func TestSelectionStripeRetainsOriginalPosition(t *testing.T) {
	s := setup(t)
	a := New(s)

	for _, tc := range []struct {
		name     string
		prepare  func()
		location func() (int, int)
		glyph    rune
	}{
		{
			name:     "side filter",
			prepare:  func() { s.Focus = PaneVault },
			location: func() (int, int) { return 1, a.sideY[0] + 2 },
			glyph:    selectionStripeGlyph,
		},
		{
			name:     "item",
			prepare:  func() { s.Focus = PaneItems },
			location: func() (int, int) { return a.itemX, 4 },
			glyph:    selectionStripeCenterMask,
		},
		{
			name:     "content field",
			prepare:  func() { s.OpenItem(); s.Focus = PaneContent },
			location: func() (int, int) { return a.contentX, 4 },
			glyph:    selectionStripeCenterMask,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.prepare()
			canvas := NewCanvas(120, 44)
			a.mainRender(canvas)
			x, y := tc.location()
			if at := canvasAt(canvas, x+selectionStripeInset, y); !balancedStripeCell(at, tc.glyph, CCyan) {
				t.Fatalf("selection marker misplaced: %+v", at)
			}
			if at := canvasAt(canvas, x+selectionStripeInset+selectionStripeWidth, y); at.R == tc.glyph {
				t.Fatalf("multiple parallel selection marks: %+v", at)
			}
		})
	}
}
