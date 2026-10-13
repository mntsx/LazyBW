package ui

import (
	"strings"
	"testing"
)

// TestAuthTitlesFollowUpperInput checks left alignment on both auth forms.
func TestAuthTitlesFollowUpperInput(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		for _, size := range [][2]int{{80, 35}, {111, 43}, {100, 25}} {
			s := setup(t)
			s.Screen = screen
			s.Busy = false
			a := New(s)
			w, h := size[0], size[1]
			layout := authGeometry(w, h, screen)
			c := NewCanvas(w, h)
			a.authRender(c)

			title := "Bitwarden API credentials"
			if screen == AuthMaster {
				title = "Unlock Bitwarden"
			}
			title = minWidth(title, layout.fieldWidth)
			start := layout.fieldX
			y := max(1, layout.firstY-2)
			for i, letter := range []rune(title) {
				got := canvasAt(c, start+i, y)
				if got.R != letter || got.FG != CBlue {
					t.Fatalf("screen=%v size=%v x=%d y=%d: title mismatch: %+v",
						screen, size, start+i, y, got)
				}
			}
			if y >= layout.firstY {
				t.Fatal("title overlaps upper input")
			}
		}
	}
}

// TestAuthButtonsEndAtTheirColoredBaseline checks the two-row auth button.
func TestAuthButtonsEndAtTheirColoredBaseline(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		for _, focused := range []int{0, 2, 3} {
			s := setup(t)
			s.Screen = screen
			s.AuthFocus = focused
			s.Busy = false
			a := New(s)
			c := NewCanvas(110, 43)
			a.authRender(c)
			g := authGeometry(c.W, c.H, screen)
			for _, button := range []struct {
				x, accent, focus int
			}{{g.enterX, CGreen, 2}, {g.quitX, CRed, 3}} {
				background := CBlack
				if focused == button.focus {
					background = button.accent
				}
				for x := button.x; x < button.x+g.buttonWidth; x++ {
					top := canvasAt(c, x, g.buttonY-1)
					textRow := canvasAt(c, x, g.buttonY)
					baseline := canvasAt(c, x, g.buttonY+1)
					below := canvasAt(c, x, g.buttonY+2)
					if top.R != '▂' || top.FG != background || top.BG != CDefault {
						t.Fatalf("screen=%v focus=%d x=%d: upper quarter=%+v",
							screen, focused, x, top)
					}
					if textRow.BG != background || baseline.BG != background ||
						baseline.FG != button.accent || baseline.R != '▃' {
						t.Fatalf("screen=%v focus=%d x=%d: text=%+v baseline=%+v",
							screen, focused, x, textRow, baseline)
					}
					if below.BG != CDefault || below.R != ' ' {
						t.Fatalf("button selection extends below colored line: %+v", below)
					}
				}
			}
		}
	}
}

// TestAuthButtonLabelsAreVerticallyCentered checks optical centering in two rows.
func TestAuthButtonLabelsAreVerticallyCentered(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		for _, focus := range []int{0, 2, 3} {
			s := setup(t)
			s.Screen, s.AuthFocus, s.Busy = screen, focus, false
			a := New(s)
			c := NewCanvas(100, 35)
			a.authRender(c)
			g := authGeometry(c.W, c.H, screen)
			for _, button := range []struct {
				x     int
				label string
			}{{g.enterX, "AUTH"}, {g.quitX, "QUIT"}} {
				labelX := button.x + (g.buttonWidth-len(button.label))/2
				for i, char := range button.label {
					if got := canvasAt(c, labelX+i, g.buttonY).R; got != char {
						t.Fatalf("screen=%v focus=%d: centered label %q[%d] = %q",
							screen, focus, button.label, i, got)
					}
				}
				if edge := canvasAt(c, labelX, g.buttonY+1); edge.R != '▃' {
					t.Fatalf("the flush accent must run under the entire label: %+v", edge)
				}
			}
		}
	}
}

// TestCompactAuthButtonsDoNotOverlapBusyHint checks the short client form.
func TestCompactAuthButtonsDoNotOverlapBusyHint(t *testing.T) {
	s := setup(t)
	s.Screen, s.Busy, s.BusyMessage = AuthClient, true, "Authenticating"
	a := New(s)
	c := NewCanvas(100, 25)
	a.authRender(c)
	g := authGeometry(c.W, c.H, AuthClient)
	if g.buttonY+1 >= g.tipY || g.buttonY+4 > g.tipY-3 {
		t.Fatalf("buttons leave no room for the spinner: %+v", g)
	}
	if got := canvasAt(c, c.W/2, g.tipY).BG; got != CBlack {
		t.Fatalf("API information box must remain visible: %d", got)
	}
}

// TestClientIDIsOneRowCloserToClientSecret verifies the compact client form.
func TestClientIDIsOneRowCloserToClientSecret(t *testing.T) {
	for _, size := range [][2]int{{80, 25}, {100, 35}, {110, 43}} {
		w, h := size[0], size[1]
		g := authGeometry(w, h, AuthClient)
		spacing := 4
		if h < 26 {
			spacing = 3
		}
		if got := g.secondY - g.firstY; got != spacing-1 {
			t.Fatalf("size=%v: expected %d rows between fields, got %d",
				size, spacing-1, got)
		}
		if g.firstY+1 >= g.secondY {
			t.Fatalf("size=%v: Client ID underline overlaps Client secret", size)
		}
		s := setup(t)
		s.Screen, s.Busy = AuthClient, false
		c := NewCanvas(w, h)
		New(s).authRender(c)
		if !strings.Contains(stringCellRow(c, g.firstY-2),
			"Bitwarden API credentials") {
			t.Fatalf("size=%v: heading should remain two rows above Client ID", size)
		}
		if !strings.Contains(stringCellRow(c, g.firstY), "Client id") {
			t.Fatalf("size=%v: Client ID is not on the new row", size)
		}
		if !strings.Contains(stringCellRow(c, g.secondY), "Client secret") {
			t.Fatalf("size=%v: Client secret moved or disappeared", size)
		}
	}
}
