package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/awesome-gocui/gocui"
	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/localvault"
	"github.com/miguelmnts/lazybw-go/internal/terminalkeys"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

func (a *App) bind(g *gocui.Gui) error {
	a.terminalBS = terminalBackspaceCode()
	textOnly := func(action func() error) func() error {
		return func() error {
			if a.textEntryActive() {
				return action()
			}
			return nil
		}
	}
	bindings := []struct {
		key      interface{}
		name     string
		fallback func() error
	}{
		{gocui.KeyArrowUp, "up", textOnly(func() error { return a.arrow(-1, 0) })},
		{gocui.KeyArrowDown, "down", textOnly(func() error { return a.arrow(1, 0) })},
		{gocui.KeyArrowLeft, "left", textOnly(func() error { return a.arrow(0, -1) })},
		{gocui.KeyArrowRight, "right", textOnly(func() error { return a.arrow(0, 1) })},
		{gocui.KeyCtrlW, "ctrl+w", func() error { return a.wordEdit(-1, true) }},
		{gocui.KeyEnter, "enter", nil},
		{gocui.KeyCtrlJ, "ctrl+enter", nil},
		{gocui.KeyEsc, "escape", nil},
		{gocui.KeyBackspace2, "backspace", func() error { return a.terminalBackspace(gocui.KeyBackspace2) }},
		{gocui.KeyBackspace, "backspace", func() error { return a.terminalBackspace(gocui.KeyBackspace) }},
		{gocui.KeyTab, "tab", func() error { return nil }},
		{gocui.KeyDelete, "delete", a.deleteForward},
		{gocui.KeyHome, "home", func() error {
			if !a.modalOpen() {
				a.S.Caret = 0
			}
			return nil
		}},
		{gocui.KeyEnd, "end", func() error {
			if !a.modalOpen() {
				if a.S.Screen != Workspace && a.S.AuthFocus < 2 {
					a.S.Caret = utf8.RuneCountInString(a.authValue())
				} else {
					a.S.Caret = utf8.RuneCountInString(a.S.Input)
				}
			}
			return nil
		}},
		{gocui.KeyCtrlQ, "ctrl+q", nil},
		{gocui.KeyCtrlS, "ctrl+s", nil},
		{gocui.KeyCtrlP, "ctrl+p", nil},
		{gocui.KeyCtrlD, "ctrl+d", nil},
		{gocui.KeyCtrlR, "ctrl+r", nil},
	}
	for _, entry := range bindings {
		spec := entry
		if err := g.SetKeybinding("", spec.key, gocui.ModNone,
			func(_ *gocui.Gui, _ *gocui.View) error {
				handled, err := a.dispatchConfiguredKey(spec.name)
				if handled {
					return err
				}
				if spec.fallback != nil {
					return a.handleKey(keyLabel(spec.key), spec.fallback)
				}
				if !a.textEntryActive() {
					a.reportKey(hintLabel(spec.name), "unbound")
				}
				return nil
			}); err != nil {
			return err
		}
	}
	for _, entry := range []struct {
		key    interface{}
		action func() error
	}{{gocui.MouseLeft, a.click},
		{gocui.MouseWheelUp, func() error { return a.arrow(-1, 0) }},
		{gocui.MouseWheelDown, func() error { return a.arrow(1, 0) }}} {
		spec := entry
		if err := g.SetKeybinding("", spec.key, gocui.ModNone,
			func(_ *gocui.Gui, _ *gocui.View) error { return a.quitResult(spec.action()) }); err != nil {
			return err
		}
	}
	if err := a.bindWordKeys(func(key interface{}, mod gocui.Modifier, fn func(*gocui.Gui, *gocui.View) error) error {
		return g.SetKeybinding("", key, mod, fn)
	}); err != nil {
		return err
	}
	return a.bindAliases(g)
}

// wordKeyBinding describes a terminal key event used for editing words.
type wordKeyBinding struct {
	key       interface{}
	modifier  gocui.Modifier
	label     string
	direction int
	remove    bool
}

// wordKeyBindings supports native Ctrl-modified keys and widely used Meta
// editing keys. No synthetic F-keys or terminal configuration is needed.
func wordKeyBindings() []wordKeyBinding {
	return []wordKeyBinding{
		{gocui.KeyArrowLeft, gocui.ModMouseCtrl, "ctrl+←", -1, false},
		{gocui.KeyArrowRight, gocui.ModMouseCtrl, "ctrl+→", 1, false},
		{gocui.KeyBackspace, gocui.ModMouseCtrl, "ctrl+backspace", -1, true},
		{gocui.KeyBackspace2, gocui.ModMouseCtrl, "ctrl+backspace", -1, true},
		{gocui.KeyDelete, gocui.ModMouseCtrl, "ctrl+delete", 1, true},
		{gocui.KeyArrowLeft, gocui.ModAlt, "alt+←", -1, false},
		{gocui.KeyArrowRight, gocui.ModAlt, "alt+→", 1, false},
		{gocui.KeyBackspace, gocui.ModAlt, "alt+backspace", -1, true},
		{gocui.KeyBackspace2, gocui.ModAlt, "alt+backspace", -1, true},
		{gocui.KeyDelete, gocui.ModAlt, "alt+delete", 1, true},
		{'b', gocui.ModAlt, "alt+b", -1, false},
		{'f', gocui.ModAlt, "alt+f", 1, false},
		{'d', gocui.ModAlt, "alt+d", 1, true},
	}
}

// bindWordKeys connects reported key events to the active editor instead of
// exercising editing methods without testing the input dispatch path.
func (a *App) bindWordKeys(register func(interface{}, gocui.Modifier, func(*gocui.Gui, *gocui.View) error) error) error {
	bindings := wordKeyBindings()
	for _, native := range a.nativeWordKeys {
		direction := 1
		remove := false
		switch native.Action {
		case terminalkeys.PreviousWord:
			direction = -1
		case terminalkeys.NextWord:
		case terminalkeys.DeleteNextWord:
			remove = true
		default:
			continue
		}
		bindings = append(bindings, wordKeyBinding{
			key: gocui.Key(native.Key), modifier: gocui.ModNone,
			label: native.Label, direction: direction, remove: remove,
		})
	}
	for _, binding := range bindings {
		spec := binding
		if err := register(spec.key, spec.modifier, func(_ *gocui.Gui, _ *gocui.View) error {
			return a.handleKey(spec.label, func() error {
				return a.wordEdit(spec.direction, spec.remove)
			})
		}); err != nil {
			return err
		}
	}
	return nil
}

// wordEdit never changes navigation outside a textbox. The lookup is
// shared by Master password, API credentials, Find and the detail editor.
func (a *App) wordEdit(direction int, remove bool) error {
	s := a.S
	if s.Popup != nil || s.Mode == DialogServer || s.Screen == AuthChecking ||
		a.creationTextInputLocked() {
		return nil
	}
	if s.Screen != Workspace || s.Mode == DialogReprompt {
		if s.Screen != Workspace && s.AuthFocus >= 2 {
			return nil
		}
		value := []rune(a.authValue())
		start := max(0, min(s.Caret, len(value)))
		end := WordBoundary(string(value), start, direction)
		if remove {
			if end < start {
				value = append(value[:end], value[start:]...)
				s.Caret = end
			} else if end > start {
				value = append(value[:start], value[end:]...)
			}
			a.setAuthValue(string(value))
		} else {
			s.Caret = end
		}
		return nil
	}
	if !a.inputMode() || (s.Mode == DialogName || s.Mode == DialogField ||
		s.Mode == DialogFolder) && s.FormFocus != 0 {
		return nil
	}
	if remove {
		s.DeleteWord(direction)
	} else {
		s.WordCursor(direction)
	}
	if s.Mode == Find {
		s.Search = s.Input
		s.recompute()
	}
	return nil
}

func (a *App) editEvent(_ *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if ch != 0 && mod == gocui.ModNone {
		name := string(ch)
		if handled, _ := a.dispatchConfiguredKey(name); handled {
			return
		}
		if a.textEntryActive() {
			_ = a.handleKey(name, func() error { return a.char(ch) })
		} else {
			a.reportKey(name, "unbound")
		}
	} else if key == gocui.KeySpace {
		if handled, _ := a.dispatchConfiguredKey("space"); handled {
			return
		}
		if a.textEntryActive() {
			_ = a.handleKey("space", func() error { return a.char(' ') })
		}
	} else if !a.textEntryActive() {
		a.reportKey(keyLabel(key), "unbound")
	}
}
func (a *App) inputMode() bool {
	s := a.S
	return s.Mode == DialogReprompt || s.Screen != Workspace && s.AuthFocus < 2 || s.Mode == Find || s.Mode == Edit || isCreationNameMode(s.Mode)
}

func isCreationNameMode(mode Mode) bool {
	return mode == DialogName || mode == DialogField || mode == DialogFolder
}

func (a *App) creationValidationPending() bool {
	return isCreationNameMode(a.S.Mode) &&
		(a.S.Busy || a.submitFlash == submitFlashDialog)
}

func (a *App) creationTextInputLocked() bool {
	return isCreationNameMode(a.S.Mode) &&
		(a.S.FormFocus != 0 || a.creationValidationPending())
}

func (a *App) char(r rune) error {
	s := a.S
	if s.Mode == DialogServer || s.Screen == AuthChecking || s.Screen == AuthOffline {
		return nil
	}
	if s.Popup != nil {
		if s.Popup.Blocking {
			return nil
		}
		if s.Popup.RemoteReview != nil {
			switch r {
			case 's', 'S':
				s.Popup.RemoteChoice = 0
				return a.enter()
			case 'h', 'H':
				if s.Stage.Len() == 0 {
					return nil
				}
				s.Popup.RemoteChoice = 1
				return a.enter()
			case 'a', 'A':
				s.Popup.RemoteChoice = 2
				return a.enter()
			}
		}
		if r == 'y' || r == 'Y' {
			s.Popup.Yes = true
			return a.enter()
		}
		if r == 'n' || r == 'N' {
			s.Popup.Yes = false
			return a.enter()
		}
		if r == 'c' || r == 'C' {
			s.Popup = nil
			return nil
		}
		if r == '?' {
			pageSize := 10
			if s.Popup.RemoteReview != nil {
				pageSize = a.reviewPageSize()
			}
			if len(s.Popup.Lines) > pageSize {
				s.Popup.Page = (s.Popup.Page + 1) % ((len(s.Popup.Lines) + pageSize - 1) / pageSize)
			}
		}
		return nil
	}
	if s.Mode == DialogFoldersChoice {
		if r == 's' || r == 'S' {
			if s.ChoiceIndex >= 0 && s.ChoiceIndex < len(s.Folders) {
				id := fmt.Sprint(s.Folders[s.ChoiceIndex]["id"])
				if s.FolderChoiceID == id {
					s.FolderChoiceID = ""
				} else {
					s.FolderChoiceID = id
				}
			}
		}
		return nil
	}
	if s.Mode == DialogCardBrand || s.Mode == DialogIdentityTitle {
		return nil
	}
	if s.Mode == DialogReprompt {
		text := []rune(s.MasterText())
		idx := max(0, min(s.Caret, len(text)))
		text = append(text[:idx], append([]rune{r}, text[idx:]...)...)
		if err := s.SetMasterText(string(text)); err != nil {
			return err
		}
		s.Caret++
		return nil
	}
	if s.Screen != Workspace {
		if !s.LocalMode && !s.ServerConnected &&
			(s.Screen == AuthClient || s.Screen == AuthMaster) {
			return nil
		}
		if s.AuthFocus >= 2 {
			return nil
		}
		text := []rune(a.authValue())
		idx := max(0, min(s.Caret, len(text)))
		text = append(text[:idx], append([]rune{r}, text[idx:]...)...)
		a.setAuthValue(string(text))
		s.Caret++
		return nil
	}
	if s.Busy && s.Screen == Workspace && s.Mode != Find {
		return nil
	}
	// A type picker accepts navigation and Enter, not character commands.
	if s.Mode == DialogCreate || s.Mode == DialogFieldType || s.Mode == DialogWebsite || s.Mode == DialogLinkedChoice {
		return nil
	}
	if a.inputMode() {
		if a.creationTextInputLocked() {
			return nil
		}
		s.Insert(r)
		if s.Mode == DialogName || s.Mode == DialogFolder || s.Mode == DialogField {
			s.FormError = ""
		}
		if s.Mode == Find {
			s.Search = s.Input
			s.recompute()
		}
		return nil
	}
	switch r {
	case '?':
		width := 90
		if a.gui != nil {
			width = max(18, 3*a.width()/4)
		}
		pages := PaginateHints(a.hints(), width)
		s.HintPage = (s.HintPage + 1) % len(pages)
	case '1', '2', '3', '4', '5', '6', '7':
		pane := paneForNumber(s, int(r-'0'))
		if pane == 0 {
			return nil
		}
		if pane == s.Focus {
			// The active panel number acts as Return rather than reselecting it.
			if pane == PaneContent {
				s.CloseItem()
			} else if pane != PaneItems {
				s.Focus = PaneItems
			}
		} else if pane <= PaneItems || (pane == PaneContent && s.SelectedID != "") {
			// Re-entering protected content must pass the re-prompt again.
			if pane == PaneContent {
				if item := s.SelectedItem(); item != nil && (item.Raw["reprompt"] == 1 || item.Raw["reprompt"] == float64(1)) && s.RepromptApprovedID != item.ID() {
					a.startReprompt(item.ID())
					return nil
				}
			}
			s.Focus = pane
			if pane <= PaneChanges {
				s.RepromptApprovedID = ""
			}
			if pane == PaneChanges {
				s.ChangeIndex = min(s.ChangeIndex, max(0, len(s.ChangesList())-1))
			}
		}
	case 'f':
		if s.Focus == PaneItems {
			s.Mode = Find
			s.Input = s.Search
			s.Caret = len([]rune(s.Input))
		} else if s.Focus == PaneContent {
			s.ToggleFavourite()
		}
	case 'n':
		if s.LocalMode {
			return nil
		}
		switch s.Focus {
		case PaneItems:
			s.NewType = 0
			s.FormFocus = 0
			s.FormError = ""
			s.Mode = DialogCreate
		case PaneFolders:
			if s.LocalMode {
				return nil
			}
			s.Input = ""
			s.Caret = 0
			s.FormFocus = 0
			s.FormError = ""
			s.Mode = DialogFolder
		case PaneContent:
			if s.LocalMode || s.SelectedItem() == nil || !s.SelectedItem().CanEdit() {
				return nil
			}
			s.Input = ""
			s.Caret = 0
			s.CustomType = 0
			s.FormFocus = 0
			s.FormError = ""
			s.Mode = DialogFieldType
		}
	case 'd':
		if s.LocalMode {
			return nil
		}
		switch s.Focus {
		case PaneItems:
			it := s.CurrentItem()
			if it != nil && it.CanEdit() {
				a.confirm("Delete item?", "Delete "+it.Name()+"?", nil, false, false, func(ok bool) {
					if !ok {
						return
					}
					// Re-verify even if Content was previously unlocked.
					if it.Raw["reprompt"] == 1 || it.Raw["reprompt"] == float64(1) {
						a.startReprompt(it.ID())
						s.RepromptDeleteID = it.ID()
						return
					}
					s.DeleteItem()
				})
			}
		case PaneChanges:
			if !s.ImportedChangeAt(s.ChangeIndex) {
				a.confirmStage(false, false)
			}
		case PaneFolders:
			if s.Filters[2] >= 1 && s.Filters[2] <= len(s.Folders) {
				folder := s.Folders[s.Filters[2]-1]
				a.confirm("Delete folder?", "Delete "+fmt.Sprint(folder["name"])+"?", nil, false, false, func(ok bool) {
					if ok {
						s.DeleteFolder()
					}
				})
			}
		case PaneContent:
			it := s.SelectedItem()
			f := s.CurrentField()
			if it != nil && it.CanEdit() && f != nil && f.Custom && f.Editable {
				a.confirm("Delete field?", "Delete "+f.Name+"?", nil, false, false, func(ok bool) {
					if !ok {
						return
					}
					arr, _ := it.Raw["fields"].([]any)
					idx := f.Path[1].(int)
					it.Raw["fields"] = append(arr[:idx], arr[idx+1:]...)
					s.Stage.Record("item", it.Raw, vault.TypeNames[it.Type()], s.Originals)
					s.loadFields()
				})
			}
		}
	case 'e':
		if s.Focus == PaneContent {
			return s.BeginEdit()
		}
	case 'c':
		if s.Focus == PaneContent {
			f := s.CurrentField()
			if f != nil && !f.Restricted {
				v, err := f.Reveal(s.CLI.Cipher)
				if err == nil {
					err = a.clipboard(v)
				}
				if err != nil {
					s.SetWorkspaceNotice("Clipboard copy failed", CRed)
				} else {
					s.SetWorkspaceNotice("Copied to clipboard", CWhite)
				}
			}
		}
	case 'r':
		if s.Focus == PaneContent {
			s.Reveal()
		}
	case 's':
		if s.LocalMode {
			return nil
		}
		if s.Focus == PaneChanges && !s.ImportedChangeAt(s.ChangeIndex) {
			a.confirmStage(true, false)
		} else if s.Focus == PaneContent {
			return s.SwitchCheckbox()
		}
	}
	return nil
}

// startReprompt opens a protected input without revealing the selected item.
func (a *App) startReprompt(id string) {
	s := a.S
	s.RepromptTargetID = id
	s.RepromptDeleteID = ""
	s.Focus = PaneItems
	s.RepromptApprovedID = ""
	s.RepromptError = ""
	s.ClearMaster()
	s.AuthVisible = false // A new master-password prompt always starts masked.
	s.Caret = 0
	s.Mode = DialogReprompt
}

// openItem enforces a master-password re-prompt before showing protected content.
func (a *App) openItem() {
	s := a.S
	item := s.CurrentItem()
	if item == nil {
		return
	}
	if item.Raw["reprompt"] == 1 || item.Raw["reprompt"] == float64(1) {
		a.startReprompt(item.ID())
		return
	}
	s.OpenItem()
}

func (a *App) authValue() string {
	s := a.S
	if s.Screen == AuthMaster || s.Mode == DialogReprompt {
		return s.MasterText()
	}
	if s.AuthFocus == 0 {
		return s.AuthID
	}
	return s.AuthSecret
}
func (a *App) setAuthValue(v string) {
	s := a.S
	if s.Screen == AuthMaster || s.Mode == DialogReprompt {
		if err := s.SetMasterText(v); err != nil {
			s.Notice = "Cannot encrypt the master password"
		}
	} else if s.AuthFocus == 0 {
		s.AuthID = v
	} else {
		s.AuthSecret = v
	}
}

// deleteForward removes the next character from any active text input.
func (a *App) deleteForward() error {
	s := a.S
	if s.Popup != nil || s.Mode == DialogServer || s.Screen == AuthChecking {
		return nil
	}
	if s.Mode == DialogReprompt {
		runes := []rune(s.MasterText())
		if s.Caret >= 0 && s.Caret < len(runes) {
			return s.SetMasterText(string(append(runes[:s.Caret], runes[s.Caret+1:]...)))
		}
		return nil
	}
	if s.Screen != Workspace && s.AuthFocus < 2 {
		runes := []rune(a.authValue())
		if s.Caret >= 0 && s.Caret < len(runes) {
			a.setAuthValue(string(append(runes[:s.Caret], runes[s.Caret+1:]...)))
		}
		return nil
	}
	if a.inputMode() {
		if a.creationTextInputLocked() {
			return nil
		}
		s.DeleteForward()
		if s.Mode == Find {
			s.Search = s.Input
			s.recompute()
		}
	}
	return nil
}

func (a *App) backspace() error {
	if a.S.Screen == AuthChecking || a.S.Mode == DialogServer {
		return nil
	}
	if a.S.Popup != nil {
		return nil
	}
	if a.S.Mode == DialogReprompt {
		r := []rune(a.S.MasterText())
		if a.S.Caret > 0 && a.S.Caret <= len(r) {
			if err := a.S.SetMasterText(string(append(r[:a.S.Caret-1], r[a.S.Caret:]...))); err != nil {
				return err
			}
			a.S.Caret--
		}
		return nil
	}
	if a.S.Screen != Workspace && a.S.AuthFocus < 2 {
		r := []rune(a.authValue())
		if a.S.Caret > 0 && a.S.Caret <= len(r) {
			a.setAuthValue(string(append(r[:a.S.Caret-1], r[a.S.Caret:]...)))
			a.S.Caret--
		}
		return nil
	}
	if a.inputMode() {
		if a.creationTextInputLocked() {
			return nil
		}
		a.S.Backspace()
		if a.S.Mode == Find {
			a.S.Search = a.S.Input
			a.S.recompute()
		}
	}
	return nil
}
func (a *App) arrow(vertical, horizontal int) error {
	s := a.S
	if s.Screen == AuthChecking || a.creationValidationPending() {
		return nil
	}
	if s.Screen == AuthOffline && s.Mode != DialogServer {
		a.requestQuit()
		return nil
	}
	if s.Mode == DialogServer {
		if vertical != 0 {
			choices := []int{}
			for i := 0; i < 2; i++ {
				if s.ServerReady[i] {
					choices = append(choices, i)
				}
			}
			if localvault.Available(s.Config.Local.VaultPath) {
				choices = append(choices, 2)
			}
			if len(choices) > 0 {
				index := 0
				for i, v := range choices {
					if v == s.ServerChoice {
						index = i
						break
					}
				}
				index = max(0, min(len(choices)-1, index+vertical))
				s.ServerChoice = choices[index]
			}
			s.ServerMoved = true
		}
		return nil
	}
	if s.Popup != nil {
		if s.Popup.Blocking {
			return nil
		}
		if s.Popup.RemoteReview != nil {
			if horizontal != 0 {
				if s.Stage.Len() == 0 {
					if horizontal < 0 {
						s.Popup.RemoteChoice = 0
					} else {
						s.Popup.RemoteChoice = 2
					}
				} else {
					s.Popup.RemoteChoice = max(0, min(2, s.Popup.RemoteChoice+horizontal))
				}
			}

			return nil
		}
		if s.Popup.Acknowledge {
			return nil
		}
		if horizontal != 0 {
			s.Popup.Yes = horizontal < 0
		}

		return nil
	}
	if s.Mode == DialogReprompt {
		if horizontal != 0 {
			s.Caret = max(0, min(len([]rune(s.MasterText())), s.Caret+horizontal))
		}
		return nil
	}
	if s.Screen != Workspace {
		if !s.LocalMode && !s.ServerConnected &&
			(s.Screen == AuthClient || s.Screen == AuthMaster) {
			if horizontal < 0 {
				s.AuthFocus = 2
			} else if horizontal > 0 {
				s.AuthFocus = 3
			}
			return nil
		}
		if s.AuthFocus >= 2 {
			if horizontal != 0 {
				if horizontal > 0 {
					s.AuthFocus = 3
				} else {
					s.AuthFocus = 2
				}
			} else if vertical != 0 {
				s.AuthFocus = 0
				if s.Screen == AuthClient && vertical < 0 {
					s.AuthFocus = 1
				}
				s.Caret = len([]rune(a.authValue()))
			}
		} else if horizontal != 0 {
			s.Caret = max(0, min(len([]rune(a.authValue())), s.Caret+horizontal))
		} else if vertical != 0 {
			if vertical > 0 {
				s.AuthFocus++
				if s.Screen == AuthMaster || s.AuthFocus > 1 {
					s.AuthFocus = 2
				}
			} else if s.Screen == AuthClient && s.AuthFocus == 0 {
				// Complete the loop: AUTH/QUIT -> Client ID with Down,
				// and Client ID -> AUTH with Up.
				s.AuthFocus = 2
			} else {
				s.AuthFocus = max(0, s.AuthFocus-1)
			}
			s.Caret = len([]rune(a.authValue()))
		}
		return nil
	}
	if s.Mode == DialogCreate || s.Mode == DialogFieldType {
		if vertical != 0 {
			if s.Mode == DialogCreate {
				s.NewType = max(0, min(len(vault.Schema().CreationOrder), s.NewType+vertical))
			} else {
				s.CustomType = max(0, min(len(fieldOptions(s.SelectedItem())), s.CustomType+vertical))
			}
		}
		return nil
	}
	if s.Mode == DialogFoldersChoice || s.Mode == DialogCardBrand ||
		s.Mode == DialogIdentityTitle {
		if vertical != 0 {
			limit := len(s.Folders)
			if s.Mode == DialogCardBrand {
				limit = len(cardBrands)
			} else if s.Mode == DialogIdentityTitle {
				limit = len(identityTitles)
			}
			s.ChoiceIndex = max(0, min(limit-1, s.ChoiceIndex+vertical))
		}
		return nil
	}
	if s.Mode == DialogLinkedChoice {
		if vertical != 0 {
			s.LinkedIndex = max(0, min(len(vault.Schema().LinkedTargets)-1, s.LinkedIndex+vertical))
		}
		return nil
	}
	if s.Mode == DialogWebsite {
		if vertical != 0 {
			s.WebsiteIndex = max(0, min(len(s.CurrentField().Values),
				s.WebsiteIndex+vertical))
		}
		return nil
	}
	if a.inputMode() {
		if s.Mode == Find && vertical < 0 {
			// Up from Find returns to the Items panel without clearing the query.
			s.Mode = Browse
			s.Focus = PaneItems
			return nil
		}
		if s.Mode == DialogName || s.Mode == DialogFolder || s.Mode == DialogField {
			if s.FormFocus == 0 && vertical > 0 {
				s.FormFocus = 1
				return nil
			}
			if s.FormFocus > 0 && vertical < 0 {
				s.FormFocus = 0
				return nil
			}
			if s.FormFocus > 0 && horizontal != 0 {
				if horizontal > 0 {
					s.FormFocus = 2
				} else {
					s.FormFocus = 1
				}
				return nil
			}
		}
		if horizontal != 0 {
			s.Cursor(horizontal)
		}
		return nil
	}
	if horizontal > 0 {
		if s.Focus == PaneItems {
			a.openItem()
		} else if s.Focus == PaneContent && !s.LocalMode {
			if isCheckboxField(s.CurrentField()) {
				return s.SwitchCheckbox()
			}
			return s.BeginEdit()
		}
		return nil
	}
	if horizontal < 0 {
		if s.Focus == PaneContent {
			s.CloseItem()
		} else if s.Focus >= PaneVault && s.Focus <= PaneChanges {
			s.Focus = PaneItems
		}
		return nil
	}
	if vertical == 0 {
		return nil
	}
	switch s.Focus {
	case PaneVault:
		s.Filters[0] = max(0, min(1, s.Filters[0]-vertical))
	case PaneTypes:
		s.Filters[1] = max(0, min(6, s.Filters[1]+vertical))
	case PaneFolders:
		s.Filters[2] = max(0, min(len(s.Folders), s.Filters[2]+vertical))
	case PaneChanges:
		s.ChangeIndex = max(0, min(len(s.ChangesList())-1, s.ChangeIndex+vertical))
	case PaneItems:
		s.ItemIndex = max(0, min(len(s.Filtered())-1, s.ItemIndex+vertical))
	case PaneContent:
		s.FieldIndex = max(0, min(len(s.Fields)-1, s.FieldIndex+vertical))
	}
	return nil
}

// submitFlashTarget tracks which button is briefly highlighted by Enter.
type submitFlashTarget uint8

const (
	submitFlashNone submitFlashTarget = iota
	submitFlashAuth
	submitFlashDialog
)

const submitFlashDuration = 150 * time.Millisecond

// keyboardSubmitTarget excludes mouse clicks and textboxes without a submit button.
func (a *App) keyboardSubmitTarget() submitFlashTarget {
	s := a.S
	if s.Busy || s.Popup != nil || s.Mode == DialogServer || s.Screen == AuthChecking {
		return submitFlashNone
	}
	if s.Screen == AuthClient || s.Screen == AuthMaster {
		if s.AuthFocus < 2 {
			return submitFlashAuth
		}
		return submitFlashNone
	}
	if (s.Mode == DialogName || s.Mode == DialogField || s.Mode == DialogFolder) &&
		s.FormFocus == 0 {
		return submitFlashDialog
	}
	return submitFlashNone
}

// keyboardEnter paints the submit button before running its existing action.
func (a *App) keyboardEnter() error {
	target := a.keyboardSubmitTarget()
	if target == submitFlashNone || a.gui == nil {
		return a.enter()
	}
	if a.submitFlash != submitFlashNone {
		return nil
	}
	screen, mode, focus := a.S.Screen, a.S.Mode, a.S.AuthFocus
	formFocus := a.S.FormFocus
	a.submitFlash = target
	time.AfterFunc(submitFlashDuration, func() {
		a.gui.Update(func(*gocui.Gui) error {
			if a.submitFlash != target {
				return nil
			}
			a.submitFlash = submitFlashNone
			if a.S.Screen != screen || a.S.Mode != mode ||
				a.S.AuthFocus != focus || a.S.FormFocus != formFocus ||
				a.keyboardSubmitTarget() != target {
				return nil
			}
			return a.enter()
		})
	})
	return nil
}

func creationNameError(mode Mode) string {
	switch mode {
	case DialogField:
		return "Field name is required"
	case DialogFolder:
		return "Folder name is required"
	default:
		return "Item name is required"
	}
}

func (a *App) enter() error {
	s := a.S
	if s.Mode == DialogReprompt {
		if !s.CLI.VerifyReprompt(s.MasterText()) {
			s.RepromptError = "Incorrect master password"
			return nil
		}
		s.RepromptApprovedID = s.RepromptTargetID
		s.RepromptTargetID = ""
		s.RepromptError = ""
		s.ClearMaster()
		s.Mode = Browse
		if deleteID := s.RepromptDeleteID; deleteID != "" {
			s.RepromptDeleteID = ""
			// Never delete a different item if the list changed while the
			// password prompt was open.
			if item := s.CurrentItem(); item != nil && item.ID() == deleteID {
				s.DeleteItem()
			}
			s.RepromptApprovedID = ""
			return nil
		}
		s.OpenItem()
		return nil
	}
	if s.Screen == AuthChecking {
		return nil
	}
	if s.Screen == AuthOffline && s.Mode != DialogServer {
		a.requestQuit()
		return nil
	}
	if s.Mode == DialogServer {
		a.selectServerChoice()
		return nil
	}

	if s.Popup != nil {
		if s.Popup.Blocking {
			return nil
		}
		p := s.Popup
		if s.Busy {
			return nil
		}
		s.Popup = nil
		if p.RemoteReview != nil {
			if p.RemoteChoice != 2 && p.Action != nil {
				p.Action(p.RemoteChoice == 0)
			}
			return nil
		}
		if p.Acknowledge {
			return nil
		}
		if p.Action != nil {
			p.Action(p.Yes)
		}
		return nil
	}
	if s.Screen != Workspace {
		if s.AuthFocus == 2 && !s.LocalMode && !s.ServerConnected {
			if localvault.Available(s.Config.Local.VaultPath) {
				a.switchToLocal()
			}
			return nil
		}
		if s.AuthFocus == 3 {
			a.requestQuit()
			return nil
		}
		a.submitAuth()
		return nil
	}
	if s.Busy {
		return nil
	}
	if isCreationNameMode(s.Mode) {
		if s.FormFocus != 2 && strings.TrimSpace(s.Input) == "" {
			s.FormError = creationNameError(s.Mode)
			return nil
		}
		s.FormError = ""
	}
	switch s.Mode {
	case DialogFoldersChoice:
		it := s.SelectedItem()
		if it != nil && it.CanEdit() {
			if s.FolderChoiceID == "" {
				it.Raw["folderId"] = nil
			} else {
				it.Raw["folderId"] = s.FolderChoiceID
			}
			s.Stage.Record("item", it.Raw, vault.TypeNames[it.Type()], s.Originals)
			s.loadFields()
		}
		s.Mode = Browse
		return nil
	case DialogCardBrand, DialogIdentityTitle:
		it := s.SelectedItem()
		if it == nil || !it.CanEdit() {
			s.Mode = Browse
			return nil
		}
		if s.Mode == DialogCardBrand && s.ChoiceIndex == len(cardBrands)-1 {
			s.Mode = Browse
			return s.BeginTextEdit()
		}
		value := ""
		if s.Mode == DialogCardBrand {
			value = cardBrands[s.ChoiceIndex]
		} else {
			value = identityTitles[s.ChoiceIndex]
		}
		if err := it.Set(s.CurrentField().Path, value); err != nil {
			return err
		}
		s.Stage.Record("item", it.Raw, vault.TypeNames[it.Type()], s.Originals)
		s.loadFields()
		s.Mode = Browse
		return nil
	case DialogCreate:
		if s.NewType >= len(vault.Schema().CreationOrder) {
			s.Mode = Browse
			return nil
		}
		s.Mode = DialogName
		s.FormFocus = 0
		s.FormError = ""
		s.Input = ""
		s.Caret = 0
		return nil
	case DialogFieldType:
		options := fieldOptions(s.SelectedItem())
		if s.CustomType >= len(options) {
			s.Mode = Browse
			return nil
		}
		if options[s.CustomType].Name == "Website" {
			s.BeginNewWebsite()
			return nil
		}
		s.Mode = DialogField
		s.FormFocus = 0
		s.FormError = ""
		s.Input = ""
		s.Caret = 0
		return nil
	case DialogLinkedChoice:
		it := s.SelectedItem()
		if it == nil || !it.CanEdit() || it.Type() != vault.Login {
			s.Mode = Browse
			return nil
		}
		if s.LinkedIndex < 0 || s.LinkedIndex >= len(vault.Schema().LinkedTargets) {
			return nil
		}
		linkedID := vault.Schema().LinkedTargets[s.LinkedIndex].ID
		wasEditing := s.LinkedEdit
		if s.LinkedEdit {
			f := s.CurrentField()
			if f != nil && f.Linked && len(f.Path) == 3 {
				arr, _ := it.Raw["fields"].([]any)
				idx, _ := f.Path[1].(int)
				if idx >= 0 && idx < len(arr) {
					entry, _ := arr[idx].(map[string]any)
					entry["linkedId"] = linkedID
				}
			}
		} else {
			arr, _ := it.Raw["fields"].([]any)
			it.Raw["fields"] = append(arr, map[string]any{"name": s.NewFieldName, "type": 3, "value": nil, "linkedId": linkedID})
		}
		s.Stage.Record("item", it.Raw, vault.TypeNames[it.Type()], s.Originals)
		s.Mode = Browse
		s.LinkedEdit = false
		s.NewFieldName = ""
		s.loadFields()
		if !wasEditing {
			s.FieldIndex = max(0, len(s.Fields)-1)
		}
		return nil
	case DialogWebsite:
		f := s.CurrentField()
		if f == nil || !f.Websites || s.WebsiteIndex >= len(f.Values) {
			s.Mode = Browse
			return nil
		}
		return s.BeginEditWebsite(s.WebsiteIndex)
	case DialogName:
		if s.FormFocus == 2 {
			s.Mode = Browse
			s.Input = ""
			return nil
		}
		if strings.TrimSpace(s.Input) != "" {
			s.CreateItem(vault.Schema().CreationOrder[s.NewType], strings.TrimSpace(s.Input))
		}
		s.Mode = Browse
		s.Input = ""
		return nil
	case DialogFolder:
		if s.FormFocus == 2 {
			s.Mode = Browse
			s.Input = ""
			return nil
		}
		if strings.TrimSpace(s.Input) != "" {
			s.CreateFolder(strings.TrimSpace(s.Input))
		}
		s.Mode = Browse
		s.Input = ""
		return nil
	case DialogField:
		if s.FormFocus == 2 {
			s.Mode = Browse
			s.Input = ""
			return nil
		}
		it := s.SelectedItem()
		if it != nil && strings.TrimSpace(s.Input) != "" && s.CustomType == 3 && it.Type() == vault.Login {
			s.NewFieldName = strings.TrimSpace(s.Input)
			s.LinkedIndex = 0
			s.LinkedEdit = false
			s.Mode = DialogLinkedChoice
			s.Input = ""
			return nil
		}
		if it != nil && strings.TrimSpace(s.Input) != "" && s.CustomType >= 0 && s.CustomType < len(customFieldOptions) {
			option := customFieldOptions[s.CustomType]
			arr, _ := it.Raw["fields"].([]any)
			it.Raw["fields"] = append(arr, map[string]any{
				"name":  strings.TrimSpace(s.Input),
				"type":  option.BitwardenType,
				"value": option.InitialValue,
			})
			s.Stage.Record("item", it.Raw, vault.TypeNames[it.Type()], s.Originals)
			s.loadFields()
			s.FieldIndex = max(0, len(s.Fields)-1)
		}
		s.Mode = Browse
		s.Input = ""
		return nil
	case Find:
		s.Mode = Browse
		s.Focus = PaneItems
		return nil
	case Edit:
		return s.CommitEdit()
	}
	switch s.Focus {
	case PaneVault:
		s.FilterVault = s.Filters[0]
		s.recompute()
	case PaneTypes:
		s.FilterType = s.Filters[1]
		s.recompute()
	case PaneFolders:
		if s.FilterFolder == s.Filters[2] {
			s.FilterFolder = -1
		} else {
			s.FilterFolder = s.Filters[2]
		}
		s.recompute()
	case PaneChanges:
		if !s.ImportedChangeAt(s.ChangeIndex) {
			a.confirmStage(true, false)
		}
	case PaneItems:
		a.openItem()
	case PaneContent:
		if !s.LocalMode {
			if isCheckboxField(s.CurrentField()) {
				return s.SwitchCheckbox()
			}
			return s.BeginEdit()
		}
	}
	return nil
}
func (a *App) escape() error {
	s := a.S
	if s.Screen == AuthChecking {
		return nil
	}
	if s.Screen == AuthOffline && s.Mode != DialogServer {
		a.requestQuit()
		return nil
	}
	if s.Mode == DialogServer {
		a.closeServerPicker()
		return nil
	}
	if s.Popup != nil {
		if s.Popup.Blocking {
			return nil
		}
		if s.Busy {
			return nil
		}
		s.Popup = nil
		return nil
	}
	if s.Screen != Workspace {
		a.requestQuit()
		return nil
	}
	switch s.Mode {
	case DialogCreate, DialogName, DialogFieldType, DialogFolder, DialogField,
		DialogWebsite, DialogLinkedChoice, DialogFoldersChoice, DialogCardBrand,
		DialogIdentityTitle, DialogReprompt, Edit:
		if s.Mode == DialogReprompt {
			s.RepromptTargetID = ""
			s.RepromptDeleteID = ""
			s.RepromptError = ""
			s.ClearMaster()
		}
		s.Mode = Browse
		s.FormError = ""
		if s.Focus == PaneDetail {
			s.ExitEdit()
		}
		s.Input = ""
	case Find:
		s.Mode = Browse
		s.Focus = PaneItems
	default:
		if s.Focus == PaneContent {
			s.CloseItem()
		} else {
			s.Focus = PaneItems
		}
	}
	return nil
}

// authClientCredentials uses the saved key only for the default region and
// only when the user has not started entering another pair of credentials.
func (a *App) authClientCredentials() (string, string) {
	s := a.S
	if s.AuthID != "" || s.AuthSecret != "" ||
		s.Server != s.Config.Authentication.DefaultServer ||
		s.Config.Authentication.ClientEnvPath == "" {
		return s.AuthID, s.AuthSecret
	}
	id, secret, err := bitwarden.ReadEnvCredentials(s.Config.Authentication.ClientEnvPath)
	if err != nil {
		return "", ""
	}
	return id, secret
}

func (a *App) submitAuth() {
	s := a.S
	if s.Busy {
		return
	}
	var step = s.Screen
	id, secret, password := s.AuthID, s.AuthSecret, s.MasterText()
	if step == AuthMaster && password == "" {
		if s.LocalMode {
			if h, e := localvault.Inspect(s.Config.Local.VaultPath); e == nil {
				s.Notice = "Password differs from the one used to encrypt the local vault on " + h.Created
				return
			}
		}
		s.Notice = "Incorrect master password. Try again"
		return
	}
	if step == AuthClient {
		id, secret = a.authClientCredentials()
		if id == "" || secret == "" {
			s.Notice = "Enter your client ID and client secret, or check client_env_path"
			return
		}
	}
	s.Busy = true
	s.BusyMessage = "Authenticating"
	go func() {
		ctx := context.Background()
		var err error
		if s.LocalMode {
			result, header, openErr := localvault.Decode(s.Config.Local.VaultPath,
				password, s.Config.Local.VaultEncryption)
			if openErr != nil {
				err = openErr
			} else {
				items := make([]*vault.Item, 0, len(result.Items))
				for _, raw := range result.Items {
					if sealErr := vault.SealItem(raw, s.CLI.Cipher); sealErr != nil {
						err = sealErr
						break
					}
					items = append(items, &vault.Item{Raw: raw, Cipher: s.CLI.Cipher})
				}
				if err == nil {
					vault.SortedItems(items)
					a.gui.Update(func(*gocui.Gui) error {
						s.Busy = false
						s.LocalOrigin = header.Origin
						s.Loaded(items, result.Folders)
						s.ClearMaster()
						return nil
					})
					return
				}
				// Errors must flow through the common completion callback,
				// which clears Busy and permits another unlock attempt.
			}
		} else if step == AuthClient {
			err = s.CLI.Login(ctx, id, secret)
		} else {
			err = s.CLI.Unlock(ctx, password)
		}
		var backupSession *localvault.Session
		if err == nil && step == AuthMaster && !s.LocalMode &&
			s.Config.Local.SaveVault && a.localWarning() == "" {
			backupSession, err = localvault.NewSession(password, s.CLI.Cipher)
		}
		a.gui.Update(func(g *gocui.Gui) error {
			s.Busy = false
			if err != nil {
				if s.LocalMode {
					s.Notice = err.Error()
				} else {
					s.Notice = bitwarden.AuthenticationMessage(err, step == AuthMaster)
				}
				return nil
			}
			s.Notice = ""
			s.Caret = 0
			if step == AuthClient {
				s.Screen = AuthMaster
				s.LocalWarning = a.localWarning()
				s.AuthFocus = 0
				s.AuthSecret = ""
			} else {
				if backupSession != nil {
					if s.LocalSession != nil {
						s.LocalSession.Close()
					}
					s.LocalSession = backupSession
				}
				s.BeginVaultLoading()
				go a.loadVault(ctx, g)
			}
			return nil
		})
	}()
}
func (a *App) click() error {
	if a.S.Screen == AuthChecking {
		return nil
	}
	x, y := a.gui.MousePosition()
	return a.clickAt(x, y)
}

// clickAt handles mouse coordinates in the same canvas used for rendering.
func (a *App) clickAt(x, y int) error {
	s := a.S
	if s.Screen == AuthChecking {
		return nil
	}
	if s.Popup != nil {
		if s.Popup.Blocking {
			return nil
		}
		p := s.Popup
		by := a.dialogY + a.dialogH - 4
		if y >= by && y <= by+1 {
			if p.Acknowledge {
				okX := a.dialogX + (a.dialogW-10)/2
				if x < okX || x >= okX+10 {
					return nil
				}
				return a.enter()
			}
			if p.RemoteReview != nil {
				clicked := -1
				buttons := remoteReviewButtons(a.dialogX, a.dialogW)
				if s.Stage.Len() == 0 {
					buttons[0] = remoteReviewButton{a.dialogX + (a.dialogW-30)/3, 13, "SYNC", CGreen}
					buttons[2] = remoteReviewButton{a.dialogX + (a.dialogW-30)*2/3 + 13, 14, "ABORT SYNC", CRed}
				}
				for i, b := range buttons {
					if s.Stage.Len() == 0 && i == 1 {
						continue
					}
					if x >= b.x && x < b.x+b.width {
						clicked = i
						break
					}
				}
				if clicked < 0 {
					return nil
				}
				p.RemoteChoice = clicked
			} else {
				yesX, yesWidth, noX, noWidth := popupConfirmationButtonBounds(
					p, a.dialogX, a.dialogW)
				switch {
				case x >= yesX && x < yesX+yesWidth:
					p.Yes = true
				case x >= noX && x < noX+noWidth:
					p.Yes = false
				default:
					return nil
				}
			}
			return a.enter()
		}
		return nil
	}
	if s.Mode == DialogServer {
		w, h := 100, 32
		if a.gui != nil {
			w, h = a.width(), a.height()
		}
		dx, dy, dw, _ := serverPickerGeometry(w, h)
		if x >= dx+3 && x < dx+dw-3 && y >= dy+3 && y <= dy+5 {
			choice := y - dy - 3
			if choice == 2 && !localvault.Available(s.Config.Local.VaultPath) {
				return nil
			}
			if choice < 2 && !s.ServerReady[choice] {
				return nil
			}
			s.ServerChoice = choice
			s.ServerMoved = true
			return a.enter()
		}
		return nil
	}
	if s.Screen == AuthOffline {
		w, h := a.width(), a.height()
		if y >= h/2 && y <= h/2+3 && x >= w/2-6 && x < w/2+6 {
			a.requestQuit()
		}
		return nil
	}
	if s.Screen != Workspace {
		return a.authClickAt(x, y, a.width(), a.height())
	}
	if s.Mode == DialogCreate || s.Mode == DialogFieldType || s.Mode == DialogWebsite ||
		s.Mode == DialogLinkedChoice || s.Mode == DialogFoldersChoice ||
		s.Mode == DialogCardBrand || s.Mode == DialogIdentityTitle {
		base := max(2, (a.height()-16)/2)
		limit := 5
		if s.Mode == DialogLinkedChoice {
			limit = 1
		}
		if s.Mode == DialogFoldersChoice {
			limit = len(s.Folders) - 1
		} else if s.Mode == DialogCardBrand {
			limit = len(cardBrands) - 1
		} else if s.Mode == DialogIdentityTitle {
			limit = len(identityTitles) - 1
		} else if s.Mode == DialogFieldType {
			limit = len(fieldOptions(s.SelectedItem()))
		} else if s.Mode == DialogWebsite {
			limit = len(s.CurrentField().Values)
			h := min(max(9, limit+5), a.height()-4)
			base = max(2, (a.height()-h)/2)
		}
		if s.Mode == DialogFoldersChoice || s.Mode == DialogCardBrand ||
			s.Mode == DialogIdentityTitle {
			count := limit + 1
			h := min(max(9, count+5), a.height()-4)
			base = max(2, (a.height()-h)/2)
		}
		w := min(65, a.width()-6)
		x0 := (a.width() - w) / 2
		if x < x0+3 || x >= x0+w-3 {
			return nil
		}
		idx := y - base - 3
		if s.Mode == DialogWebsite || s.Mode == DialogFoldersChoice ||
			s.Mode == DialogCardBrand || s.Mode == DialogIdentityTitle {
			capacity := max(1, min(max(9, limit+5), a.height()-4)-4)
			selected := s.ChoiceIndex
			if s.Mode == DialogWebsite {
				selected = s.WebsiteIndex
			}
			if selected >= capacity {
				idx += selected - capacity + 1
			}
		}
		if idx >= 0 && idx <= limit {
			switch s.Mode {
			case DialogCreate:
				s.NewType = idx
			case DialogFieldType:
				s.CustomType = idx
			case DialogWebsite:
				s.WebsiteIndex = idx
			case DialogLinkedChoice:
				s.LinkedIndex = idx
			case DialogFoldersChoice, DialogCardBrand, DialogIdentityTitle:
				s.ChoiceIndex = idx
			}
			if s.Mode == DialogFoldersChoice {
				return a.char('s')
			}
			return a.enter()
		}
		return nil
	}
	if isCreationNameMode(s.Mode) {
		if a.creationValidationPending() {
			return nil
		}
		w := min(65, a.width()-6)
		x0 := (a.width() - w) / 2
		cy := max(2, (a.height()-16)/2)
		if y >= cy+10 && y <= cy+11 {
			submitX := x0 + 9
			submitWidth := symmetricButtonWidth("SUBMIT", 16)
			cancelX := x0 + w - 26
			cancelWidth := symmetricButtonWidth("CANCEL", 16)
			switch {
			case x >= submitX && x < submitX+submitWidth:
				s.FormFocus = 1
			case x >= cancelX && x < cancelX+cancelWidth:
				s.FormFocus = 2
			default:
				return nil
			}
			return a.enter()
		}
		if y == cy+5 && x >= x0+4 && x < x0+w-4 {
			s.FormFocus = 0
			s.Caret = min(len([]rune(s.Input)), max(0, x-x0-4))
		}
		return nil
	}
	if s.Mode == DialogReprompt {
		return nil
	}
	if s.Mode == Edit {
		return nil
	}
	if x >= 1 && x < a.leftWidth {
		for j := 0; j < 4; j++ {
			if s.LocalMode && (j == 0 || j == 3) {
				continue
			}
			start := a.sideY[j]
			if y >= start && y < start+a.sideH[j] {
				s.RepromptApprovedID = ""
				s.Focus = Pane(j + 1)
				index := y - start - 2 + a.sideScroll[j]
				// Blank rows focus the pane and highlight its applied filter
				// (or the first option), but never activate a filter.
				count := 0
				switch s.Focus {
				case PaneVault:
					count = 2
				case PaneTypes:
					count = 7
				case PaneFolders:
					count = 1 + len(s.Folders)
				case PaneChanges:
					count = len(s.ChangesList())
				}
				if y < start+2 || y >= start+a.sideH[j]-1 ||
					index < 0 || index >= count {
					a.selectAppliedSidebarRow()
					return nil
				}
				switch s.Focus {
				case PaneVault:
					s.Filters[0] = 1 - index
				case PaneTypes:
					s.Filters[1] = index
				case PaneFolders:
					s.Filters[2] = index
				case PaneChanges:
					s.ChangeIndex = index
				}
				return a.enter()
			}
		}
		return nil
	}
	if s.SelectedID != "" && a.contentX > 0 && x >= a.contentX &&
		x < a.contentX+a.contentW && (a.detailY == 0 || y < a.detailY-1) {
		rowY := 3
		limitY := a.height() - 3
		if a.detailY > 0 {
			limitY = a.detailY - 1
		}
		if y >= limitY {
			return nil
		}
		for index := a.contentScroll; index < len(s.Fields); index++ {
			rowHeight := min(fieldRowHeight(s.Fields[index]), limitY-rowY)
			if y >= rowY && y < rowY+rowHeight {
				s.Focus = PaneContent
				s.FieldIndex = index
				return a.enter()
			}
			rowY += rowHeight
		}
		return nil
	}
	if s.SelectedID != "" && a.contentX > 0 &&
		x >= a.contentX && x < a.contentX+a.contentW &&
		y >= 0 && y < a.height()-3 {
		s.Focus = PaneContent
		return nil
	}
	if x >= a.itemX && x < a.itemX+a.itemW {
		// Clicking the Name / Owner / State title bar focuses Items without
		// opening an entry or changing the selected item.
		if y == 1 {
			s.Focus = PaneItems
			return nil
		}
		if y == 2 {
			return nil // Header separator is not a focusable panel area.
		}
		// Find occupies the bottom row of Items; clicking it toggles focus.
		if y == a.height()-4 {
			if s.Mode == Find {
				s.Mode = Browse
				s.Focus = PaneItems
			} else {
				s.Mode = Find
				s.Input = s.Search
				s.Caret = len([]rune(s.Input))
			}
			return nil
		}
		// Only the actual, visible item rectangles are clickable. Clicking
		// separators, empty list space or footer does not select an item.
		items := s.Filtered()
		if index, ok := itemHitIndex(x, y, a.itemX, a.itemW,
			a.height(), a.itemScroll, len(items)); ok {
			s.Focus = PaneItems
			s.ItemIndex = index
			return a.enter()
		}
		// Empty space inside Items leaves Find and transfers keyboard focus.
		s.Mode = Browse
		s.Focus = PaneItems
	}
	return nil
}

// selectAppliedSidebarRow highlights the active filter after a blank-area click.
// This changes only the keyboard selection; it does not apply another filter.
func (a *App) selectAppliedSidebarRow() {
	s := a.S
	switch s.Focus {
	case PaneVault:
		s.Filters[0] = s.FilterVault
	case PaneTypes:
		s.Filters[1] = s.FilterType
	case PaneFolders:
		s.Filters[2] = max(0, min(len(s.Folders), s.FilterFolder))
	case PaneChanges:
		s.ChangeIndex = max(0, min(len(s.ChangesList())-1, s.ChangeIndex))
	}
}

// authClickAt uses the shared form geometry without requiring a GUI in tests.
// Both authentication screens use the same button hitboxes and quit path.
func (a *App) authClickAt(x, y, width, height int) error {
	s := a.S
	g := authGeometry(width, height, s.Screen)
	if y >= g.buttonY-1 && y <= g.buttonY+1 {
		if !s.LocalMode && !s.ServerConnected {
			leftX, leftW, rightX, rightW := authOfflineButtons(width)
			if x >= leftX && x < leftX+leftW {
				s.AuthFocus = 2
				return a.enter()
			}
			if x >= rightX && x < rightX+rightW {
				s.AuthFocus = 3
				return a.enter()
			}
			return nil
		}
		if x >= g.enterX && x < g.enterX+g.buttonWidth {
			s.AuthFocus = 2
			return a.enter()
		}
		if x >= g.quitX && x < g.quitX+g.buttonWidth {
			s.AuthFocus = 3
			return a.enter()
		}
	}
	if y == g.firstY {
		s.AuthFocus = 0
		s.Caret = len([]rune(a.authValue()))
	}
	if s.Screen == AuthClient && y == g.secondY {
		s.AuthFocus = 1
		s.Caret = len([]rune(a.authValue()))
	}
	return nil
}

func (a *App) width() int {
	if a.gui == nil {
		return a.renderWidth
	}
	w, _ := a.gui.Size()
	return w
}

func (a *App) height() int {
	if a.gui == nil {
		return a.renderHeight
	}
	_, h := a.gui.Size()
	return h
}

// itemHitIndex accepts only cells within a rendered item rectangle, not
// the header, search box, separators, footer or empty scroll padding.
func itemHitIndex(x, y, panelX, panelW, height, scroll, count int) (int, bool) {
	if x < panelX || x >= panelX+panelW || y < 3 {
		return 0, false
	}
	row := (y - 3) / itemRowHeight
	visibleRows := max(0, (height-8)/itemRowHeight)
	index := scroll + row
	if row >= visibleRows || index < 0 || index >= count {
		return 0, false
	}
	return index, true
}

// toggleAuthVisibility reveals or masks authentication input without altering it.
func (a *App) toggleAuthVisibility() error {
	if a.S.Screen == Workspace && a.S.Mode == Edit &&
		a.S.Focus == PaneDetail && a.S.InputSecret {
		// Toggle the same field-specific state used by Content.
		a.S.Reveal()
		a.S.AuthVisible = a.S.Revealed[a.S.RevealKey()]
	} else if a.S.Mode == DialogReprompt ||
		((a.S.Screen == AuthClient || a.S.Screen == AuthMaster) &&
			!a.modalOpen()) {
		a.S.AuthVisible = !a.S.AuthVisible
	}
	return nil
}
