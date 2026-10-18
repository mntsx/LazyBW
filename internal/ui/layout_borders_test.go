package ui

import (
	"fmt"
	"github.com/miguelmnts/lazybw-go/internal/vault"
	"strings"
	"testing"
	"unicode/utf8"
)

// The white frame uses bold light box-drawing glyphs for intermediate weight.
// Blue title separators remain unbolded light box-drawing glyphs.
func TestMainLayoutHasContinuousWhiteFrameAndAdjacentPanels(t *testing.T) {
	s := setup(t)
	a := New(s)
	c := NewCanvas(119, 45)
	a.mainRender(c)
	at := func(x, y int) Cell { return c.Cells[y*c.W+x] }

	corners := []struct {
		x, y  int
		glyph rune
	}{
		{0, 0, '┏'}, {c.W - 1, 0, '┓'},
		{0, c.H - 1, '┗'}, {c.W - 1, c.H - 1, '┛'},
	}
	for _, corner := range corners {
		cell := at(corner.x, corner.y)
		if cell.R != corner.glyph || cell.FG != CWhite || !cell.Bold {
			t.Fatalf("incorrect frame corner (%d,%d): %#v", corner.x, corner.y, cell)
		}
	}
	if cell := at(0, 6); cell.R != '┃' || cell.FG != CWhite || !cell.Bold {
		t.Fatalf("outer vertical edge is not heavy white: %#v", cell)
	}
	if cell := at(15, 0); cell.R != '━' || cell.FG != CWhite || !cell.Bold {
		t.Fatalf("outer horizontal edge is not heavy white: %#v", cell)
	}
	sepX := a.leftWidth
	if cell := at(sepX, 0); cell.R != '┳' || cell.FG != CWhite || !cell.Bold {
		t.Fatalf("side/main divider not connected to top border: %#v", cell)
	}
	if cell := at(sepX, c.H-3); cell.R != '┻' || cell.FG != CWhite || !cell.Bold {
		t.Fatalf("side/main divider not connected to footer: %#v", cell)
	}
	for i := 0; i < 3; i++ {
		y := a.sideY[i] + a.sideH[i]
		if cell := at(0, y); cell.R != '┣' || cell.FG != CWhite || !cell.Bold {
			t.Fatalf("side panel %d not connected to frame: %#v", i, cell)
		}
		if cell := at(sepX, y); cell.R != '┫' || cell.FG != CWhite || !cell.Bold {
			t.Fatalf("side panel %d not connected to vertical divider: %#v", i, cell)
		}
		if cell := at(7, y); cell.R != '━' || cell.FG != CWhite || !cell.Bold {
			t.Fatalf("side panel %d missing thick white horizontal separator: %#v", i, cell)
		}
	}
	if cell := at(8, a.sideY[0]+1); cell.R != '━' || cell.FG != CBlue || cell.Bold {
		t.Fatalf("title/content separator should remain thin blue: %#v", cell)
	}
	if cell := at(0, c.H-3); cell.R != '┣' {
		t.Fatalf("footer line must meet outer border: %#v", cell)
	}
}

func TestSelectionMarkersAreOneContinuousHalfBlock(t *testing.T) {
	s := setup(t)
	s.Focus = PaneVault
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	marker := canvasAt(c, 1+selectionStripeInset, a.sideY[0]+2)
	if marker.R != selectionStripeGlyph || marker.FG != CCyan {
		t.Fatalf("filter must use a single cyan half-block: %#v", marker)
	}
	if cell := canvasAt(c, 1+selectionStripeInset+selectionStripeWidth, a.sideY[0]+2); cell.R == selectionStripeGlyph {
		t.Fatalf("filter selection must occupy exactly one column: %#v", cell)
	}
	s.Focus = PaneItems
	c = NewCanvas(120, 44)
	a.mainRender(c)
	for dy := 0; dy < itemRowHeight; dy++ {
		marker := canvasAt(c, a.itemX+selectionStripeInset, 3+dy)
		if !balancedStripeCell(marker, []rune{'▄', selectionStripeCenterMask, selectionStripeCenterMask}[dy], CCyan) {
			t.Fatalf("item marker row %d: %#v", dy, marker)
		}
		if adjacent := canvasAt(c, a.itemX+selectionStripeInset+selectionStripeWidth, 3+dy); adjacent.R == selectionStripeGlyph {
			t.Fatalf("multiple parallel item markers: %#v", adjacent)
		}
	}
	s.OpenItem()
	c = NewCanvas(120, 44)
	a.mainRender(c)
	if cell := canvasAt(c, a.itemX+selectionStripeInset, 3); cell.R != '▄' || cell.FG != CGreen {
		t.Fatalf("opened item must be one green half-block: %#v", cell)
	}
	if cell := canvasAt(c, a.contentX+selectionStripeInset, 3); cell.R != '▄' || cell.FG != CCyan {
		t.Fatalf("selected field must be one cyan half-block: %#v", cell)
	}
	if cell := c.Cells[a.contentX-1]; cell.R != '┳' || cell.FG != CWhite || !cell.Bold {
		t.Fatalf("content divider not connected to frame: %#v", cell)
	}
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	c = NewCanvas(120, 44)
	a.mainRender(c)
	if a.detailX != a.contentX || a.detailY <= 1 {
		t.Fatalf("details must be stacked: x=%d contentX=%d y=%d", a.detailX, a.contentX, a.detailY)
	}
	if cell := canvasAt(c, a.detailX-1, a.detailY-1); cell.R != '┣' || cell.FG != CWhite || !cell.Bold {
		t.Fatalf("detail divider not connected: %#v", cell)
	}
}

func TestAuthButtonSymmetrySpinnerPositionAndHintColors(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		for _, width := range []int{100, 101, 120, 121} {
			height := 40
			g := authGeometry(width, height, screen)
			if g.enterX != width-g.quitX-g.buttonWidth {
				t.Fatalf("%v/%d: unequal left/right margins", screen, width)
			}
			if gap := g.quitX - (g.enterX + g.buttonWidth); gap < 3 || gap > 4 {
				t.Fatalf("%v/%d: unexpected gap %d", screen, width, gap)
			}
			s.Screen = screen
			s.Busy = true
			s.BusyMessage = "Authenticating"
			s.AuthFocus = 2
			c := NewCanvas(width, height)
			a.authRender(c)
			for _, buttonX := range []int{g.enterX, g.quitX} {
				for xx := buttonX; xx < buttonX+g.buttonWidth; xx++ {
					if c.Cells[g.buttonY*c.W+xx].BG == CDefault ||
						(c.Cells[(g.buttonY+1)*c.W+xx].R != buttonAccentGlyph) {
						t.Fatalf("%v/%d: button cell missing at %d", screen, width, xx)
					}
				}
			}
			// The busy panel may not cover the buttons or the API tip.
			for y := g.buttonY; y <= g.buttonY+1; y++ {
				if c.Cells[y*c.W+width/2].BG == CBlack {
					t.Fatalf("%v/%d: popup overlaps the button row", screen, width)
				}
			}
			if screen == AuthClient {
				if !strings.Contains(stringCellRow(c, g.tipY+1), "Hint. Sign in") {
					t.Fatal("API hint hidden by spinner")
				}
			}
			footer := stringCellRow(c, g.footerY)
			index := strings.Index(footer, "ENTER select")
			if index < 0 {
				t.Fatalf("footer hint missing: %q", footer)
			}
			indexKey := utf8.RuneCountInString(footer[:index])
			if c.Cells[g.footerY*c.W+indexKey].FG != CHintKey ||
				c.Cells[g.footerY*c.W+indexKey+len("ENTER ")].FG != CHintAction {
				t.Fatalf("%v: key and action should use different grayscale intensities", screen)
			}
		}
	}
}

func TestCompactClientAuthenticationPreservesSpaceForLoadingMessage(t *testing.T) {
	s := setup(t)
	s.Screen = AuthClient
	s.Busy = true
	s.BusyMessage = "Signing in"
	a := New(s)
	for _, h := range []int{23, 25, 28, 30, 35} {
		g := authGeometry(100, h, AuthClient)
		if g.tipY-g.buttonY < 6 {
			t.Fatalf("%d rows: insufficient room between auth buttons and API hint", h)
		}
		c := NewCanvas(100, h)
		a.authRender(c)
		if !strings.Contains(stringCellRow(c, g.tipY+1), "Hint. Sign in") {
			t.Fatalf("%d rows: spinner obscures API hint", h)
		}
		if !strings.Contains(c.Render(), "Signing in") {
			t.Fatalf("%d rows: spinner message missing", h)
		}
	}
}

func TestAuthenticationScreensAreFramedFlushWithTerminal(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, screen := range []Screen{AuthChecking, AuthClient, AuthMaster} {
		s.Screen = screen
		for _, size := range [][2]int{{80, 25}, {101, 40}, {121, 45}} {
			c := NewCanvas(size[0], size[1])
			if screen == AuthChecking {
				a.startupRender(c)
			} else {
				a.authRender(c)
			}
			at := func(x, y int) Cell { return c.Cells[y*c.W+x] }
			for _, corner := range []struct {
				x, y  int
				glyph rune
			}{
				{0, 0, '┏'}, {c.W - 1, 0, '┓'},
				{0, c.H - 1, '┗'}, {c.W - 1, c.H - 1, '┛'},
			} {
				cell := at(corner.x, corner.y)
				if cell.R != corner.glyph || cell.FG != CWhite || !cell.Bold {
					t.Fatalf("screen %v (%d×%d): auth frame corner %#v", screen, c.W, c.H, cell)
				}
			}
			if at(0, c.H/2).R != '┃' || at(c.W-1, c.H/2).R != '┃' {
				t.Fatalf("screen %v: vertical frame missing", screen)
			}
			if at(c.W/2, 0).R != '━' || at(c.W/2, c.H-1).R != '━' {
				t.Fatalf("screen %v: horizontal frame missing", screen)
			}
			// The frame must not replace the form title or the loading message.
			if screen == AuthChecking && !strings.Contains(stringCellRow(c, c.H/2), "Checking") {
				t.Fatalf("neutral startup spinner missing")
			}
			if screen == AuthClient || screen == AuthMaster {
				position := authGeometry(c.W, c.H, screen)
				title := "Bitwarden API credentials"
				if screen == AuthMaster {
					title = "Unlock Bitwarden"
				}
				if !strings.Contains(stringCellRow(c, max(1, position.firstY-2)), title) {
					t.Fatalf("screen %v: auth title missing above upper input", screen)
				}
			}
		}
	}
}

func TestInactiveItemsAndFindDividersStayBlue(t *testing.T) {
	s := setup(t)
	s.Focus = PaneVault
	a := New(s)
	c := NewCanvas(120, 45)
	a.mainRender(c)
	findSeparatorY := 1 + (c.H - 4) - 2
	search := c.Cells[findSeparatorY*c.W+a.itemX+8]
	title := c.Cells[2*c.W+a.itemX+8]
	if search.R != '━' || search.FG != CBlue || search.Bold {
		t.Fatalf("search divider must be normal-width blue: %#v", search)
	}
	if search.R != title.R || search.FG != title.FG || search.Bold != title.Bold {
		t.Fatalf("search and title dividers should match: search=%#v title=%#v", search, title)
	}
}

func TestDetailsIsBelowContentsAndDoesNotResizeColumns(t *testing.T) {
	s := setup(t)
	a := New(s)
	const w, h = 130, 48
	s.OpenItem()
	before := NewCanvas(w, h)
	a.mainRender(before)
	itemsWidth, contentWidth, contentX := a.itemW, a.contentW, a.contentX
	if a.detailY != 0 {
		t.Fatalf("detail panel visible before editing")
	}
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	during := NewCanvas(w, h)
	a.mainRender(during)
	if a.itemW != itemsWidth || a.contentW != contentWidth || a.contentX != contentX {
		t.Fatalf("column resized during edit: item=%d/%d content=%d/%d", a.itemW, itemsWidth, a.contentW, contentWidth)
	}
	if a.detailX != a.contentX || a.detailY <= 4 || a.detailH < 5 {
		t.Fatalf("detail panel is not stacked in content column: x=%d y=%d h=%d", a.detailX, a.detailY, a.detailH)
	}
	for _, x := range []int{a.contentX + 1, w - 2} {
		v := during.Cells[(a.detailY-1)*w+x]
		if v.R != '━' || v.FG != CWhite || !v.Bold {
			t.Fatalf("missing white horizontal detail separator at x=%d: %#v", x, v)
		}
	}
	if cell := during.Cells[(a.detailY-1)*w+(w-1)]; cell.R != '┫' || !cell.Bold {
		t.Fatalf("detail separator fails to meet right frame: %#v", cell)
	}
	if !strings.Contains(stringCellRow(during, a.detailY), s.CurrentField().Name) {
		t.Fatalf("edited field name missing from lower detail title")
	}
}

func TestBoldPanelBordersAreRenderedSeparatelyFromBlueRules(t *testing.T) {
	c := NewCanvas(12, 5)
	drawMainBorders(c, nil, nil, -1)
	c.Line(2, 2, 8, '━', CBlue, CDefault)
	if !strings.Contains(c.Render(), "\x1b[1m") {
		t.Fatal("intermediate-weight white borders are not emitted bold")
	}
	thin := c.Cells[2*c.W+4]
	border := c.Cells[4]
	if thin.Bold || !border.Bold || thin.R != border.R {
		t.Fatalf("heavy glyph should differ in weight: thin=%#v border=%#v", thin, border)
	}
}

func TestSelectedFieldScrollsIntoVisibleUpperContentPane(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	if len(s.Fields) == 0 {
		t.Fatal("test fixture lacks editable fields")
	}
	// Reuse the fixture's existing field so encryption and field metadata
	// remain valid. A long item must not hide its selected field under Detail.
	exemplar := s.Fields[0]
	s.Fields = make([]vault.Field, 28)
	for i := range s.Fields {
		s.Fields[i] = exemplar
		s.Fields[i].Name = fmt.Sprintf("Field %02d", i)
	}
	s.FieldIndex = 26
	s.Mode = Edit
	s.Focus = PaneDetail
	a := New(s)
	c := NewCanvas(110, 35)
	a.mainRender(c)
	if a.contentScroll <= 0 {
		t.Fatal("content pane should scroll to selected field")
	}
	visible := false
	for y := 3; y < a.detailY-1; y++ {
		if strings.Contains(stringCellRow(c, y), "Field 26") {
			visible = true
			break
		}
	}
	if !visible {
		t.Fatal("selected field was hidden by the stacked Detail panel")
	}
}
