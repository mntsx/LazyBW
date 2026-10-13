package ui

import (
	"strings"
	"unicode/utf8"
)

// visibleText shows a window containing the caret without inserting cells.
func visibleText(text string, caret, width int) (string, int) {
	if width <= 0 {
		return "", 0
	}
	runes := []rune(text)
	caret = max(0, min(caret, len(runes)))
	start := max(0, caret-width+1)
	end := min(len(runes), start+width)
	return string(runes[start:end]), caret - start
}

// editCaretPoint maps the active textbox caret to the canvas coordinates.
func (a *App) editCaretPoint(width, height int) (int, int, bool) {
	s := a.S
	if s.Popup != nil || s.Mode == DialogServer || s.Screen == AuthChecking || s.Busy {
		return 0, 0, false
	}
	if s.Screen != Workspace {
		if s.AuthFocus >= 2 {
			return 0, 0, false
		}
		g := authGeometry(width, height, s.Screen)
		line := g.firstY
		if s.Screen == AuthClient && s.AuthFocus == 1 {
			line = g.secondY
		}
		value := a.authValue()
		if value == "" {
			return g.fieldX - 1, line, true
		}
		_, col := visibleText(value, s.Caret, g.fieldWidth)
		return g.fieldX + col, line, true
	}
	if s.Mode == DialogReprompt {
		w := min(66, width-4)
		x, y := (width-w)/2, max(2, (height-12)/2)
		value := s.MasterText()
		if value == "" {
			return x + 3, y + 5, true
		}
		_, col := visibleText(value, s.Caret, w-8)
		return x + 4 + col, y + 5, true
	}
	switch s.Mode {
	case Find:
		if a.itemW < 5 {
			return 0, 0, false
		}
		if s.Search == "" {
			return a.itemX + 1, height - 4, true
		}
		_, col := visibleText(s.Search, s.Caret, a.itemW-4)
		return a.itemX + 2 + col, height - 4, true
	case DialogName, DialogField, DialogFolder:
		if s.FormFocus != 0 {
			return 0, 0, false
		}
		w := min(65, width-6)
		x := (width-w)/2 + 4
		y := max(2, (height-16)/2) + 5
		if s.Input == "" {
			return x - 1, y, true
		}
		_, col := visibleText(s.Input, s.Caret, w-8)
		return x + col, y, true
	case Edit:
		if a.detailH < 5 {
			return 0, 0, false
		}
		value := s.Input
		caret := max(0, min(s.Caret, utf8.RuneCountInString(value)))
		line, col := 0, 0
		for j, r := range []rune(value) {
			if j >= caret {
				break
			}
			if r == '\n' {
				line++
				col = 0
			} else {
				col++
			}
		}
		lines := strings.Split(value, "\n")
		startLine := max(0, line-(a.detailH-4))
		if line >= len(lines) {
			return 0, 0, false
		}
		_, visibleCol := visibleText(lines[line], col, a.contentW-4)
		return a.detailX + 2 + visibleCol, a.detailY + 3 + line - startLine, true
	}
	return 0, 0, false
}
