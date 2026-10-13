package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

func TestStartupWaitsForCredentialChecks(t *testing.T) {
	cli, err := bitwarden.NewClient(nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewState(config.Config{Colors: config.DefaultColors}, cli)
	if s.Screen != AuthChecking || !s.Busy || s.BusyMessage == "" {
		t.Fatal("startup must begin with neutral loading screen")
	}
	a := New(s)
	c := NewCanvas(100, 40)
	a.startupRender(c)
	screen := c.Render()
	if strings.Contains(screen, "Client id") || strings.Contains(screen, "Master password") {
		t.Fatal("initial startup exposed login form prematurely")
	}
	if !strings.Contains(screen, s.BusyMessage) {
		t.Fatal("missing loading message")
	}
	if err := a.char('a'); err != nil {
		t.Fatal(err)
	}
	if s.AuthID != "" {
		t.Fatal("must ignore typing during startup checks")
	}
}

func TestAuthenticationFieldsNavigationHintsAndPlaceholder(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		s.Screen = screen
		s.AuthFocus = 0
		s.Caret = 0
		s.AuthID = ""
		s.Master = ""
		c := NewCanvas(115, 45)
		a.authRender(c)
		layout := authGeometry(115, 45, screen)
		expected := "Client id"
		if screen == AuthMaster {
			expected = "Master password"
		}
		// The placeholder and caret live in different cells. The first letter
		// must remain visible while the field is empty and focused.
		if c.Cells[layout.firstY*c.W+layout.fieldX].R != []rune(expected)[0] {
			t.Fatalf("%v: cursor covers the placeholder's initial letter", screen)
		}
		if c.Cells[layout.firstY*c.W+layout.fieldX].FG != CBlack {
			t.Fatal("placeholder must have normal black foreground")
		}
		if c.Cells[(layout.firstY+1)*c.W+layout.fieldX].FG != CBlue {
			t.Fatal("focused input underline must have normal blue color")
		}
		line := stringCellRow(c, layout.footerY)
		if strings.Contains(line, "←/→") {
			t.Fatal("horizontal arrows advertised on textbox")
		}
		if err := a.arrow(1, 0); err != nil {
			t.Fatal(err)
		}
		if screen == AuthClient {
			_ = a.arrow(1, 0)
		}
		if s.AuthFocus != 2 {
			t.Fatal("navigation did not reach buttons")
		}
		c = NewCanvas(115, 45)
		a.authRender(c)
		if !strings.Contains(stringCellRow(c, layout.footerY), "←/→") {
			t.Fatal("horizontal arrow hint missing on buttons")
		}
		if err := a.arrow(0, 1); err != nil {
			t.Fatal(err)
		}
		if s.AuthFocus != 3 {
			t.Fatal("right arrow did not select QUIT")
		}
	}
}

func stringCellRow(c *Canvas, row int) string {
	var b strings.Builder
	for x := 0; x < c.W; x++ {
		b.WriteRune(c.Cells[row*c.W+x].R)
	}
	return b.String()
}

func TestAuthAppearanceAndCenteredButtonGeometry(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		s.Screen = screen
		s.AuthFocus = 0
		s.Busy = false
		c := NewCanvas(110, 43)
		a.authRender(c)
		geometry := authGeometry(c.W, c.H, screen)
		if geometry.quitX-geometry.enterX-geometry.buttonWidth > 4 || geometry.buttonWidth >= 17 {
			t.Fatal("auth buttons should be narrower and closer")
		}
		if geometry.enterX != c.W-geometry.quitX-geometry.buttonWidth {
			t.Fatal("the two auth buttons need identical lateral margins")
		}
		if c.Cells[(geometry.firstY+1)*c.W+geometry.fieldX].FG != CBlue {
			t.Fatal("focused field underline blue")
		}
		if screen == AuthClient {
			tip := stringCellRow(c, geometry.tipY+1)
			if !strings.Contains(tip, "Hint. Sign in automatically") {
				t.Fatal("missing centered config hint")
			}
		}
		s.Busy = true
		s.BusyMessage = "Authenticating"
		c = NewCanvas(110, 43)
		a.authRender(c)
		if !strings.Contains(c.Render(), "Authenticating") {
			t.Fatal("missing busy popup")
		}
		limitY := geometry.footerY
		if screen == AuthClient {
			limitY = geometry.tipY
		}
		startY := geometry.buttonY + 4
		endY := limitY - 3
		popupY := startY
		if endY >= startY {
			popupY = (startY + endY) / 2
		}
		mid := c.Cells[(popupY+1)*c.W+(c.W/2)]
		if mid.BG != CBlack || popupY <= geometry.buttonY+1 {
			t.Fatal("busy popup should have black background below buttons")
		}
	}
}

func TestCheckboxSwitchIsStagedWithoutEditingDetails(t *testing.T) {
	s := setup(t)
	item := s.SelectedItem()
	if item != nil {
		t.Fatal("test setup should not have an item opened")
	}
	item = s.Items[0]
	item.Raw["fields"] = []any{map[string]any{"name": "Active", "type": 2, "value": "false"}}
	// Baseline includes the checkbox; toggling it must stage a modification.
	s.Originals[item.ID()] = vault.Clone(item.Raw)
	s.OpenItem()
	index := -1
	for n, field := range s.Fields {
		if field.Name == "Active" {
			index = n
			break
		}
	}
	if index < 0 {
		t.Fatal("missing boolean field")
	}
	s.FieldIndex = index
	a := New(s)
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Browse || s.Focus != PaneContent {
		t.Fatal("checkbox opened the details editor")
	}
	if err := a.enter(); err != nil {
        t.Fatal(err)
    }
    if s.Focus != PaneContent || s.Stage.Len() != 1 || s.CurrentField().Value != "true" {
        t.Fatal("ENTER must toggle the checkbox without opening details")
    }
    if err := a.char('s'); err != nil { t.Fatal(err) }
    if s.Stage.Len() != 0 || s.CurrentField().Value != "false" {
        t.Fatal("s switch must restore the original value")
    }
    if err := a.arrow(0, 1); err != nil { t.Fatal(err) }

	if s.Stage.Len() != 1 || s.CurrentField().Value != "true" || s.Focus != PaneContent {
		t.Fatalf("checkbox not toggled/staged: field=%#v", s.CurrentField())
	}
	c := NewCanvas(120, 40)
	a.mainRender(c)
	xx := a.contentX + 6
	yy := 3 + contentRowHeight*index + 2
	if c.Cells[yy*c.W+xx].FG != CGreen {
		t.Fatal("True must use normal green")
	}
	if !strings.Contains(strings.Join(a.hints(), " "), "s switch") {
		t.Fatal("switch keybind missing")
	}
	if err := a.char('s'); err != nil {
		t.Fatal(err)
	}
	if s.Stage.Len() != 0 || s.CurrentField().Value != "false" {
		t.Fatal("original state not restored")
	}
	c = NewCanvas(120, 40)
	a.mainRender(c)
	if c.Cells[yy*c.W+xx].FG != CRed {
		t.Fatal("False must use normal red")
	}
}

func TestCustomCheckboxAndAllVaultIcons(t *testing.T) {
	if customFieldOptions[2].Icon != "\U000F0135" {
		t.Fatal("wrong checkbox icon")
	}
	s := setup(t)
	s.Focus = PaneVault
	s.FilterVault = 1
	s.Filters[0] = 1
	a := New(s)
	c := NewCanvas(120, 40)
	a.mainRender(c)
	if !strings.Contains(c.Render(), "\uedca") {
		t.Fatal("All vaults icon missing")
	}
}
