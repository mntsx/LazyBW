package ui

import (
	"strings"
	"testing"
)

// TestOnlineFocusedFilterExpands verifies compact siblings and a maximized active panel.
func TestOnlineFocusedFilterExpands(t *testing.T) {
	s := setup(t)
	s.LocalMode = false
	for _, height := range []int{30, 44, 60} {
		for _, focus := range []Pane{PaneVault, PaneTypes, PaneFolders, PaneChanges} {
			s.Focus = focus
			a := New(s)
			c := NewCanvas(120, height)
			a.mainRender(c)

			minimum := [4]int{3, 3, 2, 1}
			if s.FilterFolder >= 0 {
				minimum[2] = 3
			}
			available := height - 4 - 3
			extra := available
			for _, value := range minimum {
				extra -= value
			}
			for index, pane := range []Pane{PaneVault, PaneTypes, PaneFolders, PaneChanges} {
				want := minimum[index]
				if pane == focus {
					want += max(0, extra)
				}
				if a.sideH[index] != want {
					t.Fatalf("height %d focus %v panel %v: got %d want %d",
						height, focus, pane, a.sideH[index], want)
				}
			}
		}
	}
}

// TestLocalSidebarRemainsFixed verifies that focus does not resize offline filters.
func TestLocalSidebarRemainsFixedAfterOnlineExpansion(t *testing.T) {
	s := setup(t)
	s.LocalMode = true
	var initial [4]int
	for index, focus := range []Pane{PaneTypes, PaneFolders, PaneItems, PaneContent} {
		s.Focus = focus
		a := New(s)
		a.mainRender(NewCanvas(120, 44))
		if index != 0 && a.sideH != initial {
			t.Fatalf("local focus %v changed sidebar: %v -> %v", focus, initial, a.sideH)
		}
		initial = a.sideH
	}
}

// TestFavouritesAreBoldBrightYellow checks stars in both panels, not stage colors.
func TestFavouritesAreBoldBrightYellow(t *testing.T) {
	s := setup(t)
	item := s.CurrentItem()
	if item == nil {
		t.Fatal("missing test item")
	}
	item.Raw["favorite"] = true
	a := New(s)
	c := NewCanvas(110, 32)
	a.itemsPanel(c, 35, 1, 74, 26)
	starX := 35 + 9 + len([]rune(item.Name()))
	itemStar := canvasAt(c, starX, 4)
	if itemStar.R != '\uf005' || itemStar.FG != CBYellow || !itemStar.Bold {
		t.Fatalf("items favourite is not bright yellow and bold: %+v", itemStar)
	}
	if got := colorCode(itemStar.FG, false); got != "\x1b[38;5;11m" {
		t.Fatalf("items star is not ANSI bright yellow: %q", got)
	}
	rendered := c.Render()
	if !strings.Contains(rendered, "\x1b[38;5;11m") {
		t.Fatal("render missing ANSI bright yellow")
	}
	s.OpenItem()
	c = NewCanvas(110, 32)
	a.contentPanel(c, 35, 1, 74, 26)
	contentStar := canvasAt(c, 35+74-3, 1)
	if contentStar.R != '\uf005' || contentStar.FG != CBYellow || !contentStar.Bold {
		t.Fatalf("content favourite is not bright yellow and bold: %+v", contentStar)
	}
	item.Raw["favorite"] = false
	c = NewCanvas(110, 32)
	a.contentPanel(c, 35, 1, 74, 26)
	contentStar = canvasAt(c, 35+74-3, 1)
	if !contentStar.Dim || contentStar.Bold || contentStar.FG != CWhite {
		t.Fatalf("inactive favourite should be dimmed white: %+v", contentStar)
	}
}
