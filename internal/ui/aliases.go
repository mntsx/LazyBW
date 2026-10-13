package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/awesome-gocui/gocui"
	"github.com/miguelmnts/lazybw-go/internal/config"
)

// Configured alternatives are registered in addition to the prototype's
// standard bindings so that personal keymaps remain usable after migration.
func (a *App) bindAliases(g *gocui.Gui) error {
	type alias struct {
		section, action, original string
		fn                        func() error
	}
	aliases := []alias{
		{"global", "quit", "ctrl+q", func() error { a.requestQuit(); return nil }},
		{"global", "push_all", "ctrl+p", func() error { a.confirmStage(true, true); return nil }},
		{"global", "sync", "ctrl+s", func() error { a.checkRemote(); return nil }},
		{"global", "focus_content", "6", func() error { return a.char('6') }},
		{"global", "hint_next_page", "?", func() error { return a.char('?') }},
		{"global", "discard_all", "ctrl+d", func() error { a.confirmStage(false, true); return nil }},
		{"global", "focus_my_vault", "1", func() error { return a.char('1') }},
		{"global", "focus_item_types", "2", func() error { return a.char('2') }},
		{"global", "focus_folders", "3", func() error { return a.char('3') }},
		{"global", "focus_changes", "4", func() error { return a.char('4') }},
		{"global", "focus_items", "5", func() error { return a.char('5') }},
		{"authentication", "next", "down", func() error { return a.arrow(1, 0) }},
		{"authentication", "previous", "up", func() error { return a.arrow(-1, 0) }},
		{"authentication", "button_left", "left", func() error { return a.arrow(0, -1) }},
		{"authentication", "button_right", "right", func() error { return a.arrow(0, 1) }},
		{"items", "search", "f", func() error { return a.char('f') }},
		{"items", "delete", "d", func() error { return a.char('d') }},
		{"items", "new", "n", func() error { return a.char('n') }},
		{"edits", "save", "s", func() error { return a.char('s') }},
		{"edits", "discard", "d", func() error { return a.char('d') }},
		{"content", "copy", "c", func() error { return a.char('c') }},
		{"content", "reveal", "r", func() error { return a.char('r') }},
		{"content", "edit", "e", func() error { return a.char('e') }},
		{"content", "switch", "s", func() error { return a.S.SwitchCheckbox() }},
		{"content", "favorite", "f", func() error { return a.char('f') }},
		{"folders", "new", "n", func() error { return a.char('n') }},
		{"folders", "delete", "d", func() error { return a.char('d') }},
		{"filters", "select", "enter", a.enter},
		{"items", "open", "right", a.enter},
		{"edits", "save", "s", a.enter},
		{"content", "open_detail", "right", a.enter},
		{"content", "new_field", "n", func() error { return a.char('n') }},
		{"content", "delete_field", "d", func() error { return a.char('d') }},
		{"folder_selector_popup", "switch", "s", func() error { return a.char('s') }},
		{"sync_popup", "soft_sync", "s", func() error { return a.char('s') }},
		{"sync_popup", "hard_sync", "h", func() error { return a.char('h') }},
		{"sync_popup", "abort", "a", func() error { return a.char('a') }},
		{"confirmation_popup", "accept", "y", func() error { return a.char('y') }},
		{"confirmation_popup", "reject", "n", func() error { return a.char('n') }},
	}
	// Every action in the generated JSONC has a dispatcher. Users may add
	// alternative bindings without changing the built-in terminal controls.
	for section, actions := range config.DefaultKeybinds {
		for action, bindings := range actions {
			handler := a.configuredShortcut(section, action)
			if handler == nil {
				continue
			}
			original := ""
			if len(bindings) > 0 {
				original = bindings[0]
			}
			aliases = append(aliases, alias{section, action, original, handler})
		}
	}
	// Track the built-in occupied keys to avoid replacing their callbacks.
	occupied := map[string]bool{"up": true, "down": true, "left": true, "right": true, "enter": true, "escape": true, "backspace": true, "delete": true, "home": true, "end": true, "ctrl+q": true, "ctrl+s": true, "ctrl+p": true, "ctrl+d": true, "ctrl+r": true, "ctrl+j": true, "ctrl+w": true, "ctrl+left": true, "ctrl+right": true, "ctrl+backspace": true, "ctrl+delete": true, "tab": true, "?": true}
	for _, r := range "12345fndrcesh" {
		occupied[string(r)] = true
	}
	byKey := make(map[string][]alias)
	for _, entry := range aliases {
		for _, name := range a.S.Config.Keys(entry.section, entry.action) {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" || occupied[name] {
				continue
			}
			byKey[name] = append(byKey[name], entry)
		}
	}
	for name, choices := range byKey {
		key, ok := parseKey(name)
		if !ok {
			continue
		}
		label := name
		options := choices
		if err := g.SetKeybinding("", key, gocui.ModNone,
			func(_ *gocui.Gui, _ *gocui.View) error {
				if a.textEntryActive() {
					return nil
				}
				for _, option := range options {
					if !a.aliasAvailable(option.section, option.action) {
						continue
					}
					before := a.feedbackState()
					err := option.fn()
					if a.exitRequested.Load() {
						return a.quitResult(err)
					}
					actionName := aliasActionName(option.action)
					if err != nil || feedbackRequiresChange(actionName) && a.feedbackState() == before {
						actionName = "unbound"
					}
					a.reportKey(label, actionName)
					return err
				}
				a.reportKey(label, "unbound")
				return nil
			}); err != nil {
			return err
		}
	}

	return nil
}

// aliasAvailable checks the selected panel and the current item/field before
// describing a configured shortcut as an available action.
func (a *App) aliasAvailable(section, action string) bool {
	s := a.S
	if s.Busy {
		return false
	}
	if s.Popup != nil {
		if s.Popup.Blocking {
			return false
		}
		if section == "sync_popup" {
			return s.Popup.RemoteReview != nil
		}
		if section == "confirmation_popup" {
			return s.Popup.RemoteReview == nil
		}
		return false
	}
	if section == "folder_selector_popup" {
		return s.Mode == DialogFoldersChoice
	}
	if section == "authentication" || section == "master_password" {
		return s.Screen == AuthClient || s.Screen == AuthMaster
	}
	if section == "authentication_offline" {
		return (s.Screen == AuthClient || s.Screen == AuthMaster) && !s.ServerConnected
	}
	if section == "server_picker" {
		return s.Mode == DialogServer
	}
	modalSection := map[string]Mode{
		"create_item_popup": DialogCreate, "create_field_popup": DialogFieldType,
		"linked_field_popup": DialogLinkedChoice, "card_brand_popup": DialogCardBrand,
		"identity_title_popup": DialogIdentityTitle, "website_popup": DialogWebsite,
		"reprompt_popup": DialogReprompt, "name_popup": DialogName,
		"folder_popup": DialogFolder, "search": Find, "detail": Edit,
	}
	if mode, ok := modalSection[section]; ok {
		return s.Mode == mode
	}
	if section == "global" && action == "sync" {
		return s.Screen == Workspace && !s.LocalMode && s.ServerConnected
	}
	if section == "global" && action == "quit" {
		return !a.modalOpen()
	}
	if s.Screen != Workspace || s.Mode != Browse {
		return false
	}
	if s.LocalMode {
		if section == "global" && (action == "push_all" || action == "discard_all" || action == "focus_my_vault" || action == "focus_changes") {
			return false
		}
		if section == "items" && action != "search" {
			return false
		}
		if section == "edits" {
			return false
		}
		if section == "content" && action != "copy" && action != "reveal" {
			return false
		}
	}
	switch section {
	case "global":
		switch action {
		case "push_all", "discard_all":
			return s.Stage.Len() > 0 && (action != "push_all" || s.ServerConnected)
		case "focus_my_vault", "focus_item_types", "focus_folders",
			"focus_changes", "focus_items", "focus_content", "hint_next_page":
			return true
		}
	case "filters":
		return s.Focus == PaneVault || s.Focus == PaneTypes || s.Focus == PaneFolders
	case "folders":
		return s.Focus == PaneFolders && !s.LocalMode &&
			(action != "delete" || s.Filters[2] > 0 && s.Filters[2] <= len(s.Folders))
	case "items":
		if s.Focus != PaneItems {
			return false
		}
		if action == "delete" {
			return s.CurrentItem() != nil && s.CurrentItem().CanEdit()
		}
		return true
	case "edits":
		if s.Focus == PaneChanges && action == "discard" && s.ImportedChangeAt(s.ChangeIndex) {
			return true
		}
		return s.Focus == PaneChanges && s.Stage.Len() > 0 && !s.ImportedChangeAt(s.ChangeIndex) &&
			(action != "save" || s.ServerConnected)
	case "content":
		if s.Focus != PaneContent {
			return false
		}
		f := s.CurrentField()
		switch action {
		case "next", "previous", "back":
			return true
		case "new_field":
			return s.SelectedItem() != nil && s.SelectedItem().CanEdit()
		case "favorite":
			return s.SelectedItem() != nil && s.SelectedItem().CanEdit()
		case "open_detail":
			return f != nil && f.Editable
		case "copy":
			return f != nil && !f.Restricted
		case "reveal":
			return f != nil && f.Hidden && !f.Restricted
		case "edit":
			return f != nil && f.Editable && !isCheckboxField(f)
		case "switch":
			return f != nil && f.Editable && isCheckboxField(f)
		case "delete_field":
			return f != nil && f.Custom && f.Editable
		}
	}
	return false
}

// configuredShortcut resolves a keymap action to its existing UI transition.
func (a *App) configuredShortcut(section, action string) func() error {
	switch action {
	case "next", "previous":
		delta := 1
		if action == "previous" {
			delta = -1
		}
		return func() error { return a.arrow(delta, 0) }
	case "button_left":
		return func() error { return a.arrow(0, -1) }
	case "button_right":
		return func() error { return a.arrow(0, 1) }
	case "submit", "confirm", "select", "apply", "open", "save":
		return a.enter
	case "cancel", "leave", "back", "return_to_items":
		return a.escape
	case "quit":
		return func() error { a.requestQuit(); return nil }
	case "server":
		return func() error { a.openServerPicker(); return nil }
	case "sync":
		return func() error { a.checkRemote(); return nil }
	case "push_all":
		return func() error { a.confirmStage(true, true); return nil }
	case "discard_all":
		return func() error { a.confirmStage(false, true); return nil }
	case "reveal_secret", "reveal_password":
		return func() error { a.S.AuthVisible = !a.S.AuthVisible; return nil }
	case "hint_next_page", "next_page":
		return func() error { return a.char('?') }
	case "soft_sync":
		return func() error { return a.char('s') }
	case "hard_sync":
		return func() error { return a.char('h') }
	case "abort":
		return func() error { return a.char('a') }
	case "accept":
		return func() error { return a.char('y') }
	case "reject":
		return func() error { return a.char('n') }
	case "favorite":
		return func() error { return a.char('f') }
	case "switch":
		return func() error {
			if section == "content" {
				return a.S.SwitchCheckbox()
			}
			return a.char('s')
		}
	case "new", "new_field":
		return func() error { return a.char('n') }
	case "delete", "delete_field", "discard":
		return func() error { return a.char('d') }
	case "copy":
		return func() error { return a.char('c') }
	case "reveal":
		return func() error { return a.char('r') }
	case "edit", "open_detail":
		return func() error { return a.S.BeginEdit() }
	case "search":
		return func() error { return a.char('f') }
	case "newline":
		return func() error { a.S.Insert('\n'); return nil }
	}
	focus := map[string]rune{
		"focus_my_vault": '1', "focus_item_types": '2',
		"focus_folders": '3', "focus_changes": '4',
		"focus_items": '5', "focus_content": '6',
	}
	if r, ok := focus[action]; ok {
		return func() error { return a.char(r) }
	}
	return nil
}

func parseKey(s string) (interface{}, bool) {
	if strings.HasPrefix(s, "ctrl+") && len(s) == 6 {
		r := s[5]
		if r >= 'a' && r <= 'z' {
			return gocui.Key(int(r - 'a' + 1)), true
		}
	}
	switch strings.ToLower(s) {
	case "up":
		return gocui.KeyArrowUp, true
	case "down":
		return gocui.KeyArrowDown, true
	case "left":
		return gocui.KeyArrowLeft, true
	case "right":
		return gocui.KeyArrowRight, true
	case "enter":
		return gocui.KeyEnter, true
	case "escape", "esc":
		return gocui.KeyEsc, true
	case "tab":
		return gocui.KeyTab, true
	case "ctrl+q":
		return gocui.KeyCtrlQ, true
	case "ctrl+s":
		return gocui.KeyCtrlS, true
	case "ctrl+p":
		return gocui.KeyCtrlP, true
	case "ctrl+d":
		return gocui.KeyCtrlD, true
	case "ctrl+r":
		return gocui.KeyCtrlR, true
	case "ctrl+j", "ctrl+enter":
		return gocui.KeyCtrlJ, true
	}
	if utf8.RuneCountInString(s) == 1 {
		r, _ := utf8.DecodeRuneInString(s)
		return r, true
	}
	return nil, false
}

// Defaults documents the actions read from the existing JSONC keymap.
var Defaults = config.DefaultKeybinds

// aliasActionName renders a configured keybinding action in footer terminology.
func aliasActionName(action string) string {
	switch {
	case strings.HasPrefix(action, "focus_"):
		return "panels"
	case strings.HasPrefix(action, "save"):
		return "push"
	case strings.HasPrefix(action, "discard"):
		return "discard"
	case strings.HasPrefix(action, "delete"):
		return "delete"
	case strings.HasPrefix(action, "new"):
		return "new"
	}
	switch action {
	case "previous", "next", "button_left", "button_right":
		return "move"
	case "search":
		return "find"
	case "switch":
		return "switch"
	case "copy", "edit", "reveal", "quit":
		return action
	}
	return "unbound"
}
