package ui

import "testing"

func TestAllVaultsIsSelectedAndAppliedByDefault(t *testing.T) {
	s := setup(t)
	if s.Filters[0] != 1 || s.FilterVault != 1 {
		t.Fatalf("expected All vaults selected and applied by default; selection=%d, applied=%d", s.Filters[0], s.FilterVault)
	}
	s.Focus = PaneVault
	a := New(s)
	canvas := NewCanvas(120, 44)
	a.mainRender(canvas)
	selected := canvasAt(canvas, 1+selectionStripeInset, a.sideY[0]+2)
	if selected.R != selectionStripeGlyph || selected.FG != CCyan {
		t.Fatalf("All vaults selection marker missing: %#v", selected)
	}
}
