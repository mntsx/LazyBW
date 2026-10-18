package ui

import (
	"fmt"
	"github.com/miguelmnts/lazybw-go/internal/vault"
	"strings"
	"time"
	"unicode/utf8"
)

// A single in-memory cell grid lets gocui display contiguous panels without
// the padding, frame constraints, or widget lifecycle costs of Textual.
type Cell struct {
	R      rune
	FG, BG int
	Bold   bool
	Dim    bool
}
type Canvas struct {
	W, H  int
	Cells []Cell
	remap map[int]int
}

const (
	CDefault  = -1
	CBlack    = 0
	CRed      = 1
	CGreen    = 2
	CYellow   = 3
	CBlue     = 4
	CMagenta  = 5
	CCyan     = 6
	CWhite    = 7
	CBBlack   = 8
	CBRed     = 9
	CBGreen   = 10
	CBYellow  = 11
	CBBlue    = 12
	CBMagenta = 13
	CBCyan    = 14
	CBWhite   = 15
	CTitle    = 17
	CFindBG   = 236
	// Fixed grayscale palette entries used by keybinding captions. The key
	// is deliberately lighter than its associated action.
	CHintKey    = 250
	CHintAction = 244
	CSubtitle   = 249 // dim white, independent of item focus
)

// A half-block draws one solid, visually thick vertical marker in a single
// terminal cell. Multiple box-drawing columns would produce parallel lines.
const (
	selectionStripeWidth = 1
	// The stripe is anchored at the original panel position; the visual
	// notch was caused by its quarter-cell padding glyphs, not its x offset.
	selectionStripeInset = 0
	selectionStripeGlyph = '▐'
	// Items and Content use full-cell width with half-height end caps.
	// Sidebar indicators continue to use the narrower selectionStripeGlyph.
	selectionStripeCenterMask = '█'
	itemRowHeight             = 3 // two text rows plus one shared half-and-half padding row
	contentRowHeight          = 3 // two text rows plus one shared half-and-half padding row
)

// The stripe has its own column. It touches the selected background at its
// right edge, so the background remains perfectly rectangular while the
// colored stripe can start/end on a HALF cell without a three-color glyph.
// A terminal cell cannot independently encode the unselected background,
// selected background, and stripe color in its two ANSI color channels.
func drawSelectionStripe(c *Canvas, x, y, height, color, _ int) {
	for dy := 0; dy < height; dy++ {
		c.Put(x+selectionStripeInset, y+dy, selectionStripeGlyph, color, CDefault)
	}
}

// drawBalancedSelectionStripe draws full-width strokes for Items and Content.
// Foreground color avoids rectangular artifacts from BG masks.
func drawBalancedSelectionStripe(c *Canvas, x, y, height, color int) {
	for dy := 0; dy < height; dy++ {
		c.Put(x+selectionStripeInset, y+dy, selectionStripeCenterMask, color, CDefault)
	}
}

// drawSharedVerticalPadding assigns the upper half of one terminal row to
// the previous element and the lower half to the next element. The background
// is painted consistently from the column immediately after the stripe.
// Items and Content use full-width blocks with half-height end caps.
// Sidebar filter markers are drawn separately and remain half-width.
func drawSharedVerticalPadding(
	c *Canvas, x, y, width, previousBG, currentBG, previousStripe, currentStripe int,
) {
	for xx := x + selectionStripeInset + selectionStripeWidth; xx < x+width; xx++ {
		switch {
		case previousBG == currentBG:
			c.Put(xx, y, ' ', CWhite, currentBG)
		case previousBG == CDefault:
			c.Put(xx, y, '▄', currentBG, CDefault)
		case currentBG == CDefault:
			c.Put(xx, y, '▀', previousBG, CDefault)
		default:
			c.Put(xx, y, '▄', currentBG, previousBG)
		}
	}
	switch {
	case previousStripe >= 0 && currentStripe >= 0 && previousStripe == currentStripe:
		c.Put(x+selectionStripeInset, y, selectionStripeCenterMask, currentStripe, CDefault)
	case currentStripe >= 0:
		// Upper edge: stripe begins at the lower half of this cell.
		c.Put(x+selectionStripeInset, y, '▄', currentStripe, CDefault)
	case previousStripe >= 0:
		// Lower edge: stripe ends at the upper half of this cell.
		c.Put(x+selectionStripeInset, y, '▀', previousStripe, CDefault)
	default:
		c.Put(x+selectionStripeInset, y, ' ', CWhite, CDefault)
	}
}

func stageKindColor(kind vault.Kind) int {
	switch kind {
	case vault.New:
		return CGreen
	case vault.Modified:
		return CYellow
	case vault.Deleted:
		return CRed
	default:
		return CWhite
	}
}

func NewCanvas(w, h int) *Canvas {
	c := &Canvas{W: w, H: h, Cells: make([]Cell, w*h)}
	for i := range c.Cells {
		c.Cells[i] = Cell{R: ' ', FG: CWhite, BG: CDefault}
	}
	return c
}
func (c *Canvas) Put(x, y int, r rune, fg, bg int) {
	if x >= 0 && x < c.W && y >= 0 && y < c.H {
		if r == 0 {
			r = ' '
		}
		if mapped, ok := c.remap[fg]; ok {
			fg = mapped
		}
		if mapped, ok := c.remap[bg]; ok {
			bg = mapped
		}
		c.Cells[y*c.W+x] = Cell{R: r, FG: fg, BG: bg}
	}
}

// PutBold is used only for white panel borders. Bold thin box glyphs
// appear between the normal blue rules and the former heavy white rules
// on terminals that support bold line-drawing characters.
// TextBold draws a string without changing its existing terminal colors.
func (c *Canvas) TextBold(x, y int, s string, fg, bg int) {
	for _, r := range s {
		if r == '\n' {
			break
		}
		c.PutBold(x, y, r, fg, bg)
		x++
	}
}

// TextDim uses the terminal's dim attribute, not a hard-coded grey color.
func (c *Canvas) TextDim(x, y int, s string, fg, bg int) {
	for _, r := range s {
		if r == '\n' {
			break
		}
		c.Put(x, y, r, fg, bg)
		if x >= 0 && x < c.W && y >= 0 && y < c.H {
			c.Cells[y*c.W+x].Dim = true
		}
		x++
	}
}
func (c *Canvas) PutBold(x, y int, r rune, fg, bg int) {
	c.Put(x, y, r, fg, bg)
	if x >= 0 && x < c.W && y >= 0 && y < c.H {
		c.Cells[y*c.W+x].Bold = true
	}
}
func (c *Canvas) Fill(x, y, w, h int, r rune, fg, bg int) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			c.Put(xx, yy, r, fg, bg)
		}
	}
}
func (c *Canvas) Text(x, y int, s string, fg, bg int) {
	for _, r := range s {
		if r == '\n' {
			break
		}
		c.Put(x, y, r, fg, bg)
		x++
	}
}
func (c *Canvas) Line(x, y, w int, r rune, fg, bg int) { c.Fill(x, y, w, 1, r, fg, bg) }
func (c *Canvas) Render() string {
	var b strings.Builder
	lastFG, lastBG, lastBold, lastDim := 999, 999, false, false
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			cell := c.Cells[y*c.W+x]
			if cell.FG != lastFG || cell.BG != lastBG || cell.Bold != lastBold ||
				cell.Dim != lastDim {
				b.WriteString("\x1b[0m")
				if cell.FG >= 0 {
					b.WriteString(colorCode(cell.FG, false))
				}
				if cell.BG >= 0 {
					b.WriteString(colorCode(cell.BG, true))
				}
				// awesome-gocui overwrites SGR attributes when parsing a
				// subsequent color. Emit weight AFTER foreground/background.
				if cell.Bold {
					b.WriteString("\x1b[1m")
				} else if cell.Dim {
					b.WriteString("\x1b[2m")
				}
				lastFG, lastBG, lastBold, lastDim = cell.FG, cell.BG, cell.Bold, cell.Dim
			}
			b.WriteRune(cell.R)
		}
		if y < c.H-1 {
			b.WriteString("\x1b[0m\n")
			lastFG, lastBG, lastBold, lastDim = 999, 999, false, false
		}
	}
	b.WriteString("\x1b[0m")
	return b.String()
}
func colorCode(v int, bg bool) string {
	if v >= 1<<24 {
		rgb := v & 0xffffff
		if bg {
			return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", (rgb>>16)&255, (rgb>>8)&255, rgb&255)
		}
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", (rgb>>16)&255, (rgb>>8)&255, rgb&255)
	}
	if v >= 16 {
		if bg {
			return fmt.Sprintf("\x1b[48;5;%dm", v)
		}
		return fmt.Sprintf("\x1b[38;5;%dm", v)
	}
	// awesome-gocui v1.1.0 does not parse SGR 90-97 / 100-107 as
	// bright ANSI colors. Its 256-color parser does support indices 8-15,
	// which use the terminal's configured bright palette (including yellow).
	if v >= 8 {
		if bg {
			return fmt.Sprintf("\x1b[48;5;%dm", v)
		}
		return fmt.Sprintf("\x1b[38;5;%dm", v)
	}
	if bg {
		return fmt.Sprintf("\x1b[%dm", 40+v)
	}
	return fmt.Sprintf("\x1b[%dm", 30+v)
}
func minWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s
}

// borderSection is a horizontal separator between stacked content panels.
type borderSection struct {
	x0, x1, y int
}

// drawMainBorders combines the outer frame, panel dividers and footer.
// Bold light box-drawing characters provide an intermediate visual weight
// between the original heavy white rules and the regular blue title rules.
// A negative footerY requests just the outer frame (authentication screens).
func drawMainBorders(c *Canvas, separators []int, sideRows []int, footerY int, sections ...borderSection) {
	const (
		left = 1 << iota
		right
		up
		down
	)
	masks := make([]uint8, c.W*c.H)
	lineH := func(x0, x1, y int) {
		if y < 0 || y >= c.H {
			return
		}
		for x := max(0, x0); x <= min(c.W-1, x1); x++ {
			if x > x0 {
				masks[y*c.W+x] |= left
			}
			if x < x1 {
				masks[y*c.W+x] |= right
			}
		}
	}
	lineV := func(x, y0, y1 int) {
		if x < 0 || x >= c.W {
			return
		}
		for y := max(0, y0); y <= min(c.H-1, y1); y++ {
			if y > y0 {
				masks[y*c.W+x] |= up
			}
			if y < y1 {
				masks[y*c.W+x] |= down
			}
		}
	}
	lineH(0, c.W-1, 0)
	lineH(0, c.W-1, c.H-1)
	if footerY >= 0 {
		lineH(0, c.W-1, footerY)
	}
	lineV(0, 0, c.H-1)
	lineV(c.W-1, 0, c.H-1)
	for _, x := range separators {
		lineV(x, 0, footerY)
	}
	for _, section := range sections {
		lineH(section.x0, section.x1, section.y)
	}
	if len(separators) > 0 {
		for _, y := range sideRows {
			lineH(0, separators[0], y)
		}
	}
	chars := map[uint8]rune{
		left | right: '━', up | down: '┃',
		right | down: '┏', left | down: '┓',
		right | up: '┗', left | up: '┛',
		left | right | down: '┳', left | right | up: '┻',
		right | up | down: '┣', left | up | down: '┫',
		left | right | up | down: '╋',
	}
	for y := range c.H {
		for x := range c.W {
			if r, ok := chars[masks[y*c.W+x]]; ok {
				c.PutBold(x, y, r, CWhite, CDefault)
			}
		}
	}
}

func (a *App) mainRender(c *Canvas) {
	s := a.S
	w, h := c.W, c.H
	a.renderWidth, a.renderHeight = w, h
	if w < 65 || h < 19 {
		drawMainBorders(c, nil, nil, h-3)
		c.Text(1, 1, "Resize terminal to at least 65 × 19", CYellow, CDefault)
		return
	}
	// The offline workspace contains only item types and folders.
	panelX, panelY := 1, 1
	interiorW := w - 2
	left := max(25, min(39, interiorW*27/100))
	topH := h - 4
	a.sideY = [4]int{}
	a.sideH = [4]int{}
	selLeft := s.Focus >= PaneVault && s.Focus <= PaneChanges
	if s.LocalMode {
		selLeft = s.Focus == PaneTypes || s.Focus == PaneFolders
	}
	panels := []struct {
		name, icon string
		pane       Pane
	}{
		{"Vault", vault.IconFilter, PaneVault},
		{"Items", vault.IconFilter, PaneTypes},
		{"Folders", vault.IconFilter, PaneFolders},
		{"Edits", vault.IconChanges, PaneChanges},
	}
	if s.LocalMode {
		panels = panels[1:3]
	}
	avail := topH - len(panels) + 1
	heights := make([]int, len(panels))
	if selLeft && !s.LocalMode {
		// Restore the compact online sidebar: only the focused filter expands.
		// Local mode intentionally keeps its two sections at fixed heights.
		minimum := []int{3, 3, 2, 1}
		if s.FilterFolder >= 0 {
			minimum[2] = 3
		}
		copy(heights, minimum)
		remaining := avail
		for _, height := range heights {
			remaining -= height
		}
		for i, p := range panels {
			if p.pane == s.Focus {
				heights[i] += max(0, remaining)
				break
			}
		}
	} else {
		for i := range heights {
			heights[i] = avail / len(panels)
			if i < avail%len(panels) {
				heights[i]++
			}
		}
	}
	row := panelY
	sideSeparators := []int{}
	for index, p := range panels {
		ph := heights[index]
		if ph < 1 {
			continue
		}
		sideIndex := int(p.pane) - 1
		a.sideY[sideIndex] = row
		a.sideH[sideIndex] = ph
		a.leftWidth = panelX + left
		a.sidePanel(c, p.name, p.icon, p.pane, panelX, row, left, ph)
		row += ph
		if index+1 < len(panels) {
			sideSeparators = append(sideSeparators, row)
			row++
		}
	}
	leftSeparator := panelX + left
	separators := []int{leftSeparator}
	middleStart := leftSeparator + 1
	end := w - 1
	contentX := 0
	if s.SelectedID != "" && s.Focus >= PaneContent {
		contentWidth := max(23, (interiorW-left-1)*42/100)
		contentX = w - 1 - contentWidth
		separators = append(separators, contentX-1)
		end = contentX - 1
	}
	a.itemX, a.itemW = middleStart, max(0, end-middleStart)
	a.contentX, a.contentW = contentX, 0
	a.detailX, a.detailY, a.detailH = 0, 0, 0
	a.itemsPanel(c, middleStart, panelY, a.itemW, topH)
	var sections []borderSection
	if contentX > 0 {
		contentWidth := w - contentX - 1
		a.contentW = contentWidth
		if s.Mode == Edit && !s.LocalMode {
			// Details occupies the bottom of the very same right-hand column.
			// Opening the editor never changes the width of Items or Content.
			detailH := max(5, topH*35/100)
			contentH := topH - detailH - 1
			// Keep the selected field visible when enough rows exist.
			needed := 4
			for index := 0; index <= s.FieldIndex && index < len(s.Fields); index++ {
				needed += fieldRowHeight(s.Fields[index])
			}
			if needed > contentH && topH-needed-1 >= 5 {
				contentH = needed
				detailH = topH - contentH - 1
			}
			a.detailX = contentX
			a.detailY = panelY + contentH + 1
			a.detailH = detailH
			a.contentPanel(c, contentX, panelY, contentWidth, contentH)
			a.detailPanel(c, contentX, a.detailY, contentWidth, detailH)
			sections = append(sections, borderSection{
				x0: contentX - 1, x1: w - 1, y: a.detailY - 1,
			})
		} else {
			a.contentPanel(c, contentX, panelY, contentWidth, topH)
		}
	}
	a.footer(c, panelX, h-2, interiorW)
	drawMainBorders(c, separators, sideSeparators, h-3, sections...)
}
func (a *App) header(c *Canvas, x, y, w int, name, icon string, pane Pane) {
	if w < 1 {
		return
	}
	c.Fill(x, y, w, 1, ' ', CWhite, CTitle)
	active := a.S.Focus == pane
	visibleNumber := panelNumber(a.S, pane)
	number := fmt.Sprintf("[%d]", visibleNumber)
	c.Text(x+1, y, number, CWhite, CTitle)
	if active {
		color := CTitle // off frame of the pulse: invisible against the title bar
		if a.S.Blink {
			color = CYellow
		}
		c.Text(x+2, y, fmt.Sprintf("%d", visibleNumber), color, CTitle)
	}
	c.Text(x+5, y, icon, CBlue, CTitle)
	c.Text(x+7, y, minWidth(name, w-8), CWhite, CTitle)
}

// panelRuleColor highlights only the panel currently receiving navigation.
// Find has its own magenta separator instead of activating the Items rule.
func panelRuleColor(active bool) int {
	if active {
		return CYellow
	}
	return CBlue
}

func (a *App) sidePanel(c *Canvas, name, icon string, pane Pane, x, y, w, h int) {
	s := a.S
	a.header(c, x, y, w, name, icon, pane)
	if h < 2 {
		return
	}
	c.Line(x, y+1, w, '━', panelRuleColor(s.Focus == pane), CDefault)
	if h < 3 {
		return
	}
	var opts []string
	var glyphs []string
	folderMarkers := map[int]vault.Kind{}
	deletedFolderRows := map[int]bool{}
	selected := 0
	applied := -1
	switch pane {
	case PaneVault:
		opts = []string{"All vaults", "My vault"}
		glyphs = []string{"\uedca", "\uf007"}
		selected = 1 - s.Filters[0]
		applied = 1 - s.FilterVault
	case PaneTypes:
		opts = []string{"All items", "Favourites", "Login", "Card", "Identity", "Secure note", "SSH key"}
		glyphs = []string{vault.IconAll, "\U000f04ce", vault.TypeIcons[1], vault.TypeIcons[3], vault.TypeIcons[4], vault.TypeIcons[2], vault.TypeIcons[5]}
		selected = s.Filters[1]
		applied = s.FilterType
	case PaneFolders:
		opts = []string{"No Folder"}
		glyphs = []string{"\U000F19F9"}
		for _, f := range s.Folders {
			index := len(opts)
			opts = append(opts, fmt.Sprint(f["name"]))
			glyphs = append(glyphs, vault.IconFolder)
			if change := s.Stage.Get(vault.Key("folder", fmt.Sprint(f["id"]))); change != nil {
				folderMarkers[index] = change.Kind
			}
		}
		// Deleted folders remain visible as read-only pending deletion rows.
		for _, change := range s.Stage.Changes() {
			if change.Entity == "folder" && change.Kind == vault.Deleted {
				index := len(opts)
				opts = append(opts, change.Name)
				glyphs = append(glyphs, vault.IconFolder)
				folderMarkers[index] = vault.Deleted
				deletedFolderRows[index] = true
			}
		}
		selected = s.Filters[2]
		applied = s.FilterFolder
	case PaneChanges:
		for _, v := range s.ChangesList() {
			opts = append(opts, v.Name)
			if v.Entity == "folder" {
				glyphs = append(glyphs, vault.IconFolder)
			} else {
				code := 1
				for t, label := range vault.TypeNames {
					if label == v.TypeName {
						code = t
						break
					}
				}
				glyphs = append(glyphs, vault.TypeIcons[code])
			}
		}
		selected = s.ChangeIndex
	}
	visible := h - 2
	if h >= 5 {
		visible-- // leave an empty row beneath the last visible option
	}
	offset := 0
	if pane != s.Focus && applied >= 0 && visible == 1 {
		offset = applied
	} else if selected >= visible {
		offset = selected - visible + 1
	}
	a.sideScroll[int(pane)-1] = offset
	for i := offset; i < len(opts) && i-offset < visible; i++ {
		yy := y + 2 + i - offset
		keyboard := s.Focus == pane && i == selected
		isApplied := i == applied && pane != PaneChanges
		bg := CDefault
		fg := CWhite
		indicator := -1
		if isApplied {
			indicator = CGreen // active filter, regardless of keyboard focus
		}
		if keyboard {
			bg = CBlack
			indicator = CCyan // keyboard selection wins over the active marker
		}
		c.Fill(x+selectionStripeInset+selectionStripeWidth, yy, w-selectionStripeInset-selectionStripeWidth, 1, ' ', fg, bg)
		if indicator >= 0 {
			drawSelectionStripe(c, x, yy, 1, indicator, bg)
		}
		ic := glyphs[i]
		c.Text(x+3, yy, ic, CBlue, bg)
		count := fmt.Sprintf("%d/%d", i+1, len(opts))
		countX := x + w - utf8.RuneCountInString(count) - 2
		labelLimit := x + w - 6
		if pane == PaneChanges || pane == PaneFolders {
			// The counter always owns the rightmost column; leave a clear gap
			// between it and N/M/D even when the row is not selected.
			labelLimit = countX - 5
		}
		if pane == PaneChanges && s.ImportedChangeAt(i) || deletedFolderRows[i] {
			fg = CBBlack
		}
		c.Text(x+5, yy, minWidth(opts[i], max(0, labelLimit-x-5)), fg, bg)
		if keyboard {
			c.Text(countX, yy, count, CBBlack, bg)
		}
		if pane == PaneFolders {
			if kind, ok := folderMarkers[i]; ok {
				c.Text(countX-4, yy, string(kind), stageKindColor(kind), bg)
			}
		}
		if pane == PaneChanges {
			changes := s.ChangesList()
			if i < len(changes) {
				kind := changes[i].Kind
				marker := string(kind)
				markerColor := stageKindColor(kind)
				if s.ImportedChangeAt(i) {
					marker, markerColor = "↓", CGreen
				}
				c.Text(countX-4, yy, marker, markerColor, bg)
			}
		}
	}
}
func (a *App) itemsPanel(c *Canvas, x, y, w, h int) {
	if w < 6 || h < 3 {
		return
	}
	s := a.S
	bg := CTitle
	c.Fill(x, y, w, 1, ' ', CWhite, bg)
	c.Text(x+1, y, "[5]", CWhite, bg)
	if s.Focus == PaneItems {
		color := bg
		if s.Mode == Find {
			color = CWhite
		} else if s.Blink {
			color = CYellow
		}
		c.Text(x+2, y, "5", color, bg)
	}
	c.Text(x+6, y, "Name", CWhite, bg)
	// Reserve a dedicated right-hand counter column. Owner and State
	// share the same center coordinates in header and item rows.
	condensed := s.SelectedID != ""
	ownerLabel, stateLabel := "Owner", "State"
	if condensed {
		ownerLabel, stateLabel = "Ow", "St"
	}
	counterWidth := 9
	stateWidth := 6
	ownerWidth := 10
	if w < 44 {
		ownerWidth = 6
	}
	stateX := x + w - counterWidth - stateWidth - 1
	ownerX := stateX - ownerWidth - 2
	center := func(text string, left, width, row, fg, bg int) {
		if width <= 0 {
			return
		}
		label := minWidth(text, width)
		c.Text(left+max(0, (width-utf8.RuneCountInString(label))/2), row, label, fg, bg)
	}
	if ownerX >= x+11 {
		center(ownerLabel, ownerX, ownerWidth, y, CWhite, bg)
		center(stateLabel, stateX, stateWidth, y, CWhite, bg)
	}
	c.Line(x, y+1, w, '━', panelRuleColor(s.Focus == PaneItems && s.Mode != Find), CDefault)
	items := s.Filtered()
	rows := max(0, (h-4)/itemRowHeight)
	scroll := 0
	if s.ItemIndex >= rows && rows > 0 {
		scroll = s.ItemIndex - rows + 1
	}
	a.itemScroll = scroll
	previousBG, previousStripe := CDefault, -1
	lastRow := -1
	for j := scroll; j < len(items) && j-scroll < rows; j++ {
		it := items[j]
		ry := y + 2 + itemRowHeight*(j-scroll)
		selected := j == s.ItemIndex
		// Keep the opened-item stripe only while Content (or Detail) is visible.
		// Focusing a filter hides Content without clearing SelectedID.
		opened := s.SelectedID == it.ID() && s.Focus >= PaneContent
		fg := CWhite
		bg := CDefault
		if selected || opened {
			bg = CBlack
		}
		line := -1
		if opened {
			line = CGreen
		}
		if selected && s.Focus == PaneItems {
			line = CCyan
		}
		drawSharedVerticalPadding(c, x, ry, w, previousBG, bg, previousStripe, line)
		c.Fill(x+selectionStripeInset+selectionStripeWidth, ry+1, w-selectionStripeInset-selectionStripeWidth, 2, ' ', fg, bg)
		if line >= 0 {
			drawBalancedSelectionStripe(c, x, ry+1, 2, line)
		}
		previousBG, previousStripe = bg, line
		lastRow = ry
		// One shared blank row separates text blocks instead of two;
		// the three-cell stride is the smallest consistent TUI reduction.
		nameY, subtitleY := ry+1, ry+2
		kind := vault.Kind("")
		if ch := s.Stage.Get(vault.Key("item", it.ID())); ch != nil {
			kind = ch.Kind
		}
		c.Text(x+4, nameY, vault.TypeIcons[it.Type()], CBlue, bg)
		name := it.Name()
		nameWidth := max(1, ownerX-x-10)
		if ownerX < x+12 {
			nameWidth = max(1, w-19)
		}
		visibleName := minWidth(name, nameWidth)
		if it.Favorite() && nameWidth >= 3 {
			visibleName = minWidth(name, nameWidth-2)
			c.TextBold(x+9+utf8.RuneCountInString(visibleName), nameY,
				"\uf005", CBYellow, bg)
		}
		c.TextBold(x+8, nameY, visibleName, fg, bg)
		if s.Search != "" {
			ok, indices, _ := vault.FuzzyMatch(name, s.Search)
			if ok {
				rn := []rune(name)
				for _, idx := range indices {
					xx := x + 8 + idx
					if xx < x+8+utf8.RuneCountInString(visibleName) {
						c.PutBold(xx, nameY, rn[idx], CMagenta, bg)
					}
				}
			}
		}
		preview := ""
		if (it.Raw["reprompt"] == 1 || it.Raw["reprompt"] == float64(1)) &&
			s.RepromptApprovedID != it.ID() {
			preview = it.MaskedSubtitle()
		} else {
			preview = it.Subtitle()
		}
		c.Text(x+8, subtitleY, minWidth(preview, nameWidth), CSubtitle, bg)
		if ownerX >= x+11 {
			center(it.Owner(), ownerX, ownerWidth, nameY, CWhite, bg)
			stateText, stateFG := "NA", CWhite
			switch kind {
			case vault.New, vault.Modified, vault.Deleted:
				stateText, stateFG = string(kind), stageKindColor(kind)
			}
			if s.GroundItemChanged(it.ID()) {
				if stateText == "NA" {
					stateText = ""
				}
				stateText += "↓"
				start := stateX + max(0, (stateWidth-utf8.RuneCountInString(stateText))/2)
				c.Text(start, nameY, strings.TrimSuffix(stateText, "↓"), stateFG, bg)
				c.Text(start+utf8.RuneCountInString(stateText)-1, nameY, "↓", CGreen, bg)
			} else {
				center(stateText, stateX, stateWidth, nameY, stateFG, bg)
			}
		}
		if selected && s.Focus == PaneItems {
			counter := fmt.Sprintf("%d/%d", j+1, len(items))
			c.Text(x+w-len(counter)-2, nameY, counter, CBBlack, bg)
		}
	}
	// Give the final visible item the same half-cell bottom padding when
	// there is a free row before the Find separator.
	if lastRow >= 0 && lastRow+itemRowHeight < y+h-2 {
		drawSharedVerticalPadding(
			c, x, lastRow+itemRowHeight, w,
			previousBG, CDefault, previousStripe, -1,
		)
	}
	searchY := y + h - 2
	searchRule := CBlue
	if s.Mode == Find {
		searchRule = CMagenta
	}
	c.Line(x, searchY, w, '━', searchRule, CDefault)
	c.Fill(x, searchY+1, w, 1, ' ', CWhite, CFindBG)
	hint := "Find items..."
	if s.Search != "" {
		hint = s.Search
	}
	if s.Mode == Find && s.Search != "" {
		hint, _ = visibleText(s.Search, s.Caret, w-4)
	}
	c.Text(x+2, searchY+1, minWidth(hint, w-4), CMagenta, CFindBG)
}

// fieldRowHeight reserves one label row and one row for each website.
func fieldRowHeight(f vault.Field) int {
	if f.Websites || f.Folders {
		return max(contentRowHeight, len(f.Values)+2)
	}
	return contentRowHeight
}

// contentStartIndex scrolls whole fields to keep the selected field visible.
func contentStartIndex(fields []vault.Field, selected, available int) int {
	if selected < 0 || selected >= len(fields) {
		return 0
	}
	used, first := 0, selected
	for first >= 0 {
		height := fieldRowHeight(fields[first])
		if used+height > available && first < selected {
			break
		}
		used += height
		first--
	}
	return first + 1
}

func (a *App) contentPanel(c *Canvas, x, y, w, h int) {
	if w < 5 {
		return
	}
	s := a.S
	a.header(c, x, y, w, "Item content", "\uf0ce", PaneContent)
	if item := s.SelectedItem(); item != nil && w >= 13 {
		if item.Favorite() {
			c.TextBold(x+w-3, y, "\uf005", CBYellow, CTitle)
		} else {
			c.TextDim(x+w-3, y, "\uf005", CWhite, CTitle)
		}
		if kind := s.FavouriteKind(item); kind != "" {
			c.Text(x+w-9, y, string(kind), stageKindColor(kind), CTitle)
		}
		if s.GroundFieldChanged(item.ID(), []any{"favorite"}) {
			c.Text(x+w-2, y, "↓", CGreen, CTitle)
		}
	}
	c.Line(x, y+1, w, '━', panelRuleColor(s.Focus == PaneContent), CDefault)
	available := max(1, h-2)
	offset := contentStartIndex(s.Fields, s.FieldIndex, available)
	a.contentScroll = offset
	previousBG, previousStripe := CDefault, -1
	lastRow, lastHeight := -1, 0
	used := 0
	for idx := offset; idx < len(s.Fields); idx++ {
		f := s.Fields[idx]
		rowHeight := fieldRowHeight(f)
		yy := y + 2 + used
		if rowHeight > available && idx == offset {
			rowHeight = available // Show as many URIs as will fit in the panel.
		}
		if used+rowHeight > available || rowHeight < contentRowHeight {
			break
		}
		used += rowHeight
		bg := CDefault
		if idx == s.FieldIndex {
			bg = CBlack
		}
		c.Fill(x+selectionStripeInset+selectionStripeWidth, yy+1,
			w-selectionStripeInset-selectionStripeWidth, rowHeight-1, ' ', CWhite, bg)
		color := CDefault
		if idx == s.FieldIndex {
			if s.Mode == Edit {
				color = CGreen
			}
			if s.Focus == PaneContent {
				color = CCyan
			}
		}
		drawSharedVerticalPadding(c, x, yy, w, previousBG, bg, previousStripe, color)
		if color != CDefault {
			drawBalancedSelectionStripe(c, x, yy+1, rowHeight-1, color)
		}
		previousBG, previousStripe = bg, color
		lastRow, lastHeight = yy, rowHeight
		labelY, valueY := yy+1, yy+2
		nameWidth := max(0, w-18)
		c.TextBold(x+3, labelY, minWidth(f.Name, nameWidth), CWhite, bg)
		if f.Websites || f.Folders {
			for k, website := range f.Values {
				if k >= rowHeight-2 {
					break
				}
				c.TextDim(x+3, valueY+k, minWidth(website, max(0, w-5)),
					CWhite, bg)
			}
			if len(f.Values) > rowHeight-2 {
				more := fmt.Sprintf("(+%d more in picker)", len(f.Values)-(rowHeight-2))
				c.Text(x+3, valueY+rowHeight-3, minWidth(more, max(0, w-5)),
					CSubtitle, bg)
			}
		} else {
			v := ""
			if f.Hidden && (f.Restricted || !s.Revealed[fmt.Sprintf("%s:%v", s.SelectedID, f.Path)]) {
				v = vault.Mask(f.Value)
			} else {
				v, _ = f.Reveal(s.CLI.Cipher)
			}
			if isCheckboxField(&f) {
				checkboxTrue := strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
				label, fg := "False", CRed
				if checkboxTrue {
					label, fg = "True", CGreen
				}
				c.Text(x+3, valueY, label, fg, bg)
			} else {
				c.TextDim(x+3, valueY, minWidth(strings.ReplaceAll(v, "\n", " ↵ "), max(0, w-5)), CWhite, bg)
			}
		}
		if k := s.FieldStatus(f); k != "" {
			c.Text(x+w-9, labelY, string(k), stageKindColor(k), bg)
		}
		if s.GroundFieldChanged(s.SelectedID, f.Path) {
			label := minWidth(f.Name, nameWidth)
			markerX := x + 4 + utf8.RuneCountInString(label)
			if markerX < x+w-9 {
				c.Text(markerX, labelY, "↓", CGreen, bg)
			}
		}
		if idx == s.FieldIndex && s.Focus == PaneContent {
			counter := fmt.Sprintf("%d/%d", idx+1, len(s.Fields))
			c.Text(x+w-utf8.RuneCountInString(counter)-2, labelY, counter, CBBlack, bg)
		}
	}
	if lastRow >= 0 && lastRow+lastHeight < y+h {
		drawSharedVerticalPadding(c, x, lastRow+lastHeight, w,
			previousBG, CDefault, previousStripe, -1)
	}
}
func (a *App) detailPanel(c *Canvas, x, y, w, h int) {
	s := a.S
	if s.Mode != Edit {
		return
	}
	f := s.CurrentField()
	if f == nil && !s.EditNewWebsite {
		return
	}
	name := "Website"
	if !s.EditNewWebsite && f != nil {
		if f.Websites {
			name = "Website"
		} else {
			name = f.Name
		}
	}
	a.header(c, x, y, w, name, "\uf044", PaneDetail)
	c.Line(x, y+1, w, '━', panelRuleColor(s.Focus == PaneDetail), CDefault)
	c.Fill(x, y+2, w, h-2, ' ', CWhite, CBlack)
	value := s.Input
	if s.InputSecret && !s.AuthVisible {
		value = strings.Repeat("•", utf8.RuneCountInString(s.Input))
	}
	lines := strings.Split(value, "\n")
	cursorLine, cursorCol := 0, 0
	for j, r := range []rune(value) {
		if j >= s.Caret {
			break
		}
		if r == '\n' {
			cursorLine++
			cursorCol = 0
		} else {
			cursorCol++
		}
	}
	startLine := max(0, cursorLine-(h-4))
	for i := startLine; i < len(lines) && y+3+i-startLine < y+h; i++ {
		line := lines[i]
		if i == cursorLine {
			line, _ = visibleText(line, cursorCol, w-4)
		}
		c.Text(x+2, y+3+i-startLine, minWidth(line, w-4), CWhite, CBlack)
	}
	// The terminal's beam cursor is drawn over the text, not into it.
}

// PaginateHints keeps each complete hint within 75% of the full footer width.
// The ? control appears on both the end of the first page and the beginning
// of every subsequent page.
func PaginateHints(hints []string, maxWidth int) [][]string {
	if maxWidth < 10 {
		maxWidth = 10
	}
	if len(hints) == 0 {
		return [][]string{{}}
	}
	width := func(hint string) int { return utf8.RuneCountInString(hint) + 3 }
	marker := "? ..."
	pages := [][]string{}
	for next := 0; next < len(hints); {
		page := []string{}
		used := 0
		if len(pages) > 0 {
			page = append(page, marker)
			used = width(marker)
		}
		for next < len(hints) {
			remaining := 0
			for _, hint := range hints[next:] {
				remaining += width(hint)
			}
			if used+remaining <= maxWidth {
				page = append(page, hints[next:]...)
				next = len(hints)
				break
			}
			needed := width(hints[next])
			if used+needed+width(marker) > maxWidth && len(page) > 0 &&
				!(len(page) == 1 && page[0] == marker) {
				break
			}
			page = append(page, hints[next])
			used += needed
			next++
		}
		if next < len(hints) {
			page = append(page, marker)
		}
		pages = append(pages, page)
	}
	return pages
}
func (a *App) footer(c *Canvas, x, y, w int) {
	s := a.S
	limit := max(18, 3*w/4)
	if !a.textEntryActive() && !s.KeyFeedbackExpires.IsZero() &&
		time.Now().Before(s.KeyFeedbackExpires) {
		c.Text(x, y, minWidth(s.KeyFeedback, limit), CHintKey, CDefault)
		pos := utf8.RuneCountInString(s.KeyFeedback) + 1
		color := CHintAction
		if s.KeyFeedbackAction == "unbound" {
			color = CRed
		}
		c.Text(x+pos, y, minWidth(s.KeyFeedbackAction, max(0, limit-pos)),
			color, CDefault)
	} else {
		pages := PaginateHints(a.hints(), limit)
		page := pages[s.HintPage%len(pages)]
		startX := x
		if s.Popup == nil && (s.Mode == DialogCreate || s.Mode == DialogFieldType ||
			s.Mode == DialogWebsite || s.Mode == DialogLinkedChoice ||
			s.Mode == DialogCardBrand || s.Mode == DialogIdentityTitle ||
			s.Mode == DialogFoldersChoice || s.Mode == DialogName ||
			s.Mode == DialogField || s.Mode == DialogFolder || s.Mode == DialogReprompt) {
			total := 0
			for _, hint := range page {
				total += utf8.RuneCountInString(hint) + 3
			}
			if total > 0 {
				total -= 3
			}
			startX += max(0, (limit-total)/2)
		}
		off := 0
		for _, hint := range page {
			if off >= limit {
				break
			}
			if hint == "? ..." {
				c.Text(startX+off, y, hint, CHintKey, CDefault)
			} else {
				parts := strings.SplitN(hint, " ", 2)
				c.Text(startX+off, y, parts[0], CHintKey, CDefault)
				if len(parts) == 2 {
					c.Text(startX+off+utf8.RuneCountInString(parts[0])+1, y, parts[1], CHintAction, CDefault)
				}
			}
			off += utf8.RuneCountInString(hint) + 3
		}
	}
	right := "• Bitwarden CLI " + s.Version
	connectionLost := s.Screen == Workspace && !s.LocalMode && !s.ServerConnected
	if s.LocalMode {
		right = "• Local"
	}
	if connectionLost {
		right = websiteFieldOption.Icon + " Server connexion lost"
	} else if s.Busy || s.BackupBusy {
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		right = fmt.Sprintf("%c %s", frames[s.Spinner%len(frames)], s.BusyMessage)
	} else if s.Notice != "" && time.Now().Before(s.NoticeUntil) {
		right = s.Notice
	}
	rx := x + max(limit+1, w-utf8.RuneCountInString(right)-1)
	available := max(0, x+w-rx)
	if connectionLost {
		c.Text(rx, y, minWidth(right, available), CRed, CDefault)
	} else if s.Busy || s.BackupBusy {
		color := CGreen
		if s.BackupBusy && !s.Busy {
			color = CBlue
		}
		c.Text(rx, y, minWidth(right, available), color, CDefault)
	} else if s.Notice != "" && time.Now().Before(s.NoticeUntil) {
		c.Text(rx, y, minWidth(right, available), s.NoticeColor, CDefault)
	} else if s.LocalMode {
		c.Text(rx, y, "•", CRed, CDefault)
		c.Text(rx+2, y, "Local", CWhite, CDefault)
	} else {
		c.Text(rx, y, "•", CGreen, CDefault)
		c.Text(rx+2, y, "Bitwarden CLI ", CWhite, CDefault)
		c.Text(rx+16, y, minWidth(s.Version, max(0, available-16)), CYellow, CDefault)
	}
}

// panelNumber maps the local read-only workspace to contiguous shortcuts.
func panelNumber(s *State, pane Pane) int {
	if !s.LocalMode {
		return int(pane)
	}
	switch pane {
	case PaneTypes:
		return 1
	case PaneFolders:
		return 2
	case PaneItems:
		return 3
	case PaneContent:
		return 4
	}
	return 0
}

// paneForNumber converts a displayed panel number to its internal identifier.
func paneForNumber(s *State, number int) Pane {
	if !s.LocalMode {
		return Pane(number)
	}
	switch number {
	case 1:
		return PaneTypes
	case 2:
		return PaneFolders
	case 3:
		return PaneItems
	case 4:
		return PaneContent
	}
	return 0
}

// panelNavigationHints only advertises configured shortcuts for rendered panels.
// It includes the focused panel key separately as a back-navigation shortcut.
func panelNavigationHints(s *State) []string {
	if s.Mode == Edit && s.Focus == PaneDetail {
		return nil
	}
	panes := []Pane{PaneVault, PaneTypes, PaneFolders, PaneChanges, PaneItems}
	if s.LocalMode {
		panes = []Pane{PaneTypes, PaneFolders, PaneItems}
	}
	if s.SelectedID != "" && s.Focus >= PaneContent {
		panes = append(panes, PaneContent)
	}
	actions := map[Pane]string{
		PaneVault: "focus_my_vault", PaneTypes: "focus_item_types",
		PaneFolders: "focus_folders", PaneChanges: "focus_changes",
		PaneItems: "focus_items", PaneContent: "focus_content",
	}
	keys := make([]string, 0, len(panes)-1)
	for _, pane := range panes {
		if pane != s.Focus {
			if key := s.Config.Key("global", actions[pane]); key != "" {
				keys = append(keys, hintLabel(key))
			}
		}
	}
	out := []string{strings.Join(keys, "/") + " panels"}
	if s.Focus != PaneItems {
		if key := s.Config.Key("global", actions[s.Focus]); key != "" {
			out = append(out, hintLabel(key)+" return")
		}
	}
	return out
}

func (a *App) baseHints() []string {
	s := a.S
	if s.Popup != nil {
		if s.Popup.Blocking {
			return nil // a blocking modal has no available keybindings
		}
		if s.Popup.Acknowledge {
			return []string{"ENTER OK", "esc close"}
		}
		h := []string{"←/→ choose", "y yes", "n no", "ENTER confirm", "c cancel", "esc close"}
		if s.Popup.RemoteReview != nil {
			if s.Stage.Len() == 0 {
				h = []string{"←/→ choose", "s sync", "a abort sync", "ENTER select", "esc close"}
			} else {
				h = []string{"←/→ choose", "s soft sync", "h hard sync", "a abort sync", "ENTER select", "esc close"}
			}
		}
		pageSize := 10
		if s.Popup.RemoteReview != nil {
			pageSize = a.reviewPageSize()
		}
		if len(s.Popup.Lines) > pageSize {
			h = append(h, "? next page")
		}
		return h
	}
	switch s.Mode {
	case DialogCreate, DialogFieldType, DialogWebsite, DialogLinkedChoice,
		DialogCardBrand, DialogIdentityTitle:
		return []string{"↑/↓ move", "ENTER select", "esc cancel"}
	case DialogFoldersChoice:
		return []string{"↑/↓ move", "s switch", "ENTER apply", "esc cancel"}
	case DialogName, DialogField, DialogFolder:
		return []string{"↑/↓ move", "←/→ move", "ENTER select", "esc cancel"}
	}
	if s.Mode == DialogReprompt {
		return []string{"ctrl+r reveal", "ENTER verify", "esc cancel"}
	}
	h := []string{"ctrl+q quit"}
	if !s.LocalMode && s.ServerConnected {
		h = append(h, "ctrl+s sync")
	}
	if s.Mode == Browse {
		h = append(h, panelNavigationHints(s)...)
	}
	if !s.LocalMode && s.Stage.Len() > 0 {
		if s.ServerConnected {
			h = append(h, "ctrl+p push all")
		}
		h = append(h, "ctrl+d discard all")
	}
	switch {
	case s.Mode == Find:
		h = append(h, "ENTER apply", "esc back")
	case s.Mode == Edit:
		h = append(h, "ENTER apply", "esc cancel", "ctrl+ENTER newline")
		if s.InputSecret {
			h = append(h, "ctrl+r reveal")
		}
	case s.Focus == PaneChanges:
		h = append(h, "↑/↓ move")
		if s.ServerConnected && !s.ImportedChangeAt(s.ChangeIndex) {
			h = append(h, "s push")
		}
		if !s.ImportedChangeAt(s.ChangeIndex) {
			h = append(h, "d discard")
		}
	case s.Focus == PaneContent:
		h = append(h, "↑/↓ move")
		item, field := s.SelectedItem(), s.CurrentField()
		if !s.LocalMode && item != nil && item.CanEdit() {
			h = append(h, "f favourite")
			if field != nil && field.Editable {
				if isCheckboxField(field) {
					h = append(h, "s switch")
				} else {
					h = append(h, "e edit")
				}
			}
			h = append(h, "n new")
			if field != nil && field.Custom && field.Editable {
				h = append(h, "d delete")
			}
		}
		if field != nil && !field.Restricted {
			h = append(h, "c copy")
			if field.Hidden {
				h = append(h, "r reveal")
			}
		}
		h = append(h, "← back")
	case s.Focus == PaneFolders:
		enterHint := "ENTER select"
		if s.Filters[2] >= 0 && s.Filters[2] == s.FilterFolder {
			enterHint = "ENTER unselect"
		}
		h = append(h, "↑/↓ move", enterHint)
		if !s.LocalMode {
			h = append(h, "n new")
			if s.Filters[2] > 0 && s.Filters[2] <= len(s.Folders) {
				h = append(h, "d delete")
			}
		}
	case s.Focus >= PaneVault && s.Focus <= PaneTypes:
		h = append(h, "↑/↓ move", "ENTER select")
	case s.Focus == PaneItems:
		if s.LocalMode {
			h = append(h, "↑/↓ move", "→ open", "f find")
		} else {
			h = append(h, "↑/↓ move", "→ open", "f find", "n new")
			if current := s.CurrentItem(); current != nil && current.CanEdit() {
				h = append(h, "d delete")
			}
		}
	}
	return h
}
