package ui

import (
	"context"
	"github.com/miguelmnts/lazybw-go/internal/vault"
	"testing"
	"time"
)

// TestStagedItemNameStaysWhite checks that State remains the sole stage color.
func TestStagedItemNameStaysWhite(t *testing.T) {
	s := setup(t)
	it := s.CurrentItem()
	if err := it.Set([]any{"login", "username"}, "changed"); err != nil {
		t.Fatal(err)
	}
	s.Stage.Record("item", it.Raw, vault.TypeNames[it.Type()], s.Originals)
	s.Focus = PaneItems
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)

	name := canvasAt(c, a.itemX+8, 4)
	if name.R != 'E' || name.FG != CWhite || !name.Bold {
		t.Fatalf("staged name should stay bold white: %+v", name)
	}
	// 'M' still appears in the State column in the stage color.
	found := false
	for x := a.itemX + 1; x < a.itemX+a.itemW; x++ {
		cell := canvasAt(c, x, 4)
		if cell.R == 'M' && cell.FG == CYellow {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("modified state marker must remain yellow")
	}
}

// TestFindMagentaAndMatchText keeps the query and matching letters in sync.
func TestFindMagentaAndMatchText(t *testing.T) {
	s := setup(t)
	s.Search, s.Input, s.Mode, s.Focus = "Ex", "Ex", Find, PaneItems
	a := New(s)
	for _, blink := range []bool{false, true} {
		s.Blink = blink
		c := NewCanvas(120, 44)
		a.mainRender(c)
		findY := c.H - 5
		for _, x := range []int{a.itemX, a.itemX + 5, a.itemX + a.itemW - 1} {
			if cell := canvasAt(c, x, findY); cell.FG != CMagenta {
				t.Fatalf("Find rule should not blink: %+v", cell)
			}
		}
		for _, x := range []int{a.itemX + 2, a.itemX + 3} {
			if cell := canvasAt(c, x, findY+1); cell.FG != CMagenta {
				t.Fatalf("Find text should be magenta: %+v", cell)
			}
		}
		for _, x := range []int{a.itemX + 8, a.itemX + 9} {
			if cell := canvasAt(c, x, 4); cell.FG != CMagenta || !cell.Bold {
				t.Fatalf("matched name letters should be bold magenta: %+v", cell)
			}
		}
		if cell := canvasAt(c, a.itemX+10, 4); cell.FG != CWhite {
			t.Fatalf("unmatched name letters must remain white: %+v", cell)
		}
		if cell := canvasAt(c, a.itemX+8, 5); cell.FG != CSubtitle {
			t.Fatalf("subtitle changed color: %+v", cell)
		}
	}
	s.Search, s.Input = "", ""
	c := NewCanvas(120, 44)
	a.mainRender(c)
	if cell := canvasAt(c, a.itemX+2, c.H-4); cell.R != 'F' || cell.FG != CMagenta {
		t.Fatalf("Find placeholder should be magenta: %+v", cell)
	}
}

// TestTitleRulesFollowFocus tests each pane including stacked Content/Detail.
func TestTitleRulesFollowFocus(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, pane := range []Pane{PaneVault, PaneTypes, PaneFolders, PaneChanges, PaneItems} {
		s.Focus = pane
		c := NewCanvas(120, 44)
		a.mainRender(c)
		for index, panel := range []Pane{PaneVault, PaneTypes, PaneFolders, PaneChanges} {
			if a.sideH[index] < 2 {
				continue // a collapsed panel has no content separator
			}
			want := CBlue
			if pane == panel {
				want = CYellow
			}
			if cell := canvasAt(c, 8, a.sideY[index]+1); cell.FG != want {
				t.Fatalf("focus=%d side=%d divider=%+v want %d", pane, panel, cell, want)
			}
		}
		want := CBlue
		if pane == PaneItems {
			want = CYellow
		}
		if cell := canvasAt(c, a.itemX+8, 2); cell.FG != want {
			t.Fatalf("focus=%d Items divider=%+v want %d", pane, cell, want)
		}
	}
	s.OpenItem()
	c := NewCanvas(120, 44)
	a.mainRender(c)
	if cell := canvasAt(c, a.contentX+8, 2); cell.FG != CYellow {
		t.Fatalf("Content divider should be yellow when focused: %+v", cell)
	}
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	c = NewCanvas(120, 44)
	a.mainRender(c)
	if cell := canvasAt(c, a.contentX+8, 2); cell.FG != CBlue {
		t.Fatalf("Content is not focused during Edit: %+v", cell)
	}
	if cell := canvasAt(c, a.detailX+8, a.detailY+1); cell.FG != CYellow {
		t.Fatalf("Detail divider should be yellow during Edit: %+v", cell)
	}
}

// TestAuthQuitMouseReturnsPromptly covers QUIT in both authentication screens.
func TestAuthQuitMouseReturnsPromptly(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		t.Run(map[Screen]string{AuthClient: "client", AuthMaster: "master"}[screen],
			func(t *testing.T) {
				s := setup(t)
				s.Screen = screen
				a := New(s)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				a.cancel = cancel
				const width, height = 120, 40
				geometry := authGeometry(width, height, screen)
				finished := make(chan error, 1)
				go func() {
					finished <- a.authClickAt(geometry.quitX+1, geometry.buttonY,
						width, height)
				}()
				select {
				case err := <-finished:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("QUIT click blocked the UI")
				}
				if s.AuthFocus != 3 || ctx.Err() != context.Canceled {
					t.Fatal("QUIT click did not request application shutdown")
				}
			})
	}
}

// TestAuthOtherButtonsAndInputsDoNotQuit checks shared auth hit-testing.
func TestAuthOtherButtonsAndInputsDoNotQuit(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		s := setup(t)
		s.Screen = screen
		s.Busy = true // Avoid starting a real Bitwarden subprocess on AUTH.
		a := New(s)
		const width, height = 120, 40
		g := authGeometry(width, height, screen)
		for _, tc := range []struct {
			x, y, focus int
		}{
			{g.enterX + 1, g.buttonY, 2},
			{g.fieldX + 1, g.firstY, 0},
		} {
			if err := a.authClickAt(tc.x, tc.y, width, height); err != nil {
				t.Fatal(err)
			}
			if s.AuthFocus != tc.focus {
				t.Fatalf("auth screen=%d clicked (%d,%d): focus=%d, want %d",
					screen, tc.x, tc.y, s.AuthFocus, tc.focus)
			}
		}
	}
}
