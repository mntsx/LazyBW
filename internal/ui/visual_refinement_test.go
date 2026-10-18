package ui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func canvasAt(c *Canvas, x, y int) Cell {
	return c.Cells[y*c.W+x]
}

func TestSelectedPanelNumberPulsesNormalYellow(t *testing.T) {
	s := setup(t)
	s.Focus = PaneVault
	a := New(s)
	for _, blinking := range []bool{false, true} {
		s.Blink = blinking
		c := NewCanvas(120, 44)
		a.mainRender(c)
		color := CTitle
		if blinking {
			color = CYellow
		}
		if got := canvasAt(c, 3, a.sideY[0]); got.R != '1' || got.FG != color || got.Bold {
			t.Fatalf("vault panel active blink=%t: %#v", blinking, got)
		}
		if got := canvasAt(c, 3, a.sideY[1]); got.FG != CWhite {
			t.Fatalf("inactive panel number must stay white: %#v", got)
		}
		s.Focus = PaneItems
		c = NewCanvas(120, 44)
		a.mainRender(c)
		if got := canvasAt(c, a.itemX+2, 1); got.R != '5' || got.FG != color || got.Bold {
			t.Fatalf("items header blink=%t: %#v", blinking, got)
		}
		s.Focus = PaneVault
	}
}

func TestActiveAndSelectedMarkersHaveDistinctColors(t *testing.T) {
	s := setup(t)
	s.Focus = PaneVault
	s.FilterVault = 0 // My vault is applied.
	s.Filters[0] = 1  // All vaults is keyboard-selected.
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	activeY := a.sideY[0] + 3
	selectedY := a.sideY[0] + 2
	if got := canvasAt(c, 1+selectionStripeInset, activeY); got.R != selectionStripeGlyph || got.FG != CGreen {
		t.Fatalf("active filter must have normal green line: %#v", got)
	}
	if got := canvasAt(c, 1+selectionStripeInset, selectedY); got.R != selectionStripeGlyph || got.FG != CCyan {
		t.Fatalf("keyboard-selected filter must have normal cyan line: %#v", got)
	}
	s.Filters[0] = 0
	c = NewCanvas(120, 44)
	a.mainRender(c)
	if got := canvasAt(c, 1+selectionStripeInset, a.sideY[0]+3); got.FG != CCyan {
		t.Fatalf("cyan should win if a filter is selected AND active: %#v", got)
	}

	s.Focus = PaneItems
	s.FilterVault = 1
	s.RebuildFromStaging()
	s.OpenItem()
	c = NewCanvas(120, 44)
	a.mainRender(c)
	if got := canvasAt(c, a.itemX+selectionStripeInset, 3); got.R != '▄' || got.FG != CGreen {
		t.Fatalf("opened item must have active green line: %#v", got)
	}
	if got := canvasAt(c, a.contentX+selectionStripeInset, 3); got.R != '▄' || got.FG != CCyan {
		t.Fatalf("selected content field must have cyan line: %#v", got)
	}
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	c = NewCanvas(120, 44)
	a.mainRender(c)
	if got := canvasAt(c, a.contentX+selectionStripeInset, 3); got.R != '▄' || got.FG != CGreen {
		t.Fatalf("field being edited should have active green line: %#v", got)
	}
}

func TestItemRowsHaveCompactSharedVerticalPadding(t *testing.T) {
	s := setup(t)
	s.Focus = PaneItems
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	for dy := 0; dy < itemRowHeight; dy++ {
		for dx := 0; dx < selectionStripeWidth; dx++ {
			if got := canvasAt(c, a.itemX+selectionStripeInset+dx, 3+dy); !balancedStripeCell(got, []rune{'▄', selectionStripeCenterMask, selectionStripeCenterMask}[dy], CCyan) {
				t.Fatalf("selection stripe (%d,%d): %#v", dx, dy, got)
			}
		}
	}
	for _, y := range []int{3} {
		if got := canvasAt(c, a.itemX+8, y); got.R != '▄' || got.FG != CBlack || got.BG != CDefault {
			t.Fatalf("shared padding missing at row %d: %#v", y, got)
		}
	}
	if got := canvasAt(c, a.itemX+4, 4); got.R == ' ' {
		t.Fatal("item icon should be placed inside centered two-line text")
	}
	if got := canvasAt(c, a.itemX+8, 5); got.FG != CSubtitle {
		t.Fatalf("subtitle lost its dim white color: %#v", got)
	}
}

func TestSidePanelLeavesBottomContentPadding(t *testing.T) {
	s := setup(t)
	s.Focus = PaneItems
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	for i := 0; i < 4; i++ {
		if a.sideH[i] < 5 {
			continue
		}
		lastY := a.sideY[i] + a.sideH[i] - 1
		if got := canvasAt(c, 7, lastY); got.R != ' ' {
			t.Fatalf("side panel %d is missing bottom padding: %#v", i, got)
		}
	}
}

func TestFooterShowsFullBitwardenCLILabelAndGreenSmallDot(t *testing.T) {
	s := setup(t)
	s.Version = "2026.9.1"
	a := New(s)
	c := NewCanvas(120, 44)
	a.footer(c, 1, 42, 118)
	row := stringCellRow(c, 42)
	index := strings.Index(row, "• Bitwarden CLI 2026.9.1")
	if index == -1 {
		t.Fatalf("incorrect CLI status: %q", row)
	}
	if got := canvasAt(c, utf8.RuneCountInString(row[:index]), 42); got.FG != CGreen || got.R != '•' {
		t.Fatalf("status indicator should be a small green dot: %#v", got)
	}
}

func TestPopupShowsOnlyItsOwnHintsAndBlocksGlobalActions(t *testing.T) {
	s := setup(t)
	s.Screen = Workspace
	s.Focus = PaneItems
	s.Popup = &Popup{Title: "Confirm", Question: "Discard changes?", Yes: false}
	a := New(s)
	hints := strings.Join(a.hints(), " | ")
	for _, expected := range []string{"y yes", "n no", "c cancel", "ENTER confirm"} {
		if !strings.Contains(hints, expected) {
			t.Fatalf("missing popup hint %q in %q", expected, hints)
		}
	}
	for _, forbidden := range []string{"ctrl+q", "ctrl+s", "ctrl+d", "f find"} {
		if strings.Contains(hints, forbidden) {
			t.Fatalf("unrelated key shown while popup open: %q", hints)
		}
	}
	if err := a.char('1'); err != nil {
		t.Fatal(err)
	}
	if s.Focus != PaneItems {
		t.Fatal("background panel reacted to a key during confirmation")
	}
	a.requestQuit()
	if s.Popup == nil {
		t.Fatal("quit action must not replace or dismiss an open popup")
	}
	a.confirmStage(true, true)
	if s.Popup == nil || s.Popup.Title != "Confirm" {
		t.Fatal("save action should not open another dialog while popup active")
	}
	if err := a.char('c'); err != nil {
		t.Fatal(err)
	}
	if s.Popup != nil {
		t.Fatal("c cancel should close popup")
	}
}

func TestBlockingPopupHasNoActiveKeybindings(t *testing.T) {
	s := setup(t)
	s.Popup = &Popup{Blocking: true, Title: "Saving changes"}
	a := New(s)
	if hints := a.hints(); len(hints) != 0 {
		t.Fatalf("blocking dialog must not advertise keyboard commands: %v", hints)
	}
	c := NewCanvas(120, 44)
	a.footer(c, 1, 42, 118) // must not panic on an empty hint page
	if err := a.char('y'); err != nil {
		t.Fatal(err)
	}
	if s.Popup == nil || !s.Popup.Blocking {
		t.Fatal("input unexpectedly dismissed blocking popup")
	}
}

func TestCreatePopupDoesNotAdvertiseGlobalShortcuts(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, mode := range []Mode{DialogCreate, DialogName, DialogFieldType, DialogField, DialogFolder} {
		s.Mode = mode
		hints := strings.Join(a.hints(), " | ")
		if strings.Contains(hints, "ctrl+q") || strings.Contains(hints, "ctrl+s") {
			t.Fatalf("create dialog %v shows global hints: %q", mode, hints)
		}
		selectionKey := "ENTER"
		if mode == DialogCreate || mode == DialogFieldType {
			selectionKey = "→"
		}
		if !strings.Contains(hints, selectionKey) || !strings.Contains(hints, "esc") {
			t.Fatalf("create dialog %v lacks its navigation hints: %q", mode, hints)
		}
	}
}

func TestValidatedMasterPasswordSwitchesToNeutralVaultLoading(t *testing.T) {
	s := setup(t)
	s.Screen = AuthMaster
	s.Master = "example-not-a-real-password"
	s.Busy = true
	s.BusyMessage = "Authenticating"
	s.BeginVaultLoading()
	if s.Screen != AuthChecking || !s.Busy || s.Master != "" || s.BusyMessage != "Loading vault" {
		t.Fatalf("incorrect neutral loading transition: screen=%v busy=%v message=%q", s.Screen, s.Busy, s.BusyMessage)
	}
	a := New(s)
	c := NewCanvas(120, 44)
	a.startupRender(c)
	display := c.Render()
	if !strings.Contains(display, "Loading vault") || strings.Contains(display, "Master password") {
		t.Fatal("loading screen must be neutral, without the master password form")
	}
}
