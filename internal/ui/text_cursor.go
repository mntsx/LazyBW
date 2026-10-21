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

// detailDisplayLines produces the editor viewport rows. In unwrapped mode every
// explicit line shares one horizontal origin, so moving through a long line
// scrolls the whole textbox as a single block.
func detailDisplayLines(value string, caret, width int, wrap bool) ([]string, int, int) {
	if width <= 0 {
		return nil, 0, 0
	}
	logical := strings.Split(value, "\n")
	caret = max(0, min(caret, utf8.RuneCountInString(value)))
	logicalLine, logicalCol := 0, 0
	for i, r := range []rune(value) {
		if i >= caret {
			break
		}
		if r == '\n' {
			logicalLine++
			logicalCol = 0
		} else {
			logicalCol++
		}
	}
	if !wrap {
		start := max(0, logicalCol-width+1)
		rows := make([]string, len(logical))
		for i, line := range logical {
			runes := []rune(line)
			from := min(start, len(runes))
			rows[i] = string(runes[from:min(len(runes), start+width)])
		}
		return rows, logicalLine, logicalCol - start
	}

	rows := []string{}
	cursorRow, cursorCol := 0, 0
	for i, line := range logical {
		runes := []rune(line)
		chunks := max(1, (len(runes)+width)/width)
		if i == logicalLine {
			chunk := min(logicalCol/width, chunks-1)
			cursorRow = len(rows) + chunk
			cursorCol = logicalCol - chunk*width
		}
		for chunk := 0; chunk < chunks; chunk++ {
			from := chunk * width
			rows = append(rows, string(runes[from:min(len(runes), from+width)]))
		}
	}
	return rows, cursorRow, cursorCol
}

// detailDisplayOffsets maps each displayed rune to its original input index.
func detailDisplayOffsets(value string, caret, width int, wrap bool) [][]int {
	if width <= 0 { return nil }
	logical := strings.Split(value, "\n")
	caret = max(0, min(caret, utf8.RuneCountInString(value)))
	logicalCol := 0
	for i, char := range []rune(value) {
		if i >= caret { break }
		if char == '\n' { logicalCol = 0 } else { logicalCol++ }
	}
	offsets := make([][]int, 0, len(logical))
	lineStart, horizontalStart := 0, 0
	if !wrap { horizontalStart = max(0, logicalCol-width+1) }
	for i, line := range logical {
		runes := []rune(line)
		if !wrap {
			from := min(horizontalStart, len(runes))
			row := make([]int, len(runes[from:min(len(runes), horizontalStart+width)]))
			for j := range row { row[j] = lineStart + from + j }
			offsets = append(offsets, row)
		} else {
			chunks := max(1, (len(runes)+width)/width)
			for chunk := 0; chunk < chunks; chunk++ {
				from := chunk * width
				to := min(len(runes), from+width)
				row := make([]int, to-from)
				for j := range row { row[j] = lineStart + from + j }
				offsets = append(offsets, row)
			}
		}
		lineStart += len(runes)
		if i < len(logical)-1 { lineStart++ }
	}
	return offsets
}

// editCaretPoint maps the active textbox caret to the canvas coordinates.
func (a *App) editCaretPoint(width, height int) (int, int, bool) {
	s := a.S
	if s.Popup != nil || s.Mode == DialogServer || s.Mode == DialogBackend || s.Screen == AuthChecking || s.Busy {
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
		if a.detailH < 4 || a.detailW < 5 {
			return 0, 0, false
		}
		_, line, col := detailDisplayLines(s.Input, s.Caret, a.detailW-4, s.InputWrap)
		startLine := max(0, line-(a.detailH-4))
		return a.detailX + 2 + col, a.detailY + 3 + line - startLine, true
	}
	return 0, 0, false
}
