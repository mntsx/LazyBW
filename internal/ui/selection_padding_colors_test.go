package ui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestStatusMarkers tests the actual foreground cells, not only ANSI strings.
func TestChangesRenderAllKindsInTheirNormalColors(t *testing.T) {
	s := setup(t)
	s.Stage.Record("item", map[string]any{"id": "local-new", "name": "New entry", "type": 1}, "Login", s.Originals)
	it := s.CurrentItem()
	if err := it.Set([]any{"login", "username"}, "modified"); err != nil {
		t.Fatal(err)
	}
	s.Stage.Record("item", it.Raw, "Login", s.Originals)
	s.Originals["id-deleted"] = map[string]any{"id": "id-deleted", "name": "Deleted entry", "type": 1}
	s.Stage.Delete("item", "id-deleted", "Login", s.Originals)
	s.Focus = PaneChanges
	s.ChangeIndex = 0
	a := New(s)
	c := NewCanvas(140, 50)
	a.mainRender(c)
	for index, expected := range []struct {
		glyph rune
		color int
	}{{'N', CGreen}, {'M', CYellow}, {'D', CRed}} {
		y := a.sideY[3] + 2 + index
		row := stringCellRow(c, y)
		at := strings.LastIndex(row, string(expected.glyph))
		if at < 0 {
			t.Fatalf("missing %q: %s", expected.glyph, row)
		}
		x := utf8.RuneCountInString(row[:at])
		if cell := canvasAt(c, x, y); cell.FG != expected.color {
			t.Fatalf("change %q has foreground %d, want %d", expected.glyph, cell.FG, expected.color)
		}
		count := fmt.Sprintf("%d/3", index+1)
		if (index == s.ChangeIndex) != strings.Contains(row, count) {
			t.Fatalf("counter must appear only on the keyboard-selected change %d: %q", index, row)
		}
	}
}

func TestContentStageLettersAndCounterDoNotOverlap(t *testing.T) {
	for _, tc := range []struct {
		kind vault.Kind
		fg   int
	}{{vault.New, CGreen}, {vault.Modified, CYellow}, {vault.Deleted, CRed}} {
		s := setup(t)
		s.OpenItem()
		switch tc.kind {
		case vault.New:
			// An item with an existing field can be pending creation.
			delete(s.Originals, s.SelectedID)
			s.Stage.Record("item", s.SelectedItem().Raw, "Login", s.Originals)
		case vault.Modified:
			it := s.SelectedItem()
			if err := it.Set([]any{"login", "username"}, "updated"); err != nil {
				t.Fatal(err)
			}
			s.Stage.Record("item", it.Raw, "Login", s.Originals)
		case vault.Deleted:
			s.Stage.Delete("item", s.SelectedID, "Login", s.Originals)
		}
		s.Focus = PaneContent
		a := New(s)
		c := NewCanvas(120, 44)
		a.mainRender(c)
		labelY := 4
		kindX := a.contentX + a.contentW - 9
		if cell := canvasAt(c, kindX, labelY); cell.R != rune(tc.kind[0]) || cell.FG != tc.fg {
			t.Fatalf("kind %s: %+v at (%d,%d)", tc.kind, cell, kindX, labelY)
		}
		count := fmt.Sprintf("1/%d", len(s.Fields))
		countX := a.contentX + a.contentW - utf8.RuneCountInString(count) - 2
		if countX-kindX < 3 {
			t.Fatalf("kind and counter too close: %d", countX-kindX)
		}
		if got := stringCellRow(c, labelY); !strings.Contains(got, count) {
			t.Fatalf("counter absent: %q", got)
		}
	}
}

func TestContentRowsUseSharedPaddingAndStripeSpansRow(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.Focus = PaneContent
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	for dy := 0; dy < contentRowHeight; dy++ {
		for dx := 0; dx < selectionStripeWidth; dx++ {
			if cell := canvasAt(c, a.contentX+selectionStripeInset+dx, 3+dy); !balancedStripeCell(cell, []rune{'▄', selectionStripeCenterMask, selectionStripeCenterMask}[dy], CCyan) {
				t.Fatalf("stripe at (%d,%d): %#v", dx, dy, cell)
			}
		}
	}
	for _, y := range []int{3} {
		if cell := canvasAt(c, a.contentX+3, y); cell.R != '▄' || cell.FG != CBlack || cell.BG != CDefault {
			t.Fatalf("padding at %d: %#v", y, cell)
		}
	}
	if cell := canvasAt(c, a.contentX+3, 4); cell.R == ' ' {
		t.Fatal("missing field name")
	}
	if cell := canvasAt(c, a.contentX+3, 5); cell.R == ' ' {
		t.Fatal("missing field value")
	}
}

func TestThreeRowClickGeometry(t *testing.T) {
	for y := 3; y <= 5; y++ {
		if index, ok := itemHitIndex(48, y, 40, 30, 44, 0, 2); !ok || index != 0 {
			t.Fatalf("first row click %d => %d %t", y, index, ok)
		}
	}
	for y := 6; y <= 8; y++ {
		if index, ok := itemHitIndex(48, y, 40, 30, 44, 0, 2); !ok || index != 1 {
			t.Fatalf("second row click %d => %d %t", y, index, ok)
		}
	}
	if _, ok := itemHitIndex(48, 9, 40, 30, 44, 0, 2); ok {
		t.Fatal("click after item list selected an item")
	}
}
