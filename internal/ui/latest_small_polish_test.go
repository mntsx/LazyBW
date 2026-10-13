package ui

import (
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

func TestFindDividerStaysMagentaWhileTyping(t *testing.T) {
	s := setup(t)
	s.Mode = Find
	s.Focus = PaneItems
	a := New(s)
	for _, tc := range []struct {
		blink bool
		color int
	}{{false, CMagenta}, {true, CMagenta}} {
		s.Blink = tc.blink
		c := NewCanvas(120, 44)
		a.mainRender(c)
		y := 44 - 5
		for x := a.itemX; x < a.itemX+a.itemW; x++ {
			got := canvasAt(c, x, y)
			if got.R != '━' || got.FG != tc.color {
				t.Fatalf("find rule at %d, blink %t: %+v", x, tc.blink, got)
			}
		}
		if got := canvasAt(c, a.itemX+2, 1); got.FG != CWhite {
			t.Fatalf("item panel number must not blink while finding: %+v", got)
		}
	}
}

func TestActivePanelNumbersBlinkYellow(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, pane := range []Pane{PaneVault, PaneTypes, PaneFolders, PaneChanges, PaneItems} {
		s.Focus = pane
		for _, tc := range []struct {
			blink bool
			color int
		}{{false, CTitle}, {true, CYellow}} {
			s.Blink = tc.blink
			c := NewCanvas(120, 44)
			a.mainRender(c)
			x, y := a.itemX+2, 1
			if pane != PaneItems {
				x, y = 3, a.sideY[int(pane)-1]
			}
			got := canvasAt(c, x, y)
			if got.FG != tc.color || got.Bold {
				t.Fatalf("pane %d, blink %t: %+v", pane, tc.blink, got)
			}
		}
	}
}

func TestFolderStartsUnappliedButNoFolderHighlighted(t *testing.T) {
	s := setup(t)
	if s.FilterFolder != -1 || s.Filters[2] != 0 {
		t.Fatalf("folder filter should start unapplied with No Folder highlighted: filter=%d cursor=%d", s.FilterFolder, s.Filters[2])
	}
	folderItem := vault.NewItem(vault.Login, "In a folder", "folder-item", s.CLI.Cipher)
	folderItem.Raw["folderId"] = "folder-1"
	s.Items = append(s.Items, folderItem)
	s.Folders = append(s.Folders, map[string]any{"id": "folder-1", "name": "Folder 1"})
	filtered := s.Filtered()
	if len(filtered) != 2 {
		t.Fatalf("No folder filter must initially show all items, got %v", filtered)
	}
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	got := canvasAt(c, 1+selectionStripeInset, a.sideY[2]+2)
	if got.R == selectionStripeGlyph && got.FG == CGreen {
		t.Fatalf("No Folder must not start applied: %+v", got)
	}
}

func TestCompactRowsRemainClickableAndKeepBothTextLines(t *testing.T) {
	if itemRowHeight != 3 || contentRowHeight != 3 {
		t.Fatalf("expected 3-cell rows, got items=%d fields=%d", itemRowHeight, contentRowHeight)
	}
	s := setup(t)
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	if got := canvasAt(c, a.itemX+8, 4); got.R != 'E' {
		t.Fatalf("item name disappeared: %+v", got)
	}
	if got := canvasAt(c, a.itemX+8, 5); got.FG != CSubtitle {
		t.Fatalf("item subtitle not retained: %+v", got)
	}
	for y := 3; y <= 5; y++ {
		if index, ok := itemHitIndex(a.itemX+8, y, a.itemX, a.itemW, 44, 0, 1); !ok || index != 0 {
			t.Fatalf("click at %d missed item: index=%d, valid=%t", y, index, ok)
		}
	}
	if _, ok := itemHitIndex(a.itemX+8, 6, a.itemX, a.itemW, 44, 0, 1); ok {
		t.Fatal("click after compact row selected an item")
	}
	s.OpenItem()
	c = NewCanvas(120, 44)
	a.mainRender(c)
	if got := canvasAt(c, a.contentX+3, 4); got.R == ' ' {
		t.Fatal("content field name is not visible")
	}
	if got := canvasAt(c, a.contentX+3, 5); got.R == ' ' {
		t.Fatal("content field value is not visible")
	}
}
