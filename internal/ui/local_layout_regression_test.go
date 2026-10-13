package ui

import (
	"strings"
	"testing"
	"time"
)

// TestLocalPanelNumbers checks the displayed shortcuts and keyboard routing.
func TestLocalPanelNumbers(t *testing.T) {
	s := setup(t)
	s.LocalMode = true
	s.Server = "local"
	a := New(s)
	for number, pane := range []Pane{PaneTypes, PaneFolders, PaneItems, PaneContent} {
		if got := panelNumber(s, pane); got != number+1 {
			t.Fatalf("number %d became %d", number+1, got)
		}
		if got := paneForNumber(s, number+1); got != pane {
			t.Fatalf("panel %d became %d", number+1, got)
		}
	}
	for _, tc := range []struct {
		key  rune
		pane Pane
	}{{'1', PaneTypes}, {'2', PaneFolders}, {'3', PaneItems}} {
		s.Focus = PaneItems
		if err := a.char(tc.key); err != nil || s.Focus != tc.pane {
			t.Fatalf("local key %c not routed: %v", tc.key, err)
		}
	}
	s.Focus = PaneItems
	s.OpenItem()
	if err := a.char('4'); err != nil || s.Focus != PaneItems {
		t.Fatalf("local content return failed: %v", err)
	}
	if err := a.char('7'); err != nil || s.Focus != PaneItems {
		t.Fatal("hidden panel unexpectedly activated")
	}
}

// TestLocalPanelsStayFixed verifies focus cannot resize the sidebar sections.
func TestLocalPanelsStayFixed(t *testing.T) {
	s := setup(t)
	s.LocalMode = true
	a := New(s)
	var expected [4]int
	var previous [4]int
	for index, pane := range []Pane{PaneItems, PaneTypes, PaneFolders, PaneContent} {
		s.Focus = pane
		c := NewCanvas(120, 44)
		a.mainRender(c)
		if a.sideY[1] != 1 || a.sideY[2] != a.sideY[1]+a.sideH[1]+1 ||
			(index > 0 && (a.sideY != previous || a.sideH != expected)) {
			t.Fatalf("local sidebar changed with focus %v: %v %v", pane, a.sideY, a.sideH)
		}
		expected, previous = a.sideH, a.sideY
		for _, tc := range []struct {
			pane Pane
			name string
		}{
			{PaneTypes, "[1]"}, {PaneFolders, "[2]"},
		} {
			row := stringCellRow(c, a.sideY[int(tc.pane)-1])
			if !strings.Contains(row, tc.name) {
				t.Fatalf("wrong heading %v: %q", pane, row)
			}
		}
		if got := canvasAt(c, 1, 42); got.R == 0 {
			t.Fatal("footer not painted")
		}
	}
}

// TestLocalFooterRestoresStatus ensures the backup notice is temporary.
func TestLocalFooterRestoresStatus(t *testing.T) {
	s := setup(t)
	s.LocalMode = true
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	if cell := canvasAt(c, 82, 42); cell.R == 0 {
		t.Fatal("footer was not rendered")
	}
	s.Notice = "Local vault backed up"
	s.NoticeUntil = time.Now().Add(-time.Second)
	c = NewCanvas(120, 44)
	a.mainRender(c)
	if !strings.Contains(stringCellRow(c, 42), "Local") ||
		strings.Contains(stringCellRow(c, 42), "backed up") {
		t.Fatalf("expired backup notice remained: %s", stringCellRow(c, 42))
	}
}
