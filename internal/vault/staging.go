package vault

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type Kind string

const (
	New      Kind = "N"
	Modified Kind = "M"
	Deleted  Kind = "D"
)

type Change struct {
	Entity, Key, Name, TypeName string
	Kind                        Kind
	Before, After               map[string]any
}
type Stage struct {
	mu      sync.RWMutex
	changes map[string]*Change
	order   []string
}

var tempCounter atomic.Uint64

func LocalID() string              { return fmt.Sprintf("local-%d", tempCounter.Add(1)) }
func NewStage() *Stage             { return &Stage{changes: make(map[string]*Change)} }
func Key(entity, id string) string { return entity + ":" + id }
func (s *Stage) Record(entity string, current map[string]any, typeName string, original map[string]map[string]any) {
	id := stringValue(current["id"])
	key := Key(entity, id)
	s.mu.Lock()
	defer s.mu.Unlock()
	old := original[id]
	if prev := s.changes[key]; prev != nil {
		old = prev.Before
	}
	if equalValue(old, current) {
		s.removeLocked(key)
		return
	}
	if s.changes[key] == nil {
		s.order = append(s.order, key)
	}
	kind := Modified
	if old == nil {
		kind = New
	}
	s.changes[key] = &Change{Entity: entity, Key: key, Kind: kind, Name: stringValue(current["name"]), TypeName: typeName, Before: copyOpt(old), After: Clone(current)}
}
func (s *Stage) Delete(entity, id, typeName string, original map[string]map[string]any) {
	key := Key(entity, id)
	s.mu.Lock()
	defer s.mu.Unlock()
	old := original[id]
	if prev := s.changes[key]; prev != nil {
		old = prev.Before
	}
	if old == nil {
		s.removeLocked(key)
		return
	}
	if s.changes[key] == nil {
		s.order = append(s.order, key)
	}
	s.changes[key] = &Change{Entity: entity, Key: key, Kind: Deleted, Name: stringValue(old["name"]), TypeName: typeName, Before: Clone(old)}
}
func (s *Stage) removeLocked(key string) {
	delete(s.changes, key)
	for j, k := range s.order {
		if k == key {
			s.order = append(s.order[:j], s.order[j+1:]...)
			break
		}
	}
}
func (s *Stage) Remove(key string) { s.mu.Lock(); defer s.mu.Unlock(); s.removeLocked(key) }
func (s *Stage) Get(key string) *Change {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyChange(s.changes[key])
}
func (s *Stage) Changes() []*Change {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Change, 0, len(s.changes))
	for _, k := range s.order {
		if c := s.changes[k]; c != nil {
			out = append(out, copyChange(c))
		}
	}
	return out
}
func (s *Stage) Len() int { s.mu.RLock(); defer s.mu.RUnlock(); return len(s.changes) }
func copyOpt(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return Clone(m)
}
func copyChange(c *Change) *Change {
	if c == nil {
		return nil
	}
	r := *c
	r.Before = copyOpt(c.Before)
	r.After = copyOpt(c.After)
	return &r
}
func (s *Stage) FieldKind(id string, path []any, cipher *Cipher) Kind {
	change := s.Get(Key("item", id))
	if change == nil {
		return ""
	}
	if change.Kind != Modified {
		return change.Kind
	}
	a, b := getPath(change.Before, path), getPath(change.After, path)
	// New custom fields are N even when their initial value is blank.
	if len(path) == 3 && path[0] == "fields" {
		currentFields, _ := change.After["fields"].([]any)
		oldFields, _ := change.Before["fields"].([]any)
		idx, ok := path[1].(int)
		if ok && idx >= 0 && idx < len(currentFields) {
			field, _ := currentFields[idx].(map[string]any)
			matched := false
			for _, candidate := range oldFields {
				prev, _ := candidate.(map[string]any)
				if prev != nil && prev["name"] == field["name"] && prev["type"] == field["type"] {
					a = prev["value"]
					matched = true
					break
				}
			}
			if !matched {
				return New
			}
		}
	}
	if a == nil {
		a = ""
	}
	if b == nil {
		b = ""
	}
	if !equalValue(a, b) {
		return Modified
	}
	return ""
}
func (s *Stage) RemapFolder(localID, remoteID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.changes {
		if ch.Entity == "item" && ch.After != nil && ch.After["folderId"] == localID {
			ch.After["folderId"] = remoteID
		}
	}
}
func (s *Stage) OrderedForSave() []*Change {
	changes := s.Changes()
	sort.SliceStable(changes, func(i, j int) bool {
		priority := func(c *Change) int {
			if c.Entity == "folder" && c.Kind == New {
				return 0
			}
			if c.Entity == "item" && c.Kind == Deleted {
				return 3
			}
			if c.Entity == "folder" && c.Kind == Deleted {
				return 4
			}
			return 1
		}
		return priority(changes[i]) < priority(changes[j])
	})
	return changes
}

// FuzzyMatch matches only the name, returning character offsets and a score.
func FuzzyMatch(name, query string) (bool, []int, int) {
	if query == "" {
		return true, nil, 0
	}
	s := []rune(strings.ToLower(name))
	q := []rune(strings.ToLower(query))
	ix := []int{}
	at := 0
	score := 0
	for _, target := range q {
		found := false
		for at < len(s) {
			if s[at] == target {
				ix = append(ix, at)
				if len(ix) > 1 && at == ix[len(ix)-2]+1 {
					score += 5
				}
				score += 100 - at
				at++
				found = true
				break
			}
			at++
		}
		if !found {
			return false, nil, 0
		}
	}
	return true, ix, score
}

// EqualContent compares nested remote data without decrypting sealed secrets.
// Re-encrypted sensitive fields are compared by their authenticated tags.
func EqualContent(a, b any) bool { return equalValue(a, b) }

// equalValue treats re-encrypted identical secrets as unchanged.
func equalValue(a, b any) bool {
	switch x := a.(type) {
	case *Sealed:
		if y, ok := b.(*Sealed); ok {
			return SameSecret(x, y)
		}
		return false
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			if !equalValue(v, y[k]) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i, v := range x {
			if !equalValue(v, y[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}
