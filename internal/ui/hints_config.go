package ui

import "strings"

type hintAction struct{ section, action string }

// hintLabel formats the preferred binding while preserving special key names.
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

// hints uses the first configured key of each action in the footer.
func (a *App) hints() []string {
	hints := a.baseHints()
	s := a.S
	section := ""
	if s.Popup != nil {
		if s.Popup.RemoteReview != nil {
			section = "sync_popup"
		} else {
			section = "confirmation_popup"
		}
	} else {
		switch s.Mode {
		case DialogFoldersChoice:
			section = "folder_selector_popup"
		case DialogCreate:
			section = "create_item_popup"
		case DialogFieldType:
			section = "create_field_popup"
		case DialogLinkedChoice:
			section = "linked_field_popup"
		case DialogCardBrand:
			section = "card_brand_popup"
		case DialogIdentityTitle:
			section = "identity_title_popup"
		case DialogWebsite:
			section = "website_popup"
		case DialogReprompt:
			section = "reprompt_popup"
		case DialogFolder:
			section = "folder_popup"
		case DialogName, DialogField:
			section = "name_popup"
		case Find:
			section = "search"
		case Edit:
			section = "detail"
		default:
			switch s.Focus {
			case PaneVault, PaneTypes:
				section = "filters"
			case PaneFolders:
				section = "folders"
			case PaneItems:
				section = "items"
			case PaneContent:
				section = "content"
			case PaneChanges:
				section = "edits"
			}
		}
	}

	for i, label := range hints {
		if label == "↑/↓ move" {
			previous := s.Config.Key(section, "previous")
			next := s.Config.Key(section, "next")
			if previous != "" && next != "" {
				hints[i] = hintLabel(previous) + "/" + hintLabel(next) + " move"
			} else {
				hints[i] = ""
			}
			continue
		}
		if label == "←/→ move" || label == "←/→ choose" {
			previous := s.Config.Key(section, "button_left")
			next := s.Config.Key(section, "button_right")
			if previous != "" && next != "" {
				verb := "choose"
				if label == "←/→ move" {
					verb = "move"
				}
				hints[i] = hintLabel(previous) + "/" + hintLabel(next) + " " + verb
			} else {
				hints[i] = ""
			}
			continue
		}
		if label == "ctrl+q quit" || label == "ctrl+s sync" ||
			label == "ctrl+p push all" || label == "ctrl+d discard all" {
			action := map[string]string{
				"ctrl+q quit": "quit", "ctrl+s sync": "sync",
				"ctrl+p push all": "push_all", "ctrl+d discard all": "discard_all",
			}[label]
			if key := s.Config.Key("global", action); key != "" {
				hints[i] = hintLabel(key) + " " + strings.SplitN(label, " ", 2)[1]
			} else {
				hints[i] = ""
			}
			continue
		}
		action := configuredHintAction(section, label)
		if action == "" {
			continue
		}
		if key := s.Config.Key(section, action); key != "" {
			parts := strings.SplitN(label, " ", 2)
			if len(parts) == 2 {
				hints[i] = hintLabel(key) + " " + parts[1]
			}
		} else {
			hints[i] = ""
		}
	}
	visible := hints[:0]
	for _, hint := range hints {
		if hint != "" && hint != " panels" {
			visible = append(visible, hint)
		}
	}
	return visible
}

func (a *App) configuredHint(section, action, verb string) string {
	if key := a.S.Config.Key(section, action); key != "" {
		return hintLabel(key) + " " + verb
	}
	return ""
}

func (a *App) configuredMoveHint(section string, horizontal bool) string {
	keys := []string{
		a.S.Config.Key(section, "previous"),
		a.S.Config.Key(section, "next"),
	}
	if horizontal {
		keys = append(keys,
			a.S.Config.Key(section, "button_left"),
			a.S.Config.Key(section, "button_right"))
	}
	labels := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != "" {
			labels = append(labels, hintLabel(key))
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

func configuredHintAction(section, label string) string {
	switch label {
	case "ENTER select", "ENTER unselect":
		if section == "filters" || section == "folders" {
			return "select"
		}
		return "confirm"
	case "ENTER apply":
		switch section {
		case "search":
			return "apply"
		case "detail":
			return "save"
		default:
			return "confirm"
		}
	case "ENTER confirm", "ENTER OK", "ENTER verify":
		return "confirm"
	case "ENTER save":
		return "save"
	case "ENTER open", "→ open":
		return "open"
	case "e edit":
		return "edit"
	case "s switch":
		return "switch"
	case "f favourite":
		return "favorite"
	case "n new":
		return "new"
	case "d delete":
		return "delete"
	case "f find":
		return "search"
	case "c copy":
		return "copy"
	case "r reveal":
		return "reveal"
	case "ctrl+r reveal":
		if section == "reprompt_popup" {
			return "reveal_password"
		}
		return "reveal"
	case "ctrl+ENTER newline":
		return "newline"
	case "ctrl+w wrap", "ctrl+w unwrap":
		return "wrap"
	case "ctrl+z undo":
		return "undo"
	case "ctrl+y redo":
		return "redo"
	case "ctrl+c copy":
		return "copy"
	case "ctrl+v paste":
		return "paste"
	case "s push":
		return "save"
	case "d discard":
		return "discard"
	case "y yes":
		return "accept"
	case "n no":
		return "reject"
	case "esc cancel":
		if section == "detail" {
			return "back"
		}
		return "cancel"
	case "c cancel":
		return "cancel"
	case "esc back":
		return "leave"
	case "esc close":
		if section == "sync_popup" {
			return "abort"
		}
		return "cancel"
	case "← back":
		return "back"
	case "s sync", "s soft sync":
		return "soft_sync"
	case "h hard sync":
		return "hard_sync"
	case "a abort sync":
		return "abort"
	case "? next page":
		return "next_page"
	}
	return ""
}
