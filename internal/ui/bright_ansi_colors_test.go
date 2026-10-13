package ui

import (
	"fmt"
	"strings"
	"testing"
)

// TestBrightANSIColorsUse256Palette ensures that gocui can parse bright ANSI
// colors instead of silently rendering them in the terminal's default white.
func TestBrightANSIColorsUse256Palette(t *testing.T) {
	for code := CBBlack; code <= CBWhite; code++ {
		foreground := fmt.Sprintf("\x1b[38;5;%dm", code)
		background := fmt.Sprintf("\x1b[48;5;%dm", code)
		if got := colorCode(code, false); got != foreground {
			t.Errorf("bright fg %d: got %q want %q", code, got, foreground)
		}
		if got := colorCode(code, true); got != background {
			t.Errorf("bright bg %d: got %q want %q", code, got, background)
		}
	}
	c := NewCanvas(2, 1)
	c.PutBold(0, 0, '\uf005', CBYellow, CDefault)
	got := c.Render()
	if !strings.Contains(got, "\x1b[38;5;11m") {
		t.Errorf("favorite star must use gocui-supported bright yellow SGR: %q", got)
	}
	if strings.Contains(got, "\x1b[93m") {
		t.Errorf("favorite star uses unsupported SGR 93: %q", got)
	}
}
