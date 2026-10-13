package ui

import (
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// Two-line elements retain their three-cell stride. Each shared spacer cell
// gives its upper half to the preceding element and its lower half to the next.
func TestItemsHaveHalfCellPaddingAboveAndBelow(t *testing.T) {
	s := setup(t)
	s.Focus = PaneItems
	s.Items = append(s.Items, vault.NewItem(vault.Login, "Second", "id-2", s.CLI.Cipher))
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)

	x := a.itemX + 12
	firstTop := canvasAt(c, x, 3)
	if firstTop.R != '▄' || firstTop.FG != CBlack || firstTop.BG != CDefault {
		t.Fatalf("first item's upper half padding: %+v", firstTop)
	}
	between := canvasAt(c, x, 6)
	if between.R != '▀' || between.FG != CBlack || between.BG != CDefault {
		t.Fatalf("first item's lower half padding: %+v", between)
	}
	if got := canvasAt(c, a.itemX+selectionStripeInset, 6); got.R != '▀' || got.FG != CCyan {
		t.Fatalf("selection stripe must finish at the shared boundary: %+v", got)
	}
	if itemRowHeight != 3 {
		t.Fatalf("expected no loss of list capacity: %d", itemRowHeight)
	}
}

func TestContentHasBalancedHalfCellPadding(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.Focus = PaneContent
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)

	x := a.contentX + 12
	top := canvasAt(c, x, 3)
	if top.R != '▄' || top.FG != CBlack || top.BG != CDefault {
		t.Fatalf("field top padding: %+v", top)
	}
	after := canvasAt(c, x, 6)
	if after.R != '▀' || after.FG != CBlack || after.BG != CDefault {
		t.Fatalf("field bottom padding: %+v", after)
	}
}
