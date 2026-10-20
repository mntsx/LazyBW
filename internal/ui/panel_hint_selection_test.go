package ui

import (
	"reflect"
	"strings"
	"testing"
)

// balancedStripeCell verifies which ANSI color channel paints a stripe cell.
func balancedStripeCell(got Cell, glyph rune, color int) bool {
	return got.R == glyph && got.FG == color && got.BG == CDefault
}

func TestPanelNavigationHintsFollowFocusedAndVisiblePanels(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, tc := range []struct {
		name       string
		focus      Pane
		selectedID bool
		panels     string
		back       string
	}{
		{"items", PaneItems, false, "1/2/3/4 panels", ""},
		{"vault", PaneVault, false, "2/3/4/5 panels", "1 return"},
		{"types", PaneTypes, false, "1/3/4/5 panels", "2 return"},
		{"folders", PaneFolders, false, "1/2/4/5 panels", "3 return"},
		{"changes", PaneChanges, false, "1/2/3/5 panels", "4 return"},
		{"content", PaneContent, true, "1/2/3/4/5 panels", "6 return"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.Mode = Browse
			s.Focus = tc.focus
			s.SelectedID = ""
			if tc.selectedID {
				s.SelectedID = s.CurrentItem().ID()
			}
			nav := panelNavigationHints(s)
			expected := []string{tc.panels}
			if tc.back != "" {
				expected = append(expected, tc.back)
			}
			if !reflect.DeepEqual(nav, expected) {
				t.Fatalf("got %v, expected %v", nav, expected)
			}
			all := strings.Join(a.hints(), " | ")
			if !strings.Contains(all, tc.panels) || strings.Contains(all, "5 items") {
				t.Fatalf("wrong hints: %s", all)
			}
		})
	}
}

func TestActivePanelNumberReturnsToItemsOrClosesContent(t *testing.T) {
	s := setup(t)
	a := New(s)
	for number := rune('1'); number <= '4'; number++ {
		s.Focus = Pane(number - '0')
		if err := a.char(number); err != nil {
			t.Fatal(err)
		}
		if s.Focus != PaneItems {
			t.Fatalf("%c did not return to Items: %v", number, s.Focus)
		}
	}
	if err := a.char('6'); err != nil {
		t.Fatal(err)
	}
	if s.Focus != PaneItems {
		t.Fatalf("closed content should not be activated: %v", s.Focus)
	}
	s.OpenItem()
	if s.Focus != PaneContent || s.SelectedID == "" {
		t.Fatal("item was not opened")
	}
	if err := a.char('6'); err != nil {
		t.Fatal(err)
	}
	if s.Focus != PaneItems || s.SelectedID != "" || s.Mode != Browse {
		t.Fatalf("6 return did not close content cleanly: %#v", s)
	}
}

func TestDetailEditorPreservesNumericInput(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	a := New(s)
	start := s.Input
	if err := a.char('7'); err != nil {
		t.Fatal(err)
	}
	if s.Focus != PaneDetail || s.Mode != Edit || s.Input != "7"+start {
		t.Fatal("typing digit 7 in Details should not leave the editor")
	}
	h := strings.Join(a.hints(), " | ")
	if strings.Contains(h, "panels") || strings.Contains(h, "return") ||
		!strings.Contains(h, "esc cancel") {
		t.Fatalf("detail editor must not advertise panel shortcuts: %s", h)
	}
	if got := a.keyAction("alt+7"); got != "unbound" {
		t.Fatalf("detail must not bind Alt+7: %s", got)
	}
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	if err := a.escape(); err != nil {
		t.Fatal(err)
	}
	if s.Focus != PaneContent || s.Mode != Browse {
		t.Fatal("escaping Details must return to Content")
	}
}

func TestBalancedItemAndContentMarkersKeepHalfHeightEdges(t *testing.T) {
	s := setup(t)
	a := New(s)
	s.Focus = PaneItems
	c := NewCanvas(120, 44)
	a.mainRender(c)
	for _, tc := range []struct {
		y     int
		glyph rune
	}{
		{3, '▄'}, {4, selectionStripeCenterMask},
		{5, selectionStripeCenterMask}, {6, '▀'},
	} {
		got := canvasAt(c, a.itemX, tc.y)
		if !balancedStripeCell(got, tc.glyph, CCyan) {
			t.Fatalf("item marker at y=%d: %+v", tc.y, got)
		}
	}
	s.OpenItem()
	s.Focus = PaneContent
	c = NewCanvas(120, 44)
	a.mainRender(c)
	for _, tc := range []struct {
		y     int
		glyph rune
	}{
		{3, '▄'}, {4, selectionStripeCenterMask},
		{5, selectionStripeCenterMask}, {6, '▀'},
	} {
		got := canvasAt(c, a.contentX, tc.y)
		if !balancedStripeCell(got, tc.glyph, CCyan) {
			t.Fatalf("content marker at y=%d: %+v", tc.y, got)
		}
	}
}

func TestDetailCannotNavigateByDigits(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	a := New(s)
	old := s.Input
	for _, digit := range "1234567" {
		if err := a.char(digit); err != nil {
			t.Fatal(err)
		}
	}
	if s.Focus != PaneDetail || s.Mode != Edit || s.Input != "1234567"+old {
		t.Fatal("number keys must only type into the editor")
	}
}
