package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type jsonValueNode struct {
	start, end int
	keyStart   int
	object     *jsonObjectNode
}

type jsonObjectNode struct {
	start, end   int
	depth        int
	lastValueEnd int
	fields       map[string]*jsonValueNode
}

type jsonParser struct {
	data []byte
	pos  int
}

func parseJSONTree(data []byte) (*jsonValueNode, error) {
	parser := &jsonParser{data: data}
	value, err := parser.value(0)
	if err != nil {
		return nil, err
	}
	parser.space()
	if parser.pos != len(data) {
		return nil, errors.New("unexpected data after JSON value")
	}
	return value, nil
}

func (parser *jsonParser) space() {
	for parser.pos < len(parser.data) {
		switch parser.data[parser.pos] {
		case ' ', '\t', '\r', '\n':
			parser.pos++
		default:
			return
		}
	}
}

func (parser *jsonParser) value(depth int) (*jsonValueNode, error) {
	parser.space()
	if parser.pos >= len(parser.data) {
		return nil, errors.New("unexpected end of JSON")
	}
	start := parser.pos
	switch parser.data[parser.pos] {
	case '{':
		object, err := parser.object(depth)
		if err != nil {
			return nil, err
		}
		return &jsonValueNode{start: start, end: parser.pos, object: object}, nil
	case '[':
		if err := parser.array(depth); err != nil {
			return nil, err
		}
	case '"':
		if _, err := parser.string(); err != nil {
			return nil, err
		}
	default:
		for parser.pos < len(parser.data) && !strings.ContainsRune(" \t\r\n,]}", rune(parser.data[parser.pos])) {
			parser.pos++
		}
		if parser.pos == start {
			return nil, fmt.Errorf("invalid JSON value at byte %d", start)
		}
	}
	return &jsonValueNode{start: start, end: parser.pos}, nil
}

func (parser *jsonParser) object(depth int) (*jsonObjectNode, error) {
	object := &jsonObjectNode{start: parser.pos, depth: depth, fields: map[string]*jsonValueNode{}}
	parser.pos++
	parser.space()
	if parser.pos < len(parser.data) && parser.data[parser.pos] == '}' {
		object.end = parser.pos
		parser.pos++
		return object, nil
	}
	for {
		parser.space()
		keyStart := parser.pos
		key, err := parser.string()
		if err != nil {
			return nil, err
		}
		parser.space()
		if parser.pos >= len(parser.data) || parser.data[parser.pos] != ':' {
			return nil, fmt.Errorf("missing colon at byte %d", parser.pos)
		}
		parser.pos++
		value, err := parser.value(depth + 1)
		if err != nil {
			return nil, err
		}
		value.keyStart = keyStart
		object.fields[key] = value
		object.lastValueEnd = value.end
		parser.space()
		if parser.pos >= len(parser.data) {
			return nil, errors.New("unterminated JSON object")
		}
		switch parser.data[parser.pos] {
		case ',':
			parser.pos++
			continue
		case '}':
			object.end = parser.pos
			parser.pos++
			return object, nil
		default:
			return nil, fmt.Errorf("invalid JSON object separator at byte %d", parser.pos)
		}
	}
}

func (parser *jsonParser) array(depth int) error {
	parser.pos++
	parser.space()
	if parser.pos < len(parser.data) && parser.data[parser.pos] == ']' {
		parser.pos++
		return nil
	}
	for {
		if _, err := parser.value(depth + 1); err != nil {
			return err
		}
		parser.space()
		if parser.pos >= len(parser.data) {
			return errors.New("unterminated JSON array")
		}
		switch parser.data[parser.pos] {
		case ',':
			parser.pos++
		case ']':
			parser.pos++
			return nil
		default:
			return fmt.Errorf("invalid JSON array separator at byte %d", parser.pos)
		}
	}
}

func (parser *jsonParser) string() (string, error) {
	if parser.pos >= len(parser.data) || parser.data[parser.pos] != '"' {
		return "", fmt.Errorf("expected JSON string at byte %d", parser.pos)
	}
	start := parser.pos
	parser.pos++
	escaped := false
	for parser.pos < len(parser.data) {
		character := parser.data[parser.pos]
		parser.pos++
		if escaped {
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if character == '"' {
			var value string
			if err := json.Unmarshal(parser.data[start:parser.pos], &value); err != nil {
				return "", err
			}
			return value, nil
		}
	}
	return "", errors.New("unterminated JSON string")
}

type jsonInsertion struct {
	position int
	content  string
}

func defaultObject(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err = json.Unmarshal(encoded, &object); err != nil {
		return nil, err
	}
	return object, nil
}

func legacyCompatibleDefaults(filename string, existing, defaults map[string]any) {
	if !strings.HasPrefix(filename, "keybinds.") {
		return
	}
	if _, exists := existing["edits"]; !exists {
		if _, legacy := existing["changes"].(map[string]any); legacy {
			// Treat the old section as the canonical section while it remains in
			// the file, so later user edits keep taking effect without duplicates.
			defaults["changes"] = defaults["edits"]
			delete(defaults, "edits")
		}
	}
	global, _ := existing["global"].(map[string]any)
	globalDefaults, _ := defaults["global"].(map[string]any)
	if _, canonical := global["push_all"]; !canonical {
		if _, legacy := global["save_all"]; legacy {
			// save_all already supplies this action through the compatibility
			// loader; do not persist a competing push_all value beside it.
			delete(globalDefaults, "push_all")
		}
	}
}

func collectDefaultInsertions(source []byte, object *jsonObjectNode, defaults map[string]any, edits *[]jsonInsertion) error {
	keys := make([]string, 0, len(defaults))
	for key := range defaults {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	missing := make([]string, 0)
	for _, key := range keys {
		current := object.fields[key]
		childDefaults, nested := defaults[key].(map[string]any)
		if current == nil {
			missing = append(missing, key)
			continue
		}
		if nested && current.object != nil {
			if err := collectDefaultInsertions(source, current.object, childDefaults, edits); err != nil {
				return err
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	closeIndent, insertionPosition, startsOnNewLine := objectClosingIndent(source, object)
	childIndent := objectChildIndent(source, object, closeIndent)
	entries := make([]string, 0, len(missing))
	for _, key := range missing {
		encoded, err := json.MarshalIndent(defaults[key], "", "  ")
		if err != nil {
			return err
		}
		value := strings.ReplaceAll(string(encoded), "\n", "\n"+childIndent)
		name, _ := json.Marshal(key)
		entries = append(entries, childIndent+string(name)+": "+value)
	}
	prefix := ""
	if !startsOnNewLine {
		prefix = "\n"
	}
	content := prefix + strings.Join(entries, ",\n") + "\n" + closeIndent
	*edits = append(*edits, jsonInsertion{position: insertionPosition, content: content})
	if len(object.fields) > 0 && !hasTrailingComma(source[object.lastValueEnd:object.end]) {
		*edits = append(*edits, jsonInsertion{position: object.lastValueEnd, content: ","})
	}
	return nil
}

func objectClosingIndent(source []byte, object *jsonObjectNode) (string, int, bool) {
	lineStart := lineStartAt(source, object.end)
	prefix := source[lineStart:object.end]
	if len(bytesTrimSpace(prefix)) == 0 {
		return string(prefix), lineStart, true
	}
	indent := strings.Repeat(indentationUnit(source), object.depth)
	return indent, object.end, false
}

// objectChildIndent preserves the indentation already used by an object's
// fields. Inline or empty objects have no such reference, so use the file's
// indentation unit instead of assuming two spaces.
func objectChildIndent(source []byte, object *jsonObjectNode, closeIndent string) string {
	for _, value := range object.fields {
		lineStart := lineStartAt(source, value.keyStart)
		prefix := source[lineStart:value.keyStart]
		if len(prefix) > 0 && len(bytesTrimSpace(prefix)) == 0 {
			return string(prefix)
		}
	}
	return closeIndent + indentationUnit(source)
}

func lineStartAt(source []byte, position int) int {
	for position > 0 && source[position-1] != '\n' && source[position-1] != '\r' {
		position--
	}
	return position
}

func indentationUnit(source []byte) string {
	unit := ""
	for lineStart := 0; lineStart < len(source); {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' && source[lineEnd] != '\r' {
			lineEnd++
		}
		prefixEnd := lineStart
		for prefixEnd < lineEnd && (source[prefixEnd] == ' ' || source[prefixEnd] == '\t') {
			prefixEnd++
		}
		if prefixEnd > lineStart && prefixEnd < lineEnd {
			candidate := string(source[lineStart:prefixEnd])
			if unit == "" || len(candidate) < len(unit) {
				unit = candidate
			}
		}
		lineStart = lineEnd + 1
		if lineEnd < len(source) && source[lineEnd] == '\r' && lineStart < len(source) && source[lineStart] == '\n' {
			lineStart++
		}
	}
	if unit == "" {
		return "  "
	}
	return unit
}

func bytesTrimSpace(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}

func hasTrailingComma(source []byte) bool {
	lineComment, blockComment := false, false
	for index := 0; index < len(source); index++ {
		if lineComment {
			if source[index] == '\n' || source[index] == '\r' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if source[index] == '*' && index+1 < len(source) && source[index+1] == '/' {
				blockComment = false
				index++
			}
			continue
		}
		if source[index] == '/' && index+1 < len(source) {
			switch source[index+1] {
			case '/':
				lineComment = true
				index++
				continue
			case '*':
				blockComment = true
				index++
				continue
			}
		}
		if source[index] == ',' {
			return true
		}
	}
	return false
}

func applyInsertions(source []byte, insertions []jsonInsertion) []byte {
	sort.SliceStable(insertions, func(first, second int) bool {
		return insertions[first].position > insertions[second].position
	})
	result := append([]byte(nil), source...)
	for _, insertion := range insertions {
		result = append(result[:insertion.position], append([]byte(insertion.content), result[insertion.position:]...)...)
	}
	return result
}

var errConfigChanged = errors.New("configuration changed while defaults were being added")

func writeConfigAtomic(path string, content, expected []byte, requireMissing bool) error {
	writePath := path
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil {
			return resolveErr
		}
		writePath = resolved
	}
	mode := os.FileMode(0600)
	if info, err := os.Stat(writePath); err == nil {
		mode = info.Mode().Perm()
	}
	file, err := os.CreateTemp(filepath.Dir(writePath), ".lazybw-config-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err = file.Chmod(mode); err == nil {
		_, err = file.Write(content)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if requireMissing {
		// A hard link in the same directory publishes the fully synced temporary
		// file atomically and fails rather than replacing a concurrently created file.
		if err = os.Link(temporary, writePath); err != nil {
			if errors.Is(err, os.ErrExist) {
				return errConfigChanged
			}
			return err
		}
	} else {
		current, readErr := os.ReadFile(writePath)
		if readErr != nil {
			return readErr
		}
		current = bytes.TrimPrefix(current, []byte{0xef, 0xbb, 0xbf})
		if !bytes.Equal(current, expected) {
			return errConfigChanged
		}
		if err = os.Rename(temporary, writePath); err != nil {
			return err
		}
	}
	directory, err := os.Open(filepath.Dir(writePath))
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func addMissingDefaults(path string, source, normalized []byte, defaults any) error {
	root, err := parseJSONTree(normalized)
	if err != nil || root.object == nil {
		return err
	}
	defaultValues, err := defaultObject(defaults)
	if err != nil {
		return err
	}
	var existing map[string]any
	if err = json.Unmarshal(normalized, &existing); err != nil {
		return err
	}
	legacyCompatibleDefaults(filepath.Base(path), existing, defaultValues)
	var insertions []jsonInsertion
	if err = collectDefaultInsertions(source, root.object, defaultValues, &insertions); err != nil {
		return err
	}
	if len(insertions) == 0 {
		return nil
	}
	return writeConfigAtomic(path, applyInsertions(source, insertions), source, false)
}
