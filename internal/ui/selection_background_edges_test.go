package ui

import "testing"

// The half-height background must not have a one-column notch beside the
// selection marker. Both the marker column and the next cell use the exact
// same background fragment at each boundary.
func TestSelectionBackgroundHasFlushHalfHeightEdges(t *testing.T) {
	s := setup(t)
	s.Focus = PaneItems
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	for _, tc := range []struct {
		y     int
		glyph rune
	}{
		{3, '▄'}, {6, '▀'},
	} {
		for x := a.itemX + selectionStripeWidth; x < a.itemX+8; x++ {
			cell := canvasAt(c, x, tc.y)
			if cell.R != tc.glyph || cell.FG != CBlack || cell.BG != CDefault {
				t.Fatalf("half-height background has a notch at (%d,%d): %+v", x, tc.y, cell)
			}
		}
	}
	for _, tc := range []struct {
		y     int
		glyph rune
	}{{3, '▄'}, {6, '▀'}} {
		stripe := canvasAt(c, a.itemX, tc.y)
		if !balancedStripeCell(stripe, tc.glyph, CCyan) {
			t.Fatalf("stripe half-height end at y=%d: %+v", tc.y, stripe)
		}
	}
	for _, y := range []int{4, 5} {
		stripe := canvasAt(c, a.itemX, y)
		if !balancedStripeCell(stripe, selectionStripeCenterMask, CCyan) {
			t.Fatalf("normal selection stripe lost at y=%d: %+v", y, stripe)
		}
	}
}

// Item content uses the same half-height leading/trailing stripe geometry as
// Items; changing focus should not shorten the green active-state stripe.
func TestContentSelectionStripeExtendsToHalfRowEdges(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.Focus = PaneContent
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	for _, tc := range []struct {
		y     int
		glyph rune
	}{{3, '▄'}, {4, selectionStripeCenterMask}, {5, selectionStripeCenterMask}, {6, '▀'}} {
		stripe := canvasAt(c, a.contentX, tc.y)
		if !balancedStripeCell(stripe, tc.glyph, CCyan) {
			t.Fatalf("content stripe clipped at y=%d: %+v", tc.y, stripe)
		}
	}
	// The selected background must still cover exactly half the first/last
	// row over its *whole* width to the right of the stripe.
	for _, tc := range []struct {
		y     int
		glyph rune
	}{{3, '▄'}, {6, '▀'}} {
		for xx := a.contentX + 1; xx < a.contentX+8; xx++ {
			cell := canvasAt(c, xx, tc.y)
			if cell.R != tc.glyph || cell.FG != CBlack || cell.BG != CDefault {
				t.Fatalf("content background at (%d,%d): %+v", xx, tc.y, cell)
			}
		}
	}
}
