package ui

import (
	"fmt"
	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
	"github.com/miguelmnts/lazybw-go/internal/localvault"
	"github.com/miguelmnts/lazybw-go/internal/vault"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Screen int

const (
	AuthClient Screen = iota
	AuthMaster
	Workspace
	AuthChecking // Neutral startup screen until status/API checks complete.
	AuthOffline  // No Bitwarden endpoint responded and no local backup exists.
)

type Mode int

const (
	Browse Mode = iota
	Find
	Edit
	DialogCreate
	DialogName
	DialogFieldType
	DialogField
	DialogFolder
	DialogWebsite
	DialogServer
	DialogLinkedChoice
	DialogFoldersChoice
	DialogCardBrand
	DialogIdentityTitle
	DialogReprompt
)

type Pane int

const (
	PaneVault Pane = iota + 1
	PaneTypes
	PaneFolders
	PaneChanges
	PaneItems
	PaneContent
	PaneDetail
)

type Popup struct {
	Blocking               bool
	Title, Question        string
	Lines                  []string
	Save                   bool
	Destructive            bool
	Quit                   bool
	RemoteReview           *bitwarden.RemoteReview
	Acknowledge            bool
	RemoteChoice           int // 0 soft sync, 1 hard sync, 2 abort sync
	RemoteFromRegistration bool
	Yes                    bool
	Page                   int
	Action                 func(bool)
}
type State struct {
	Config                             config.Config
	Server                             string
	ServerChoice                       int
	ServerReady                        [2]bool
	ServerMoved                        bool
	LocalMode                          bool
	LocalOrigin                        string
	LocalWarning                       string
	ServerConnected                    bool
	LocalSession                       *localvault.Session
	BackupBusy                         bool
	CLI                                *bitwarden.Client
	Screen                             Screen
	Mode                               Mode
	Focus                              Pane
	Items                              []*vault.Item
	Folders                            []map[string]any
	Stage                              *vault.Stage
	ImportedChanges                    []*vault.Change
	InitialGrounds                     map[string]map[string]any
	UnverifiedPush                     bool
	Originals                          map[string]map[string]any
	FolderOriginals                    map[string]map[string]any
	FilterVault                        int
	FilterType                         int
	FilterFolder                       int
	Filters                            [4]int
	ItemIndex, FieldIndex, ChangeIndex int
	SelectedID                         string
	Search                             string
	SearchCursor                       int
	Fields                             []vault.Field
	Revealed                           map[string]bool
	Input                              string
	Caret                              int
	InputSecret                        bool
	AuthID, AuthSecret, Master         string
	masterSealed                       *vault.Sealed
	masterProtected                    bool
	AuthFocus                          int
	AuthVisible                        bool
	NewType                            int
	NewName                            string
	CustomType                         int
	WebsiteIndex                       int
	EditNewWebsite                     bool
	LinkedIndex                        int
	LinkedEdit                         bool
	ChoiceIndex                        int
	FolderChoiceID                     string
	RepromptTargetID                   string
	RepromptError                      string
	RepromptApprovedID                 string
	KeyFeedback, KeyFeedbackAction     string
	KeyFeedbackExpires                 time.Time
	EditorOriginal                     string
	EditPath                           []any
	FormFocus                          int
	NewFieldName                       string
	Popup                              *Popup
	Busy                               bool
	BusyMessage                        string
	Version, Notice                    string
	NoticeUntil                        time.Time
	NoticeColor                        int
	HintPage                           int
	Blink                              bool
	Spinner                            int
	QuitWhenSaved                      bool
}

// SetWorkspaceNotice displays a one-second footer message without affecting
// persistent authentication diagnostics.
func (s *State) SetWorkspaceNotice(message string, color int) {
	s.Notice = message
	s.NoticeColor = color
	s.NoticeUntil = time.Now().Add(time.Second)
}

// ClearWorkspaceNotice clears a transient status message before the next task.
func (s *State) ClearWorkspaceNotice() {
	s.Notice = ""
	s.NoticeUntil = time.Time{}
}

func NewState(cfg config.Config, cli *bitwarden.Client) *State {
	return &State{Config: cfg, ServerConnected: true, Server: config.NormalizeServer(cfg.Authentication.DefaultServer), CLI: cli, Screen: AuthChecking, Busy: true, BusyMessage: "Checking Bitwarden session", Stage: vault.NewStage(), Focus: PaneItems, Originals: map[string]map[string]any{}, FolderOriginals: map[string]map[string]any{}, Revealed: map[string]bool{}, InputSecret: true, FilterVault: 1, FilterFolder: -1, Filters: [4]int{1, 0, -1, 0}}
}

// ProtectMaster encrypts the editing buffer between keystrokes in the live UI.
// The transient strings required by the renderer and bw subprocess cannot be
// guaranteed to disappear from Go's heap immediately.
func (s *State) ProtectMaster() error {
	if s.masterProtected {
		return nil
	}
	if s.Master != "" {
		value, err := s.CLI.Cipher.Seal(s.Master)
		if err != nil {
			return err
		}
		s.masterSealed = value.(*vault.Sealed)
		s.Master = ""
	}
	s.masterProtected = true
	return nil
}

func (s *State) MasterText() string {
	if !s.masterProtected || s.masterSealed == nil {
		return s.Master
	}
	value, err := s.CLI.Cipher.Open(s.masterSealed)
	if err != nil {
		s.Notice = "Cannot decrypt the master password"
		return ""
	}
	return value
}

func (s *State) SetMasterText(value string) error {
	if !s.masterProtected {
		s.Master = value
		return nil
	}
	if value == "" {
		s.ClearMaster()
		return nil
	}
	sealed, err := s.CLI.Cipher.Seal(value)
	if err != nil {
		return err
	}
	// Replace the previous ciphertext only after the new value is sealed.
	if s.masterSealed != nil {
		for i := range s.masterSealed.Data {
			s.masterSealed.Data[i] = 0
		}
		for i := range s.masterSealed.Nonce {
			s.masterSealed.Nonce[i] = 0
		}
	}
	s.masterSealed = sealed.(*vault.Sealed)
	s.Master = ""
	return nil
}

func (s *State) ClearMaster() {
	if s.masterSealed != nil {
		for i := range s.masterSealed.Data {
			s.masterSealed.Data[i] = 0
		}
		for i := range s.masterSealed.Nonce {
			s.masterSealed.Nonce[i] = 0
		}
		s.masterSealed = nil
	}
	s.Master = ""
}

// BeginVaultLoading leaves the password form as soon as authentication succeeds.
// Snapshot loading then uses the neutral startup layout with a centered spinner.
func (s *State) BeginVaultLoading() {
	s.Screen = AuthChecking
	s.Busy = true
	s.BusyMessage = "Loading vault"
	s.ClearMaster()
	s.Caret = 0
	s.Notice = ""
}

func (s *State) Loaded(items []*vault.Item, folders []map[string]any) {
	s.Items = items
	s.Folders = folders
	s.ImportedChanges = nil
	s.InitialGrounds = nil
	s.UnverifiedPush = false
	s.Originals = map[string]map[string]any{}
	for _, it := range items {
		s.Originals[it.ID()] = vault.Clone(it.Raw)
	}
	s.FolderOriginals = map[string]map[string]any{}
	for _, f := range folders {
		s.FolderOriginals[fmt.Sprint(f["id"])] = vault.Clone(f)
	}
	s.Screen = Workspace
	s.Busy = false
	s.BusyMessage = ""
	s.Focus = PaneItems
	s.recompute()
}
func (s *State) RebuildFromStaging() { s.recompute(); s.loadFields() }
func (s *State) Filtered() []*vault.Item {
	out := make([]*vault.Item, 0, len(s.Items))
	for _, it := range s.Items {
		if s.FilterVault == 0 && it.Raw["organizationId"] != nil && it.Raw["organizationId"] != "" {
			continue
		}
		if s.FilterType == 1 && !it.Favorite() {
			continue
		}
		if s.FilterType > 1 {
			expected := map[int]int{2: vault.Login, 3: vault.Card, 4: vault.Identity, 5: vault.SecureNote, 6: vault.SSHKey}[s.FilterType]
			if it.Type() != expected {
				continue
			}
		}
		if s.FilterFolder > 0 && it.FolderID() != fmt.Sprint(s.Folders[s.FilterFolder-1]["id"]) {
			continue
		}
		if s.FilterFolder == 0 && it.FolderID() != "" {
			continue
		}
		ok, _, _ := vault.FuzzyMatch(it.Name(), s.Search)
		if ok {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		_, _, si := vault.FuzzyMatch(out[i].Name(), s.Search)
		_, _, sj := vault.FuzzyMatch(out[j].Name(), s.Search)
		if s.Search != "" && si != sj {
			return si > sj
		}
		return strings.ToLower(out[i].Name()) < strings.ToLower(out[j].Name())
	})
	return out
}
func (s *State) recompute() {
	items := s.Filtered()
	if len(items) == 0 {
		s.ItemIndex = 0
		return
	}
	if s.SelectedID != "" {
		for j, it := range items {
			if it.ID() == s.SelectedID {
				s.ItemIndex = j
				return
			}
		}
	}
	if s.ItemIndex >= len(items) {
		s.ItemIndex = len(items) - 1
	}
	if s.ItemIndex < 0 {
		s.ItemIndex = 0
	}
}
func (s *State) CurrentItem() *vault.Item {
	items := s.Filtered()
	if len(items) == 0 {
		return nil
	}
	if s.ItemIndex < 0 || s.ItemIndex >= len(items) {
		s.ItemIndex = 0
	}
	return items[s.ItemIndex]
}
func (s *State) OpenItem() {
	it := s.CurrentItem()
	if it == nil {
		return
	}
	if (it.Raw["reprompt"] == 1 || it.Raw["reprompt"] == float64(1)) && s.RepromptApprovedID != it.ID() {
		return
	}
	s.SelectedID = it.ID()
	s.FieldIndex = 0
	s.loadFields()
	s.Focus = PaneContent
}
func (s *State) CloseItem() {
	s.Focus = PaneItems
	s.SelectedID = ""
	s.RepromptApprovedID = ""
	s.Fields = nil
	s.Mode = Browse
}

// ToggleFavourite stages Bitwarden's native favorite flag, without a detail field.
func (s *State) ToggleFavourite() bool {
	if s.LocalMode || s.Screen != Workspace || s.Mode != Browse ||
		s.Focus != PaneContent {
		return false
	}
	item := s.SelectedItem()
	if item == nil || !item.CanEdit() {
		return false
	}
	item.Raw["favorite"] = !item.Favorite()
	s.Stage.Record("item", item.Raw, vault.TypeNames[item.Type()], s.Originals)
	s.recompute()
	return true
}

// FavouriteKind describes changes to this flag, not to other item fields.
func (s *State) FavouriteKind(item *vault.Item) vault.Kind {
	if item == nil {
		return ""
	}
	return s.Stage.FieldKind(item.ID(), []any{"favorite"}, s.CLI.Cipher)
}

func (s *State) SelectedItem() *vault.Item {
	for _, it := range s.Items {
		if it.ID() == s.SelectedID {
			return it
		}
	}
	return nil
}
func (s *State) loadFields() {
	it := s.SelectedItem()
	if it == nil {
		s.Fields = nil
		return
	}
	s.Fields = it.Fields()
	var names []string
	for _, folder := range s.Folders {
		if fmt.Sprint(folder["id"]) == it.FolderID() {
			names = append(names, fmt.Sprint(folder["name"]))
		}
	}
	s.Fields = append(s.Fields, vault.Field{
		Name: "Folder", Value: strings.Join(names, "\n"), Editable: it.CanEdit(),
		Folders: true, Path: []any{"folderId"}, Values: names,
	})
	if s.FieldIndex >= len(s.Fields) {
		s.FieldIndex = max(0, len(s.Fields)-1)
	}
}
func (s *State) CurrentField() *vault.Field {
	if s.FieldIndex < 0 || s.FieldIndex >= len(s.Fields) {
		return nil
	}
	return &s.Fields[s.FieldIndex]
}
func (s *State) RevealKey() string {
	f := s.CurrentField()
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%s:%v", s.SelectedID, f.Path)
}
func (s *State) Reveal() {
	f := s.CurrentField()
	if f != nil && f.Hidden && !f.Restricted {
		s.Revealed[s.RevealKey()] = !s.Revealed[s.RevealKey()]
	}
}
func (s *State) BeginEdit() error {
	if s.LocalMode {
		return fmt.Errorf("local vault is read-only")
	}
	f := s.CurrentField()
	if s.SelectedItem() == nil || !s.SelectedItem().CanEdit() || f == nil || !f.Editable || isCheckboxField(f) {
		return nil // Checkbox is toggled in place; it never opens Details.
	}
	if f.Folders {
		s.Mode = DialogFoldersChoice
		s.FolderChoiceID = s.SelectedItem().FolderID()
		s.ChoiceIndex = 0
		for index, folder := range s.Folders {
			if fmt.Sprint(folder["id"]) == s.FolderChoiceID {
				s.ChoiceIndex = index
			}
		}
		return nil
	}
	if len(f.Path) == 2 && f.Path[0] == "card" && f.Path[1] == "brand" {
		s.Mode = DialogCardBrand
		s.ChoiceIndex = 0
		for index, name := range cardBrands {
			value, _ := f.Reveal(s.CLI.Cipher)
			if value == name {
				s.ChoiceIndex = index
			}
		}
		return nil
	}
	if len(f.Path) == 2 && f.Path[0] == "identity" && f.Path[1] == "title" {
		s.Mode = DialogIdentityTitle
		s.ChoiceIndex = 0
		for index, name := range identityTitles {
			value, _ := f.Reveal(s.CLI.Cipher)
			if value == name {
				s.ChoiceIndex = index
			}
		}
		return nil
	}
	if f.Linked {
		s.Mode = DialogLinkedChoice
		s.LinkedEdit = true
		s.LinkedIndex = 0
		if f.LinkedID == 101 {
			s.LinkedIndex = 1
		}
		return nil
	}
	if f.Websites {
		s.WebsiteIndex = 0
		s.Mode = DialogWebsite
		return nil
	}
	return s.BeginTextEdit()
}

// BeginTextEdit opens the ordinary text editor for the selected field.
func (s *State) BeginTextEdit() error {
	f := s.CurrentField()
	if f == nil || !f.Editable || f.Restricted {
		return fmt.Errorf("field cannot be edited")
	}
	v, err := f.Reveal(s.CLI.Cipher)
	if err != nil {
		return err
	}
	s.Input = v
	s.Caret = utf8.RuneCountInString(v)
	s.EditorOriginal = v
	s.EditPath = append([]any(nil), f.Path...)
	s.Mode = Edit
	s.Focus = PaneDetail
	s.InputSecret = f.Hidden
	return nil
}

// BeginEditWebsite edits one URI in place, preserving its match type and metadata.
func (s *State) BeginEditWebsite(index int) error {
	if s.LocalMode {
		return fmt.Errorf("local vault is read-only")
	}
	item := s.SelectedItem()
	if item == nil || !item.CanEdit() || item.Type() != vault.Login {
		return nil
	}
	uris, _ := item.Raw["login"].(map[string]any)["uris"].([]any)
	if index < 0 || index >= len(uris) {
		return nil
	}
	path := []any{"login", "uris", index, "uri"}
	entry, ok := uris[index].(map[string]any)
	if !ok {
		return fmt.Errorf("website entry %d is invalid", index)
	}
	value, err := s.CLI.Cipher.Open(entry["uri"])
	if err != nil {
		return err
	}
	s.Input, s.EditorOriginal = value, value
	s.Caret = utf8.RuneCountInString(value)
	s.EditPath, s.EditNewWebsite = path, false
	s.Mode, s.Focus, s.InputSecret = Edit, PaneDetail, false
	return nil
}

// BeginNewWebsite opens a blank editor without adding an empty URI to the vault.
func (s *State) BeginNewWebsite() {
	s.Input, s.EditorOriginal, s.Caret = "", "", 0
	s.EditPath, s.EditNewWebsite = nil, true
	s.InputSecret, s.Mode, s.Focus = false, Edit, PaneDetail
}

func (s *State) CommitEdit() error {
	if s.LocalMode {
		return fmt.Errorf("local vault is read-only")
	}
	it := s.SelectedItem()
	if it == nil || !it.CanEdit() {
		return fmt.Errorf("item cannot be edited")
	}
	if s.EditNewWebsite {
		if strings.TrimSpace(s.Input) == "" {
			s.ExitEdit()
			return nil
		}
		login, _ := it.Raw["login"].(map[string]any)
		uris, _ := login["uris"].([]any)
		login["uris"] = append(uris, map[string]any{
			"uri": s.Input, "match": nil,
		})
		s.FieldIndex = 3 // Grouped Websites sits after TOTP.
	} else if err := it.Set(s.EditPath, s.Input); err != nil {
		return err
	}
	s.Stage.Record("item", it.Raw, vault.TypeNames[it.Type()], s.Originals)
	s.ExitEdit()
	s.recompute()
	return nil
}

// isCheckboxField identifies Bitwarden custom boolean fields (type 2).
func isCheckboxField(f *vault.Field) bool {
	return f != nil && (f.Reprompt || f.Custom && f.FieldType == 2)
}

// SwitchCheckbox toggles the currently selected custom boolean field locally.
// It preserves the field selection and stages the change without contacting bw.
func (s *State) SwitchCheckbox() error {
	if s.LocalMode {
		return fmt.Errorf("local vault is read-only")
	}
	f := s.CurrentField()
	if s.Focus != PaneContent || s.Mode != Browse || !isCheckboxField(f) {
		return nil
	}
	item := s.SelectedItem()
	if item == nil || !item.CanEdit() || !f.Editable {
		return fmt.Errorf("item cannot be edited")
	}
	if f.Reprompt {
		current := item.Raw["reprompt"]
		enabled := current == 1 || current == float64(1)
		original := s.Originals[item.ID()]
		originalValue, exists := original["reprompt"]
		originalEnabled := originalValue == 1 || originalValue == float64(1)
		if !enabled == originalEnabled {
			// Restore the original representation, including an omitted key.
			// This removes the staged M when toggled back to its baseline.
			if exists {
				item.Raw["reprompt"] = originalValue
			} else {
				delete(item.Raw, "reprompt")
			}
		} else if enabled {
			item.Raw["reprompt"] = 0
		} else {
			item.Raw["reprompt"] = 1
		}
		if item.Raw["reprompt"] == 1 && s.SelectedID == item.ID() &&
			s.Focus == PaneContent {
			// Enabling re-prompt must not interrupt an item already open.
			s.RepromptApprovedID = item.ID()
		}
		s.Stage.Record("item", item.Raw, vault.TypeNames[item.Type()], s.Originals)
		s.loadFields()
		return nil
	}
	current, err := f.Reveal(s.CLI.Cipher)
	if err != nil {
		return err
	}
	value := "true"
	if strings.EqualFold(strings.TrimSpace(current), "true") || strings.TrimSpace(current) == "1" {
		value = "false"
	}
	if err := item.Set(f.Path, value); err != nil {
		return err
	}
	s.Stage.Record("item", item.Raw, vault.TypeNames[item.Type()], s.Originals)
	s.loadFields()
	return nil
}

func (s *State) ExitEdit() {
	s.Mode = Browse
	s.Focus = PaneContent
	s.Input = ""
	s.EditorOriginal = ""
	s.InputSecret = false
	s.EditPath = nil
	s.EditNewWebsite = false
	s.loadFields()
}
func (s *State) CreateItem(typ int, name string) {
	if s.LocalMode {
		return
	}
	it := vault.NewItem(typ, name, vault.LocalID(), s.CLI.Cipher)
	s.Items = append(s.Items, it)
	vault.SortedItems(s.Items)
	s.Stage.Record("item", it.Raw, vault.TypeNames[typ], s.Originals)
	s.recompute()
}
func (s *State) CreateFolder(name string) {
	if s.LocalMode {
		return
	}
	id := vault.LocalID()
	folder := map[string]any{"id": id, "name": name, "object": "folder"}
	s.Folders = append(s.Folders, folder)
	sort.Slice(s.Folders, func(i, j int) bool {
		return strings.ToLower(fmt.Sprint(s.Folders[i]["name"])) < strings.ToLower(fmt.Sprint(s.Folders[j]["name"]))
	})
	s.Stage.Record("folder", folder, "Folder", s.FolderOriginals)
}
func (s *State) DeleteItem() {
	if s.LocalMode {
		return
	}
	it := s.CurrentItem()
	if it == nil || !it.CanEdit() {
		return
	}
	s.Stage.Delete("item", it.ID(), vault.TypeNames[it.Type()], s.Originals)
	s.removeItem(it.ID())
	s.CloseItem()
	s.recompute()
}
func (s *State) DeleteFolder() {
	if s.LocalMode {
		return
	}
	if s.Filters[2] < 1 || s.Filters[2] > len(s.Folders) {
		return
	}
	f := s.Folders[s.Filters[2]-1]
	id := fmt.Sprint(f["id"])
	// Detaching items is a separate staged edit. Restoring the folder later
	// must not silently discard the items' folder edits.
	for _, item := range s.Items {
		if item.FolderID() != id {
			continue
		}
		item.Raw["folderId"] = nil
		s.Stage.Record("item", item.Raw, vault.TypeNames[item.Type()], s.Originals)
	}
	s.Stage.Delete("folder", id, "Folder", s.FolderOriginals)
	s.Folders = append(s.Folders[:s.Filters[2]-1], s.Folders[s.Filters[2]:]...)
	s.Filters[2] = -1
	s.FilterFolder = -1
	s.recompute()
	s.loadFields()
}
func (s *State) removeItem(id string) {
	for i, v := range s.Items {
		if v.ID() == id {
			s.Items = append(s.Items[:i], s.Items[i+1:]...)
			break
		}
	}
}
func (s *State) Restore(change *vault.Change) {
	if change == nil {
		return
	}
	s.Stage.Remove(change.Key)
	var raw map[string]any
	if change.Before != nil {
		raw = vault.Clone(change.Before)
	}
	if change.Entity == "item" {
		id := ""
		if change.After != nil {
			id = fmt.Sprint(change.After["id"])
		} else if raw != nil {
			id = fmt.Sprint(raw["id"])
		}
		s.removeItem(id)
		if raw != nil {
			s.Items = append(s.Items, &vault.Item{Raw: raw, Cipher: s.CLI.Cipher})
		}
		vault.SortedItems(s.Items)
	} else {
		newFolders := s.Folders[:0]
		for _, f := range s.Folders {
			if change.After != nil && f["id"] == change.After["id"] {
				continue
			}
			if change.Before != nil && f["id"] == change.Before["id"] {
				continue
			}
			newFolders = append(newFolders, f)
		}
		s.Folders = newFolders
		if raw != nil {
			s.Folders = append(s.Folders, raw)
		}
	}
	s.recompute()
	s.loadFields()
}
func (s *State) CommitRemote(ch *vault.Change, remote map[string]any) {
	if ch == nil {
		return
	}
	s.Stage.Remove(ch.Key)
	if ch.Entity == "item" {
		if ch.Kind == vault.Deleted {
			if ch.Before != nil {
				delete(s.Originals, fmt.Sprint(ch.Before["id"]))
			}
			return
		}
		if remote == nil {
			return
		}
		oldID := ""
		if ch.After != nil {
			oldID = fmt.Sprint(ch.After["id"])
		}
		s.removeItem(oldID)
		item := &vault.Item{Raw: remote, Cipher: s.CLI.Cipher}
		s.Items = append(s.Items, item)
		vault.SortedItems(s.Items)
		delete(s.Originals, oldID)
		s.Originals[item.ID()] = vault.Clone(remote)
		s.acknowledgePushedGround(oldID, item.ID(), ch.Before, ch.After, remote)
		if s.SelectedID == oldID {
			s.SelectedID = item.ID()
		}
	} else {
		if ch.Kind == vault.Deleted {
			if ch.Before != nil {
				delete(s.FolderOriginals, fmt.Sprint(ch.Before["id"]))
			}
			return
		}
		if remote != nil {
			oldID := fmt.Sprint(ch.After["id"])
			for idx, f := range s.Folders {
				if fmt.Sprint(f["id"]) == oldID {
					s.Folders[idx] = remote
				}
			}
			delete(s.FolderOriginals, oldID)
			s.FolderOriginals[fmt.Sprint(remote["id"])] = vault.Clone(remote)
			// Replace temporary IDs in local items when a folder is created.
			if ch.Kind == vault.New {
				for _, item := range s.Items {
					if item.FolderID() == oldID {
						item.Raw["folderId"] = remote["id"]
					}
				}
			}
		}
	}
	s.recompute()
	s.loadFields()
}
func (s *State) FieldStatus(f vault.Field) vault.Kind {
	it := s.SelectedItem()
	if it == nil {
		return ""
	}
	return s.Stage.FieldKind(it.ID(), f.Path, s.CLI.Cipher)
}
func (s *State) Insert(r rune) {
	text := []rune(s.Input)
	if s.Caret < 0 {
		s.Caret = 0
	}
	if s.Caret > len(text) {
		s.Caret = len(text)
	}
	text = append(text[:s.Caret], append([]rune{r}, text[s.Caret:]...)...)
	s.Input = string(text)
	s.Caret++
}
func (s *State) Backspace() {
	r := []rune(s.Input)
	if s.Caret > 0 && s.Caret <= len(r) {
		r = append(r[:s.Caret-1], r[s.Caret:]...)
		s.Input = string(r)
		s.Caret--
	}
}
func (s *State) DeleteForward() {
	r := []rune(s.Input)
	if s.Caret >= 0 && s.Caret < len(r) {
		r = append(r[:s.Caret], r[s.Caret+1:]...)
		s.Input = string(r)
	}
}
func (s *State) Cursor(delta int) { s.Caret = max(0, min(len([]rune(s.Input)), s.Caret+delta)) }

// WordBoundary returns the next word boundary using Unicode space/punctuation.
// A word includes letters, marks, digits and underscores.
func WordBoundary(text string, caret, direction int) int {
	rs := []rune(text)
	caret = max(0, min(len(rs), caret))
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_' }
	if direction < 0 {
		for caret > 0 && !isWord(rs[caret-1]) {
			caret--
		}
		for caret > 0 && isWord(rs[caret-1]) {
			caret--
		}
	} else if direction > 0 {
		// Move past the current word, then its following separators,
		// landing at the beginning of the next word.
		for caret < len(rs) && isWord(rs[caret]) {
			caret++
		}
		for caret < len(rs) && !isWord(rs[caret]) {
			caret++
		}
	}
	return caret
}

func (s *State) WordCursor(direction int) {
	s.Caret = WordBoundary(s.Input, s.Caret, direction)
}
func (s *State) DeleteWord(direction int) {
	rs := []rune(s.Input)
	start := max(0, min(len(rs), s.Caret))
	end := WordBoundary(s.Input, start, direction)
	if end < start {
		rs = append(rs[:end], rs[start:]...)
		s.Caret = end
	} else if end > start {
		rs = append(rs[:start], rs[end:]...)
	}
	s.Input = string(rs)
}

// Field dropdowns are sourced from the Bitwarden schema dictionary.
var cardBrands = vault.FieldOptions(vault.Card, "Brand")
var identityTitles = vault.FieldOptions(vault.Identity, "Title")
