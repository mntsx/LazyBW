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

func (a *App) icon(section, name string) string {
	return a.S.Config.Icon(section, name)
}

func (a *App) itemIcon(itemType int) string {
	name := map[int]string{
		vault.Login: "login", vault.SecureNote: "secure_note", vault.Card: "card",
		vault.Identity: "identity", vault.SSHKey: "ssh_key",
	}[itemType]
	return a.icon("items", name)
}

func (a *App) fieldIcon(option customFieldOption) string {
	if option.Name == "Website" {
		return a.icon("contents", "web")
	}
	name := map[int]string{0: "text", 1: "hidden", 2: "checkbox", 3: "linked"}[option.BitwardenType]
	return a.icon("contents", name)
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
		{"Vault", a.icon("layout", "vault"), PaneVault},
		{"Items", a.icon("layout", "items"), PaneTypes},
		{"Folders", a.icon("layout", "folders"), PaneFolders},
		{"Edits", a.icon("layout", "edits"), PaneChanges},
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
	a.detailX, a.detailY, a.detailW, a.detailH = 0, 0, 0, 0
	a.itemsPanel(c, middleStart, panelY, a.itemW, topH)
	var sections []borderSection
	if contentX > 0 {
		contentWidth := w - contentX - 1
		a.contentW = contentWidth
		if s.Mode == Edit && !s.LocalMode && !s.DetailMaximized {
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
			a.detailW = contentWidth
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
	nameX := x + 5
	if icon != "" {
		c.Text(nameX, y, icon, CBlue, CTitle)
		nameX += 2
	}
	c.Text(nameX, y, minWidth(name, max(0, x+w-nameX-1)), CWhite, CTitle)
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
	overlongChanges := map[int]bool{}
	selected := 0
	applied := -1
	switch pane {
	case PaneVault:
		opts = []string{"All vaults", "My vault"}
		glyphs = []string{a.icon("items", "vault"), a.icon("items", "user")}
		selected = 1 - s.Filters[0]
		applied = 1 - s.FilterVault
	case PaneTypes:
		opts = []string{"All items", "Favourites", "Login", "Card", "Identity", "Secure note", "SSH key"}
		glyphs = []string{a.icon("items", "all"), a.icon("items", "favourite"), a.itemIcon(vault.Login), a.itemIcon(vault.Card), a.itemIcon(vault.Identity), a.itemIcon(vault.SecureNote), a.itemIcon(vault.SSHKey)}
		selected = s.Filters[1]
		applied = s.FilterType
	case PaneFolders:
		opts = []string{"No Folder"}
		glyphs = []string{a.icon("items", "no_folder")}
		for _, f := range s.Folders {
			index := len(opts)
			opts = append(opts, fmt.Sprint(f["name"]))
			glyphs = append(glyphs, a.icon("items", "folder"))
			if change := s.Stage.Get(vault.Key("folder", fmt.Sprint(f["id"]))); change != nil {
				folderMarkers[index] = change.Kind
			}
		}
		// Deleted folders remain visible as read-only pending deletion rows.
		for _, change := range s.Stage.Changes() {
			if change.Entity == "folder" && change.Kind == vault.Deleted {
				index := len(opts)
				opts = append(opts, change.Name)
				glyphs = append(glyphs, a.icon("items", "folder"))
				folderMarkers[index] = vault.Deleted
				deletedFolderRows[index] = true
			}
		}
		selected = s.Filters[2]
		applied = s.FilterFolder
	case PaneChanges:
		for index, v := range s.ChangesList() {
			opts = append(opts, v.Name)
			raw := v.After
			if raw == nil {
				raw = v.Before
			}
			if v.Entity == "item" && raw != nil {
				overlongChanges[index] = (&vault.Item{Raw: raw, Cipher: s.CLI.Cipher}).HasOverlongField()
			}
			if v.Entity == "folder" {
				glyphs = append(glyphs, a.icon("items", "folder"))
			} else {
				code := 1
				for t, label := range vault.TypeNames {
					if label == v.TypeName {
						code = t
						break
					}
				}
				glyphs = append(glyphs, a.itemIcon(code))
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
		labelX := x + 3
		if ic != "" {
			c.Text(labelX, yy, ic, CBlue, bg)
			labelX += 2
		}
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
		labelWidth := max(0, labelLimit-labelX)
		if pane == PaneChanges && overlongChanges[i] {
			labelWidth = max(0, labelWidth-2)
		}
		label := minWidth(opts[i], labelWidth)
		c.Text(labelX, yy, label, fg, bg)
		if pane == PaneChanges && overlongChanges[i] {
			c.Text(labelX+utf8.RuneCountInString(label)+1, yy, "!", CRed, bg)
		}
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
		itemIcon := a.itemIcon(it.Type())
		nameX := x + 4
		if itemIcon != "" {
			c.Text(nameX, nameY, itemIcon, CBlue, bg)
			nameX += 4
		}
		name := it.Name()
		nameWidth := max(1, ownerX-nameX-2)
		if ownerX < x+12 {
			nameWidth = max(1, w-(nameX-x)-11)
		}
		overlong := it.HasOverlongField()
		favourite := it.Favorite() && nameWidth >= 3
		suffixWidth := 0
		if overlong {
			suffixWidth += 2 // space and warning marker
		}
		if favourite {
			suffixWidth += 2 // space and favourite icon
		}
		visibleName := minWidth(name, max(1, nameWidth-suffixWidth))
		c.TextBold(nameX, nameY, visibleName, fg, bg)
		suffixX := nameX + utf8.RuneCountInString(visibleName) + 1
		if overlong {
			c.Text(suffixX, nameY, "!", CRed, bg)
			suffixX += 2
		}
		if favourite {
			c.TextBold(suffixX, nameY, a.S.Config.FavouriteIcon("items"), CBYellow, bg)
		}
		if s.Search != "" {
			ok, indices, _ := vault.FuzzyMatch(name, s.Search)
			if ok {
				rn := []rune(name)
				for _, idx := range indices {
					xx := nameX + idx
					if xx < nameX+utf8.RuneCountInString(visibleName) {
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
		c.Text(nameX, subtitleY, minWidth(preview, nameWidth), CSubtitle, bg)
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

const minuteTimestampLayout = "2006-01-02T15:04"

func itemModifiedTimestamp(s *State, item *vault.Item) (string, int, bool) {
	if item == nil {
		return "", CWhite, true
	}
	if change := s.Stage.Get(vault.Key("item", item.ID())); change != nil &&
		(change.Kind == vault.New || change.Kind == vault.Modified) && !change.ChangedAt.IsZero() {
		color := CYellow
		if change.Kind == vault.New {
			color = CGreen
		}
		return change.ChangedAt.Format(minuteTimestampLayout), color, false
	}
	raw, _ := item.Raw["revisionDate"].(string)
	if raw == "" {
		return "", CWhite, true
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.Format(minuteTimestampLayout), CWhite, true
		}
	}
	if len(raw) >= len(minuteTimestampLayout) {
		raw = raw[:len(minuteTimestampLayout)]
	}
	return raw, CWhite, true
}

func (a *App) contentPanel(c *Canvas, x, y, w, h int) {
	if w < 5 {
		return
	}
	s := a.S
	const title = "Last modified:"
	a.header(c, x, y, w, title, a.icon("contents", "last_modified"), PaneContent)
	item := s.SelectedItem()
	if timestamp, color, dim := itemModifiedTimestamp(s, item); timestamp != "" {
		timestampX := x + 8 + utf8.RuneCountInString(title)
		text := minWidth(timestamp, max(0, x+w-timestampX))
		if dim {
			c.TextDim(timestampX, y, text, color, CTitle)
		} else {
			c.Text(timestampX, y, text, color, CTitle)
		}
	}
	if item != nil && w >= 13 {
		if item.Favorite() {
			c.TextBold(x+w-3, y, a.S.Config.FavouriteIcon("contents"), CBYellow, CTitle)
		} else {
			c.TextDim(x+w-3, y, a.S.Config.FavouriteIcon("contents"), CWhite, CTitle)
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
		label := minWidth(f.Name, nameWidth)
		c.TextBold(x+3, labelY, label, CWhite, bg)
		markerX := x + 4 + utf8.RuneCountInString(label)
		if f.ExceedsUTF8Limit(s.CLI.Cipher) && markerX < x+w-9 {
			c.Text(markerX, labelY, "!", CRed, bg)
			markerX += 2
		}
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
		if s.GroundFieldChanged(s.SelectedID, f.Path) && markerX < x+w-9 {
			c.Text(markerX, labelY, "↓", CGreen, bg)
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

// inputLengthCounter formats the editor's byte-based guidance and reports the
// appropriate themed color. A zero encrypted limit identifies a non-text editor.
func inputLengthCounter(value string, encryptedLimit int) (string, int, bool) {
	maximum := vault.PlaintextByteLimit(encryptedLimit)
	if maximum == 0 {
		return "", CDefault, false
	}
	color := CBBlack
	if len(value) > maximum {
		color = CRed
	}
	return fmt.Sprintf("UTF-8 bytes %d/%d", len(value), maximum), color, true
}

func (a *App) detailAllowsNewlines() bool {
	f := a.S.CurrentField()
	return f != nil && (strings.EqualFold(f.Name, "notes") || f.Custom && f.FieldType == 0)
}

// detailPopupRender draws the active editor as a centered presentation of the
// same State buffer. It intentionally does not use State.Popup, which would
// disable the normal detail keymap and text entry.
func (a *App) detailPopupRender(c *Canvas) {
	outerW := min(max(24, c.W*60/100), max(3, c.W-4))
	outerH := 7 // Outline plus title, rule, centered text and bottom padding.
	if a.detailAllowsNewlines() {
		outerH = min(max(8, c.H*65/100), max(6, c.H-4))
	}
	outerX := max(0, (c.W-outerW)/2)
	outerY := max(1, (c.H-outerH)/2)
	drawDialogBorder(c, outerX, outerY, outerW, outerH)
	a.detailX, a.detailY = outerX+1, outerY+1
	a.detailW, a.detailH = outerW-2, outerH-2
	a.detailPanel(c, a.detailX, a.detailY, a.detailW, a.detailH)
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
	counter, counterColor, showCounter := inputLengthCounter(s.Input, s.InputEncryptedLimit)
	counterWidth := utf8.RuneCountInString(counter)
	if showCounter && w >= counterWidth+14 {
		// Reserve the title-bar's right edge for the counter rather than drawing
		// over a long field name.
		name = minWidth(name, max(0, w-counterWidth-8))
	} else {
		showCounter = false
	}
	a.header(c, x, y, w, name, a.icon("contents", "edit"), PaneDetail)
	if showCounter {
		c.Text(x+w-counterWidth-1, y, counter, counterColor, CTitle)
	}
	c.Line(x, y+1, w, '━', panelRuleColor(s.Focus == PaneDetail), CDefault)
	c.Fill(x, y+2, w, h-2, ' ', CWhite, CBlack)
	value := s.Input
	if s.InputSecret && !s.AuthVisible {
		value = strings.Repeat("•", utf8.RuneCountInString(s.Input))
	}
	lines, cursorLine, _ := detailDisplayLines(value, s.Caret, w-4, s.InputWrap)
	offsets := detailDisplayOffsets(value, s.Caret, w-4, s.InputWrap)
	selectedStart, selectedEnd, selecting := s.selection()
	startLine := max(0, cursorLine-(h-4))
	for i := startLine; i < len(lines) && y+3+i-startLine < y+h; i++ {
		for column, char := range []rune(lines[i]) {
			fg, bg := CWhite, CBlack
			if selecting && i < len(offsets) && column < len(offsets[i]) &&
				offsets[i][column] >= selectedStart && offsets[i][column] < selectedEnd {
				fg, bg = CBlack, CYellow
			}
			c.Put(x+2+column, y+3+i-startLine, char, fg, bg)
		}
	}
	// The terminal's beam cursor is drawn over the text, not into it.
}

// PaginateHints keeps each complete hint within 75% of the full footer width.
// The configured pagination control appears on both the end of the first page
// and the beginning of every subsequent page.
func PaginateHints(hints []string, maxWidth int) [][]string {
	if maxWidth < 10 {
		maxWidth = 10
	}
	if len(hints) == 0 {
		return [][]string{{}}
	}
	width := func(hint string) int { return utf8.RuneCountInString(hint) + 3 }
	marker := defaultHintPageMarker()
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
		page := append([]string(nil), pages[s.HintPage%len(pages)]...)
		if a.textEntryActive() {
			for i, hint := range page {
				if hint == defaultHintPageMarker() {
					page[i] = a.paginationHint(true)
				}
			}
		}
		startX := x
		off := 0
		for _, hint := range page {
			if off >= limit {
				break
			}
			if hint == defaultHintPageMarker() || hint == a.paginationHint(true) {
				rendered := a.paginationHint(a.textEntryActive())
				c.Text(startX+off, y, rendered, CHintKey, CDefault)
				hint = rendered
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
		right = "Server connexion lost"
		if icon := a.icon("layout", "connection"); icon != "" {
			right = icon + " " + right
		}
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
			action := actions[pane]
			if s.LocalMode {
				action = localFocusSlotAction(action)
			}
			bindings := s.Config.Keys("global", action)
			if len(bindings) > 0 && bindings[0] != "" {
				keys = append(keys, hintLabel(bindings[0]))
			}
		}
	}
	out := []string{strings.Join(keys, "/") + " panels"}
	if s.Focus != PaneItems {
		action := actions[s.Focus]
		if s.LocalMode {
			action = localFocusSlotAction(action)
		}
		bindings := s.Config.Keys("global", action)
		if len(bindings) > 0 && bindings[0] != "" {
			out = append(out, hintLabel(bindings[0])+" return")
		}
	}
	return out
}

// baseHints contains semantic actions only. Never place a literal key here:
// defaults must be added to config.DefaultKeybinds so keybinds.jsonc is created
// and completed with the same values, then rendered via Config.Keys.
func (a *App) baseHints() []string {
	s := a.S
	if s.Popup != nil {
		if s.Popup.Blocking {
			return nil // a blocking modal has no available keybindings
		}
		section := "confirmation_popup"
		specs := []hintSpec{
			combinedHint(section, "choose", "button_left", "button_right"),
			hint(section, "accept", "yes"), hint(section, "reject", "no"),
			hint(section, "confirm", "confirm"), hint(section, "cancel", "cancel"),
		}
		if s.Popup.Acknowledge {
			specs = []hintSpec{hint(section, "confirm", "OK"), hint(section, "cancel", "close")}
		} else if s.Popup.RemoteReview != nil {
			section = "sync_popup"
			specs = []hintSpec{combinedHint(section, "choose", "button_left", "button_right")}
			if s.Stage.Len() == 0 {
				specs = append(specs, hint(section, "soft_sync", "sync"))
			} else {
				specs = append(specs, hint(section, "soft_sync", "soft sync"), hint(section, "hard_sync", "hard sync"))
			}
			specs = append(specs, hint(section, "abort", "abort sync"), hint(section, "confirm", "select"))
		}
		pageSize := 10
		if s.Popup.RemoteReview != nil {
			pageSize = a.reviewPageSize()
		}
		if len(s.Popup.Lines) > pageSize {
			specs = append(specs, hint(section, "next_page", "next page"))
		}
		return a.renderHints(specs)
	}

	switch s.Mode {
	case DialogCreate, DialogFieldType, DialogWebsite, DialogLinkedChoice, DialogCardBrand, DialogIdentityTitle:
		section := map[Mode]string{DialogCreate: "create_item_popup", DialogFieldType: "create_field_popup", DialogWebsite: "website_popup", DialogLinkedChoice: "linked_field_popup", DialogCardBrand: "card_brand_popup", DialogIdentityTitle: "identity_title_popup"}[s.Mode]
		return a.renderHints([]hintSpec{combinedHint(section, "move", "previous", "next"), hint(section, "confirm", "select"), hint(section, "cancel", "cancel")})
	case DialogFoldersChoice:
		return a.renderHints([]hintSpec{combinedHint("folder_selector_popup", "move", "previous", "next"), hint("folder_selector_popup", "switch", "switch"), hint("folder_selector_popup", "confirm", "apply"), hint("folder_selector_popup", "cancel", "cancel")})
	case DialogName, DialogField, DialogFolder:
		section := "name_popup"
		if s.Mode == DialogFolder {
			section = "folder_popup"
		}
		return a.renderHints([]hintSpec{combinedHint(section, "move", "previous", "next"), combinedHint(section, "move", "button_left", "button_right"), hint(section, "confirm", "select"), hint(section, "cancel", "cancel")})
	case DialogReprompt:
		return a.renderHints([]hintSpec{hint("reprompt_popup", "reveal_password", "reveal"), hint("reprompt_popup", "confirm", "verify"), hint("reprompt_popup", "cancel", "cancel")})
	}

	specs := []hintSpec{}
	if s.Mode != Edit {
		specs = append(specs, hint("global", "quit", "quit"))
		if !s.LocalMode && s.ServerConnected {
			specs = append(specs, hint("global", "sync", "sync"))
		}
		if !s.LocalMode && s.Stage.Len() > 0 {
			if s.ServerConnected {
				specs = append(specs, hint("global", "push_all", "push all"))
			}
			specs = append(specs, hint("global", "discard_all", "discard all"))
		}
	}
	out := a.renderHints(specs)
	if s.Mode == Browse {
		out = append(out, panelNavigationHints(s)...)
	}

	specs = nil
	switch {
	case s.Mode == Find:
		specs = []hintSpec{hint("search", "apply", "apply"), hint("search", "leave", "back")}
	case s.Mode == Edit:
		wrapLabel, maximizeLabel := "wrap", "expand"
		if s.InputWrap { wrapLabel = "unwrap" }
		if s.DetailMaximized { maximizeLabel = "enclose" }
		specs = []hintSpec{hint("detail", "save", "apply"), hint("detail", "back", "cancel"), hint("detail", "maximize", maximizeLabel), hint("detail", "wrap", wrapLabel), hint("detail", "newline", "newline"), hint("detail", "undo", "undo"), hint("detail", "redo", "redo"), hint("detail", "cut", "cut"), hint("detail", "copy", "copy"), hint("detail", "paste", "paste")}
		if s.InputSecret { specs = append(specs, hint("detail", "reveal", "reveal")) }
	case s.Focus == PaneChanges:
		specs = append(specs, combinedHint("edits", "move", "previous", "next"))
		if s.ServerConnected && !s.ImportedChangeAt(s.ChangeIndex) { specs = append(specs, hint("edits", "save", "push")) }
		if !s.ImportedChangeAt(s.ChangeIndex) { specs = append(specs, hint("edits", "discard", "discard")) }
	case s.Focus == PaneContent:
		specs = append(specs, combinedHint("content", "move", "previous", "next"))
		item, field := s.SelectedItem(), s.CurrentField()
		if !s.LocalMode && item != nil && item.CanEdit() {
			specs = append(specs, hint("content", "favorite", "favourite"))
			if field != nil && field.Editable {
				action := "edit"; verb := "edit"
				if isCheckboxField(field) { action, verb = "switch", "switch" }
				specs = append(specs, hint("content", action, verb))
			}
			specs = append(specs, hint("content", "new_field", "new"))
			if field != nil && field.Custom && field.Editable { specs = append(specs, hint("content", "delete_field", "delete")) }
		}
		if field != nil && !field.Restricted {
			specs = append(specs, hint("content", "copy", "copy"))
			if field.Hidden { specs = append(specs, hint("content", "reveal", "reveal")) }
		}
		specs = append(specs, hint("content", "back", "back"))
	case s.Focus == PaneFolders:
		verb := "select"
		if s.Filters[2] >= 0 && s.Filters[2] == s.FilterFolder { verb = "unselect" }
		specs = append(specs, combinedHint("folders", "move", "previous", "next"), hint("folders", "select", verb))
		if !s.LocalMode {
			specs = append(specs, hint("folders", "new", "new"))
			if s.Filters[2] > 0 && s.Filters[2] <= len(s.Folders) { specs = append(specs, hint("folders", "delete", "delete")) }
		}
	case s.Focus >= PaneVault && s.Focus <= PaneTypes:
		specs = []hintSpec{combinedHint("filters", "move", "previous", "next"), hint("filters", "select", "select")}
	case s.Focus == PaneItems:
		specs = []hintSpec{combinedHint("items", "move", "previous", "next"), hint("items", "open", "open"), hint("items", "search", "find")}
		if !s.LocalMode {
			specs = append(specs, hint("items", "new", "new"))
			if current := s.CurrentItem(); current != nil && current.CanEdit() { specs = append(specs, hint("items", "delete", "delete")) }
		}
	}
	return append(out, a.renderHints(specs)...)
}
