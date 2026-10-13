package vault

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const (
	Login      = 1
	SecureNote = 2
	Card       = 3
	Identity   = 4
	SSHKey     = 5
)

var TypeNames = schemaTypeNames()
var TypeIcons = schemaTypeIcons()
var (
	IconFolder  = "\U000F0256"
	IconAll     = "\ueb94"
	IconChanges = "\ueadc"
	IconFilter  = "\U000f10e5"
)

type Item struct {
	Raw    map[string]any
	Cipher *Cipher
}
type Field struct {
	Name       string
	Value      any
	Hidden     bool
	Restricted bool
	Custom     bool
	Editable   bool
	Path       []any
	FieldType  int
	Websites   bool
	Linked     bool
	LinkedID   int
	Reprompt   bool
	Values     []string
	Folders    bool
}

func (f Field) Reveal(c *Cipher) (string, error) {
	if f.Restricted {
		return "", fmt.Errorf("permission denied: protected field")
	}
	return c.Open(f.Value)
}
func (i *Item) ID() string       { return stringValue(i.Raw["id"]) }
func (i *Item) Name() string     { return stringValue(i.Raw["name"]) }
func (i *Item) Type() int        { return intValue(i.Raw["type"]) }
func (i *Item) FolderID() string { return stringValue(i.Raw["folderId"]) }
func (i *Item) Owner() string {
	if stringValue(i.Raw["organizationId"]) != "" {
		if v := stringValue(i.Raw["_owner_name"]); v != "" {
			return v
		}
		return "Organisation"
	}
	return "Me"
}

// CanEdit reports a permission granted to this item by Bitwarden.
// Personal items predate these organization-only permission properties.
func (i *Item) CanEdit() bool {
	if allowed, ok := i.Raw["edit"].(bool); ok {
		return allowed
	}
	return stringValue(i.Raw["organizationId"]) == ""
}

// CanViewSecrets reports permission to reveal protected credentials.
func (i *Item) CanViewSecrets() bool {
	if allowed, ok := i.Raw["viewPassword"].(bool); ok {
		return allowed
	}
	return stringValue(i.Raw["organizationId"]) == ""
}

// CanPushItemChange checks restrictions before a remote item write.
func CanPushItemChange(before, after map[string]any) bool {
	current := before
	if current == nil {
		current = after
	}
	if current == nil {
		return false
	}
	item := &Item{Raw: current}
	if !item.CanEdit() {
		return false
	}
	if before == nil && stringValue(current["organizationId"]) != "" {
		return false
	}
	if !item.CanViewSecrets() && before != nil && after != nil {
		for _, path := range protectedItemPaths() {
			if !reflect.DeepEqual(getPath(before, path), getPath(after, path)) {
				return false
			}
		}
		a, _ := before["fields"].([]any)
		b, _ := after["fields"].([]any)
		// Reordering/removing hidden fields cannot bypass the edit restriction.
		if !reflect.DeepEqual(hiddenCustomFields(a), hiddenCustomFields(b)) {
			return false
		}
	}
	return true
}

func hiddenCustomFields(fields []any) []any {
	result := []any{}
	for _, raw := range fields {
		field, ok := raw.(map[string]any)
		if ok && intValue(field["type"]) == 1 {
			result = append(result, field)
		}
	}
	return result
}

func (i *Item) Favorite() bool { return i.Raw["favorite"] == true }
func (i *Item) Subtitle() string {
	switch i.Type() {
	case Login:
		return stringValue(section(i.Raw, "login")["username"])
	case Card:
		return stringValue(section(i.Raw, "card")["brand"])
	case Identity:
		m := section(i.Raw, "identity")
		return strings.TrimSpace(stringValue(m["firstName"]) + " " + stringValue(m["lastName"]))
	case SSHKey:
		return stringValue(section(i.Raw, "sshKey")["fingerprint"])
	default:
		// Secure Notes use the first line, like other item previews. The
		// renderer masks it if master-password reprompt has not passed.
		value := i.Raw["notes"]
		if i.Cipher != nil {
			opened, err := i.Cipher.Open(value)
			if err != nil {
				return ""
			}
			return strings.SplitN(opened, "\n", 2)[0]
		}
		return strings.SplitN(stringValue(value), "\n", 2)[0]
	}
}

// MaskedSubtitle produces a preview without opening protected notes.
// Its length is all that is exposed while master-password reprompt is locked.
func (i *Item) MaskedSubtitle() string {
	switch i.Type() {
	case Login:
		return Mask(section(i.Raw, "login")["username"])
	case Card:
		return Mask(section(i.Raw, "card")["brand"])
	case Identity:
		return Mask(strings.TrimSpace(stringValue(section(i.Raw, "identity")["firstName"]) +
			" " + stringValue(section(i.Raw, "identity")["lastName"])))
	case SSHKey:
		return Mask(section(i.Raw, "sshKey")["fingerprint"])
	default:
		// Mask sealed notes directly, without decrypting them for the list.
		return Mask(i.Raw["notes"])
	}
}
func stringValue(v any) string {
	if v == nil {
		return ""
	}
	if _, sealed := v.(*Sealed); sealed {
		return ""
	}
	return fmt.Sprint(v)
}
func intValue(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	default:
		return 0
	}
}
func section(m map[string]any, k string) map[string]any {
	if x, ok := m[k].(map[string]any); ok {
		return x
	}
	return map[string]any{}
}
func isHidden(item map[string]any, path []any) bool {
	if len(path) == 2 {
		for _, protected := range protectedItemPaths() {
			if reflect.DeepEqual(path, protected) {
				return true
			}
		}
	}

	if len(path) == 3 && path[0] == "fields" && path[2] == "value" {
		fields, _ := item["fields"].([]any)
		idx, ok := path[1].(int)
		if ok && idx >= 0 && idx < len(fields) {
			m, _ := fields[idx].(map[string]any)
			return intValue(m["type"]) == 1
		}
	}
	return false
}
func getPath(m map[string]any, path []any) any {
	var cur any = m
	for _, p := range path {
		switch x := cur.(type) {
		case map[string]any:
			k, ok := p.(string)
			if !ok {
				return nil
			}
			cur = x[k]
		case []any:
			idx, ok := p.(int)
			if !ok || idx < 0 || idx >= len(x) {
				return nil
			}
			cur = x[idx]
		default:
			return nil
		}
	}
	return cur
}
func setPath(m map[string]any, path []any, value any) error {
	if len(path) == 0 {
		return fmt.Errorf("empty path")
	}
	var cur any = m
	for _, p := range path[:len(path)-1] {
		switch x := cur.(type) {
		case map[string]any:
			k := p.(string)
			v, ok := x[k]
			if !ok || v == nil {
				v = map[string]any{}
				x[k] = v
			}
			cur = v
		case []any:
			idx := p.(int)
			if idx < 0 || idx >= len(x) {
				return fmt.Errorf("index out of range")
			}
			cur = x[idx]
		}
	}
	switch x := cur.(type) {
	case map[string]any:
		x[path[len(path)-1].(string)] = value
	case []any:
		x[path[len(path)-1].(int)] = value
	default:
		return fmt.Errorf("invalid nested path")
	}
	return nil
}

// SealItem immediately replaces all hidden values with ephemeral AES-GCM ciphertext.
// protectedItemPaths is shared by encryption and the final push guard.
func protectedItemPaths() [][]any {
	return [][]any{
		{"login", "password"}, {"login", "totp"},
		{"card", "number"}, {"card", "code"},
		{"identity", "ssn"}, {"identity", "passportNumber"},
		{"identity", "licenseNumber"}, {"sshKey", "privateKey"},
	}
}

func SealItem(m map[string]any, c *Cipher) error {
	// Notes are readable fields, but may contain secrets; seal them in memory.
	// Do not add them to protectedItemPaths: Bitwarden does not hide notes
	// from users with the viewPassword restriction.
	if notes, ok := m["notes"].(string); ok && notes != "" {
		sealed, err := c.Seal(notes)
		if err != nil {
			return err
		}
		m["notes"] = sealed
	}
	paths := protectedItemPaths()
	if fields, ok := m["fields"].([]any); ok {
		for idx := range fields {
			paths = append(paths, []any{"fields", idx, "value"})
		}
	}
	for _, p := range paths {
		if !isHidden(m, p) {
			continue
		}
		if s, ok := getPath(m, p).(string); ok && s != "" {
			sealed, err := c.Seal(s)
			if err != nil {
				return err
			}
			if err = setPath(m, p, sealed); err != nil {
				return err
			}
		}
	}
	return nil
}
func (i *Item) Set(path []any, value string) error {
	var v any = value
	if i.Cipher != nil && (isHidden(i.Raw, path) ||
		(len(path) == 1 && path[0] == "notes")) {
		prev, err := i.Cipher.Open(getPath(i.Raw, path))
		if err != nil {
			return err
		}
		if prev == value {
			return nil
		}
		v, err = i.Cipher.Seal(value)
		if err != nil {
			return err
		}
	}
	return setPath(i.Raw, path, v)
}
func (i *Item) Fields() []Field {
	result := []Field{}
	add := func(name string, path []any, hidden bool) {
		result = append(result, Field{Name: name, Value: getPath(i.Raw, path), Hidden: hidden, Editable: true, Path: path})
	}
	for _, definition := range FieldDefinitions(i.Type()) {
		if definition.Websites {
			uris, ok := section(i.Raw, "login")["uris"].([]any)
			if !ok || len(uris) == 0 {
				continue
			}
			values := make([]string, 0, len(uris))
			for _, raw := range uris {
				entry, _ := raw.(map[string]any)
				values = append(values, stringValue(entry["uri"]))
			}
			result = append(result, Field{
				Name: definition.Name, Value: strings.Join(values, "\n"),
				Editable: true, Websites: true, Values: values,
				Path: []any{"login", "uris"},
			})
			continue
		}
		path := make([]any, len(definition.Path))
		for n, part := range definition.Path {
			path[n] = part
		}
		add(definition.Name, path, definition.Hidden)
	}

	result = append(result, Field{Name: "Master password reprompt", Value: i.Raw["reprompt"] == float64(1) || i.Raw["reprompt"] == 1, Reprompt: true, Editable: true, Path: []any{"reprompt"}})
	if arr, ok := i.Raw["fields"].([]any); ok {
		for idx, raw := range arr {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name := stringValue(m["name"])
			if name == "" {
				name = "Custom field"
			}
			if intValue(m["type"]) == 3 {
				id := intValue(m["linkedId"])
				target := "Username"
				if id == 101 {
					target = "Password"
				}
				result = append(result, Field{Name: name, Value: target, Custom: true, Linked: true, LinkedID: id, Editable: true, FieldType: 3, Path: []any{"fields", idx, "linkedId"}})
			} else {
				result = append(result, Field{Name: name, Value: m["value"], Hidden: intValue(m["type"]) == 1, Editable: true, Custom: true, FieldType: intValue(m["type"]), Path: []any{"fields", idx, "value"}})
			}
		}
	}
	if !i.CanEdit() || !i.CanViewSecrets() {
		for index := range result {
			f := &result[index]
			f.Editable = f.Editable && i.CanEdit()
			if !i.CanViewSecrets() && (f.Hidden || f.Linked && f.LinkedID == 101) {
				f.Hidden = true
				f.Restricted = true
				f.Editable = false
			}
		}
	}
	return result
}
func SortedItems(items []*Item) {
	sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Name()) < strings.ToLower(items[j].Name()) })
}
func NewItem(typ int, name, id string, c *Cipher) *Item {
	m := map[string]any{"id": id, "type": typ, "name": name, "favorite": false, "fields": []any{}, "organizationId": nil, "folderId": nil}
	switch typ {
	case Login:
		m["login"] = map[string]any{"username": "", "password": "", "totp": "", "uris": []any{}}
	case SecureNote:
		m["secureNote"] = map[string]any{"type": 0}
	case Card:
		m["card"] = map[string]any{}
	case Identity:
		m["identity"] = map[string]any{}
	case SSHKey:
		m["sshKey"] = map[string]any{}
	}
	return &Item{Raw: m, Cipher: c}
}
