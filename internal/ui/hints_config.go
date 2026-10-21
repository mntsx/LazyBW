package ui

import (
	"fmt"
	"strings"

	"github.com/miguelmnts/lazybw-go/internal/config"
)

type hintSpec struct {
	section string
	actions []string
	verb    string
}

// Footer hints are semantic section/action/verb specifications. Do not add
// user-facing key combinations here: DefaultKeybinds is the sole source of
// default bindings, and Config.Keys is the sole source used to render them.
func hintLabel(key string) string {
	switch key {
	case "enter":
		return "ENTER"
	case "escape", "esc":
		return "esc"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	default:
		return key
	}
}

func hint(section, action, verb string) hintSpec {
	return hintSpec{section: section, actions: []string{action}, verb: verb}
}

func combinedHint(section, verb string, actions ...string) hintSpec {
	return hintSpec{section: section, actions: actions, verb: verb}
}

func (a *App) renderHint(spec hintSpec) string {
	labels := make([]string, 0, len(spec.actions))
	for _, action := range spec.actions {
		// Use the preferred configured binding only. The complete binding list is
		// deliberately obtained through Config.Keys so defaults never leak into
		// presentation code as hardcoded combinations.
		keys := a.S.Config.Keys(spec.section, action)
		if len(keys) == 0 || keys[0] == "" {
			return ""
		}
		labels = append(labels, hintLabel(keys[0]))
	}
	if len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, "/") + " " + spec.verb
}

func (a *App) renderHints(specs []hintSpec) []string {
	hints := make([]string, 0, len(specs))
	for _, spec := range specs {
		if rendered := a.renderHint(spec); rendered != "" {
			hints = append(hints, rendered)
		}
	}
	return hints
}

// defaultHintPageMarker preserves PaginateHints' standalone API while taking
// its visible binding from DefaultKeybinds rather than a literal key string.
func defaultHintPageMarker() string {
	keys := config.Config{}.Keys("global", "hint_next_page")
	if len(keys) == 0 {
		return ""
	}
	return hintLabel(keys[0]) + " ..."
}

// paginationHint uses an alternate configured binding while text is being
// entered, when available, so the primary binding remains ordinary input.
func (a *App) paginationHint(textEntry bool) string {
	keys := a.S.Config.Keys("global", "hint_next_page")
	if len(keys) == 0 {
		return ""
	}
	index := 0
	if textEntry && len(keys) > 1 {
		index = 1
	}
	return hintLabel(keys[index]) + " ..."
}

// popupPageHint renders popup pagination from its configured action. Do not
// embed a key here: defaults belong only in config.DefaultKeybinds and are
// persisted by the JSONC creation/completion path.
func (a *App) popupPageHint(section, action, verb string, page, total int) string {
	key := a.S.Config.Key(section, action)
	if key == "" {
		return fmt.Sprintf("page %d/%d", page, total)
	}
	return fmt.Sprintf("%s %s  page %d/%d", hintLabel(key), verb, page, total)
}

// hints renders the available semantic footer specifications.
func (a *App) hints() []string { return a.baseHints() }

func (a *App) configuredHint(section, action, verb string) string {
	return a.renderHint(hint(section, action, verb))
}

func (a *App) configuredMoveHint(section string, horizontal bool) string {
	actions := []string{"previous", "next"}
	if horizontal {
		actions = append(actions, "button_left", "button_right")
	}
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		keys := a.S.Config.Keys(section, action)
		if len(keys) > 0 && keys[0] != "" {
			labels = append(labels, hintLabel(keys[0]))
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, "/") + " move"
}

func joinConfiguredHints(hints ...string) string {
	visible := hints[:0]
	for _, hint := range hints {
		if hint != "" {
			visible = append(visible, hint)
		}
	}
	return strings.Join(visible, "    ")
}
