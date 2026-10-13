package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// ChangesList includes remote imports for display, never for registration.
func (s *State) ChangesList() []*vault.Change {
	pending := s.Stage.Changes()
	return append(pending, s.ImportedChanges...)
}

// ImportedChangeAt identifies a read-only imported entry in the Changes panel.
func (s *State) ImportedChangeAt(index int) bool {
	return index >= s.Stage.Len() && index < len(s.ChangesList())
}

// ClearImportedChange is intentionally disabled: session imports are immutable.
func (s *State) ClearImportedChange(index int) {
	_ = index
}

// mergeValues performs field-level three-way reconciliation of sealed vault data.
// For overlapping edits, keepLocal chooses which value wins; no secret is opened.
func mergeValues(base, local, remote any, keepLocal bool) any {
	if vault.EqualContent(local, base) {
		return copyValue(remote)
	}
	if vault.EqualContent(remote, base) {
		return copyValue(local)
	}
	b, bok := base.(map[string]any)
	l, lok := local.(map[string]any)
	r, rok := remote.(map[string]any)
	if bok && lok && rok {
		result := make(map[string]any)
		keys := make(map[string]struct{})
		for key := range b {
			keys[key] = struct{}{}
		}
		for key := range l {
			keys[key] = struct{}{}
		}
		for key := range r {
			keys[key] = struct{}{}
		}
		for key := range keys {
			if key == "fields" {
				baseFields, bOK := b[key].([]any)
				localFields, lOK := l[key].([]any)
				remoteFields, rOK := r[key].([]any)
				if bOK && lOK && rOK {
					result[key] = mergeCustomFields(baseFields, localFields, remoteFields, keepLocal)
					continue
				}
			}
			merged := mergeValues(b[key], l[key], r[key], keepLocal)
			if merged != nil {
				result[key] = merged
			}
		}
		return result
	}
	bs, bok := base.([]any)
	ls, lok := local.([]any)
	rs, rok := remote.([]any)
	if bok && lok && rok && len(bs) == len(ls) && len(bs) == len(rs) {
		merged := make([]any, len(rs))
		for i := range rs {
			merged[i] = mergeValues(bs[i], ls[i], rs[i], keepLocal)
		}
		return merged
	}
	if keepLocal {
		return copyValue(local)
	}
	return copyValue(remote)
}

// mergeCustomFields preserves independent additions and field edits when the
// two sides change different fields. Duplicate field names are disambiguated
// by their occurrence number; sensitive values are never decrypted.
func mergeCustomFields(base, local, remote []any, keepLocal bool) []any {
	indexes := func(fields []any) ([]string, map[string]any) {
		keys := make([]string, 0, len(fields))
		values := make(map[string]any, len(fields))
		occurrences := map[string]int{}
		for _, v := range fields {
			entry, _ := v.(map[string]any)
			if entry == nil {
				return nil, nil
			}
			identity := fmt.Sprint(entry["name"], "|", entry["type"])
			n := occurrences[identity]
			occurrences[identity] = n + 1
			key := fmt.Sprintf("%s#%d", identity, n)
			keys = append(keys, key)
			values[key] = v
		}
		return keys, values
	}
	_, bv := indexes(base)
	lk, lv := indexes(local)
	rk, rv := indexes(remote)
	if bv == nil || lv == nil || rv == nil { // Unexpected field structure: fail closed on conflicts.
		if keepLocal {
			return copyValue(local).([]any)
		}
		return copyValue(remote).([]any)
	}
	result := make([]any, 0, len(remote)+len(local))
	for _, k := range rk {
		var localValue any = lv[k]
		if _, present := lv[k]; !present {
			if _, existed := bv[k]; !existed {
				localValue = rv[k]
			}
		}
		merged := mergeValues(bv[k], localValue, rv[k], keepLocal)
		if merged != nil {
			result = append(result, merged)
		}
	}
	for _, k := range lk {
		if _, onRemote := rv[k]; onRemote {
			continue
		}
		if _, original := bv[k]; original {
			if keepLocal && !vault.EqualContent(lv[k], bv[k]) {
				result = append(result, copyValue(lv[k]))
			}
			continue
		}
		// Local additions do not conflict with remote edits to other fields.
		result = append(result, copyValue(lv[k]))
	}
	return result
}

// copyValue copies mutable data but keeps authenticated sealed values opaque.
func copyValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return vault.Clone(v)
	case []any:
		cloned := make([]any, len(v))
		for i := range v {
			cloned[i] = copyValue(v[i])
		}
		return cloned
	default:
		return value
	}
}

// clonedMap keeps nil meaningful when representing deleted remote objects.
func clonedMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	return vault.Clone(src)
}

// ApplyRemoteReview reconciles confirmed server data and outstanding local edits.
// track keeps overlapping local edits; hard sync discards conflicting edits.
// Both modes record immutable, informational ground changes for the session.
func (s *State) ApplyRemoteReview(review *bitwarden.RemoteReview, track bool) {
	if review == nil || s.LocalMode {
		return
	}
	s.UnverifiedPush = false
	staged := s.Stage.Changes()
	selectedFolderID := ""
	if s.FilterFolder > 0 && s.FilterFolder <= len(s.Folders) {
		selectedFolderID = fmt.Sprint(s.Folders[s.FilterFolder-1]["id"])
	}
	if s.InitialGrounds == nil {
		s.InitialGrounds = make(map[string]map[string]any, len(s.Originals))
		for id, raw := range s.Originals {
			s.InitialGrounds[id] = vault.Clone(raw)
		}
	}
	previousItems := s.Originals
	previousFolders := s.FolderOriginals
	currentNames := make(map[string]string)
	for _, item := range s.Items {
		if owner, ok := item.Raw["_owner_name"].(string); ok {
			currentNames[item.ID()] = owner
		}
	}
	s.Originals = make(map[string]map[string]any, len(review.Items))
	items := make(map[string]map[string]any, len(review.Items))
	for id, remote := range review.Items {
		clean := vault.Clone(remote)
		if owner := currentNames[id]; owner != "" {
			clean["_owner_name"] = owner
		}
		s.Originals[id] = vault.Clone(clean)
		items[id] = clean
	}
	s.FolderOriginals = make(map[string]map[string]any, len(review.Folders))
	folders := make(map[string]map[string]any, len(review.Folders))
	for id, remote := range review.Folders {
		s.FolderOriginals[id] = vault.Clone(remote)
		folders[id] = vault.Clone(remote)
	}
	nextStage := vault.NewStage()
	for _, ch := range staged {
		base := previousItems[ch.Key[len(ch.Entity)+1:]]
		remote := s.Originals[ch.Key[len(ch.Entity)+1:]]
		originals := s.Originals
		destination := items
		if ch.Entity == "folder" {
			base = previousFolders[ch.Key[len(ch.Entity)+1:]]
			remote = s.FolderOriginals[ch.Key[len(ch.Entity)+1:]]
			originals = s.FolderOriginals
			destination = folders
		}
		if ch.Kind == vault.New {
			value := clonedMap(ch.After)
			destination[fmt.Sprint(value["id"])] = value
			nextStage.Record(ch.Entity, value, ch.TypeName, originals)
			continue
		}
		if remote == nil {
			if track && ch.After != nil {
				value := clonedMap(ch.After)
				value["id"] = vault.LocalID()
				destination[fmt.Sprint(value["id"])] = value
				nextStage.Record(ch.Entity, value, ch.TypeName, originals)
			}
			continue
		}
		if ch.Kind == vault.Deleted {
			if track || vault.EqualContent(base, remote) {
				delete(destination, fmt.Sprint(remote["id"]))
				nextStage.Delete(ch.Entity, fmt.Sprint(remote["id"]), ch.TypeName, originals)
			}
			continue
		}
		merged, _ := mergeValues(base, ch.After, remote, track).(map[string]any)
		if merged == nil {
			merged = vault.Clone(remote)
		}
		merged["id"] = remote["id"]
		destination[fmt.Sprint(remote["id"])] = merged
		nextStage.Record(ch.Entity, merged, ch.TypeName, originals)
	}
	s.Stage = nextStage
	{
		for _, diff := range review.Differences {
			// Imported changes only carry metadata: they never retain secret payloads.
			key := vault.Key(diff.Entity, diff.ID)
			found := false
			for i, entry := range s.ImportedChanges {
				if entry.Key == "import:"+key {
					s.ImportedChanges[i].Kind = diff.Kind
					s.ImportedChanges[i].Name = diff.Name
					found = true
					break
				}
			}
			if !found {
				s.ImportedChanges = append(s.ImportedChanges, &vault.Change{
					Entity: diff.Entity, Key: "import:" + key, Kind: diff.Kind,
					Name: diff.Name, TypeName: diff.TypeName,
				})
			}
		}
	}
	s.Items = make([]*vault.Item, 0, len(items))
	for _, raw := range items {
		s.Items = append(s.Items, &vault.Item{Raw: raw, Cipher: s.CLI.Cipher})
	}
	vault.SortedItems(s.Items)
	s.Folders = make([]map[string]any, 0, len(folders))
	for _, raw := range folders {
		s.Folders = append(s.Folders, raw)
	}
	sort.Slice(s.Folders, func(i, j int) bool {
		return strings.ToLower(fmt.Sprint(s.Folders[i]["name"])) <
			strings.ToLower(fmt.Sprint(s.Folders[j]["name"]))
	})
	if selectedFolderID != "" {
		s.FilterFolder = 0
		for i, folder := range s.Folders {
			if fmt.Sprint(folder["id"]) == selectedFolderID {
				s.FilterFolder = i + 1
				break
			}
		}
		s.Filters[2] = s.FilterFolder
	} else if s.FilterFolder > len(s.Folders) {
		s.FilterFolder = -1
		s.Filters[2] = 0 // No Folder is highlighted even with no applied filter.
	}
	// The highlighted folder may also have been removed while no folder
	// filter was applied. Keep the cursor on a visible row in either case.
	s.Filters[2] = max(0, min(len(s.Folders), s.Filters[2]))
	if s.SelectedID != "" && s.SelectedItem() == nil {
		s.CloseItem()
	}
	if s.ChangeIndex >= len(s.ChangesList()) {
		s.ChangeIndex = max(0, len(s.ChangesList())-1)
	}
	s.Revealed = make(map[string]bool)
	s.recompute()
	s.loadFields()
}

// GroundFieldChanged reports a remote baseline difference since session start.
// Comparison is performed on authenticated sealed values, not plaintext.
func (s *State) GroundFieldChanged(id string, path []any) bool {
	if s.InitialGrounds == nil {
		return false
	}
	initial, existed := s.InitialGrounds[id]
	current := s.Originals[id]
	if !existed || current == nil {
		return false
	}
	originalValue, originalOK := groundPath(initial, path)
	currentValue, currentOK := groundPath(current, path)
	return originalOK != currentOK || !vault.EqualContent(originalValue, currentValue)
}

// groundPath reads a raw field path without opening protected field contents.
func groundPath(raw map[string]any, path []any) (any, bool) {
	var value any = raw
	for _, part := range path {
		switch node := value.(type) {
		case map[string]any:
			key, ok := part.(string)
			if !ok {
				return nil, false
			}
			value, ok = node[key]
			if !ok {
				return nil, false
			}
		case []any:
			index, ok := part.(int)
			if !ok || index < 0 || index >= len(node) {
				return nil, false
			}
			value = node[index]
		default:
			return nil, false
		}
	}
	return value, true
}

// GroundItemChanged detects any baseline field changed by a remote sync.
func (s *State) GroundItemChanged(id string) bool {
	if s.InitialGrounds == nil {
		return false
	}
	initial, existed := s.InitialGrounds[id]
	current, present := s.Originals[id]
	if !existed || !present {
		return existed != present
	}
	return !vault.EqualContent(groundContent(initial), groundContent(current))
}

// groundContent excludes server-generated bookkeeping from the per-item
// indicator, while retaining fields that can be modified in Bitwarden.
func groundContent(raw map[string]any) map[string]any {
	value := vault.Clone(raw)
	for _, key := range []string{
		"id", "object", "revisionDate", "creationDate", "deletedDate",
		"passwordRevisionDate", "_owner_name", "edit", "viewPassword",
	} {
		delete(value, key)
	}
	return value
}

// acknowledgePushedGround clears remote indicators only for fields pushed
// successfully. Other remotely changed fields retain their session markers.
func (s *State) acknowledgePushedGround(oldID, newID string,
	before, after, remote map[string]any) {
	if s.InitialGrounds == nil || before == nil || after == nil || remote == nil {
		return
	}
	initial := s.InitialGrounds[oldID]
	if initial == nil {
		return
	}
	if oldID != newID {
		delete(s.InitialGrounds, oldID)
		s.InitialGrounds[newID] = initial
	}
	updatePushedPaths(initial, before, after, remote)
	retained := s.ImportedChanges[:0]
	for _, entry := range s.ImportedChanges {
		if entry.Key == "import:item:"+newID && !s.GroundItemChanged(newID) {
			continue
		}
		retained = append(retained, entry)
	}
	s.ImportedChanges = retained
}

// updatePushedPaths updates the initial ground for edited paths alone.
// Arrays and encrypted values remain atomic, avoiding plaintext comparisons.
func updatePushedPaths(initial, before, after, remote map[string]any) {
	keys := make(map[string]bool, len(before)+len(after))
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	for key := range keys {
		prior, hadPrior := before[key]
		edited, hasEdited := after[key]
		if hadPrior == hasEdited && vault.EqualContent(prior, edited) {
			continue
		}
		priorArray, paOK := prior.([]any)
		editedArray, eaOK := edited.([]any)
		remoteArray, raOK := remote[key].([]any)
		initialArray, iaOK := initial[key].([]any)
		if hadPrior && hasEdited && paOK && eaOK && raOK && iaOK &&
			len(priorArray) == len(editedArray) &&
			len(remoteArray) == len(editedArray) &&
			len(initialArray) == len(editedArray) {
			for index := range editedArray {
				if vault.EqualContent(priorArray[index], editedArray[index]) {
					continue
				}
				priorElement, pb := priorArray[index].(map[string]any)
				editedElement, eb := editedArray[index].(map[string]any)
				remoteElement, rb := remoteArray[index].(map[string]any)
				initialElement, ib := initialArray[index].(map[string]any)
				if pb && eb && rb && ib {
					updatePushedPaths(initialElement, priorElement, editedElement,
						remoteElement)
				} else {
					initialArray[index] = copyValue(remoteArray[index])
				}
			}
			continue
		}
		priorMap, pOK := prior.(map[string]any)
		editedMap, eOK := edited.(map[string]any)
		remoteMap, rOK := remote[key].(map[string]any)
		initialMap, iOK := initial[key].(map[string]any)
		if hadPrior && hasEdited && pOK && eOK && rOK && iOK {
			updatePushedPaths(initialMap, priorMap, editedMap, remoteMap)
			continue
		}
		if value, present := remote[key]; present {
			initial[key] = copyValue(value)
		} else {
			delete(initial, key)
		}
	}
}

// pushedEntry identifies an entity whose write was confirmed by bw create/edit.
// The final synced list determines the canonical post-push ground value.
type pushedEntry struct {
	entity string
	id     string
	change *vault.Change
}

// AcceptPushedSnapshot replaces provisional bw edit/create responses with the
// authoritative synced representation for pushed entries alone. Unrelated
// remote activity must still appear as a conflict during the next check.
func (s *State) AcceptPushedSnapshot(
	review *bitwarden.RemoteReview, pushed []pushedEntry,
) bool {
	if review == nil {
		return false
	}
	for _, entry := range pushed {
		if entry.change.Kind == vault.Deleted {
			if entry.entity == "item" && review.Items[entry.id] != nil ||
				entry.entity == "folder" && review.Folders[entry.id] != nil {
				return false
			}
			continue
		}
		if entry.entity == "item" && review.Items[entry.id] == nil ||
			entry.entity == "folder" && review.Folders[entry.id] == nil {
			return false
		}
	}
	for _, entry := range pushed {
		if entry.change.Kind == vault.Deleted {
			continue
		}
		if entry.entity == "item" {
			clean := vault.Clone(review.Items[entry.id])
			if before := s.Originals[entry.id]; before != nil {
				if owner, ok := before["_owner_name"].(string); ok {
					clean["_owner_name"] = owner
				}
			}
			s.Originals[entry.id] = vault.Clone(clean)
			for _, it := range s.Items {
				if it.ID() == entry.id {
					it.Raw = vault.Clone(clean)
					break
				}
			}
			s.acknowledgePushedGround(entry.id, entry.id,
				entry.change.Before, entry.change.After, clean)
		} else {
			clean := vault.Clone(review.Folders[entry.id])
			s.FolderOriginals[entry.id] = vault.Clone(clean)
			for index, folder := range s.Folders {
				if fmt.Sprint(folder["id"]) == entry.id {
					s.Folders[index] = clean
					break
				}
			}
		}
	}
	s.UnverifiedPush = false
	s.recompute()
	s.loadFields()
	return true
}
