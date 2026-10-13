package vault

import (
	"reflect"
	"strings"
	"testing"
)

func TestCipherAndNoPlainSecret(t *testing.T) {
	c, err := NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{"type": float64(1), "login": map[string]any{"password": "s3cret", "username": "user"}}
	if err = SealItem(raw, c); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.TrimSpace(raw["login"].(map[string]any)["password"].(*Sealed).String()), "s3cret") {
		t.Fatal("plaintext string representation")
	}
	i := Item{Raw: raw, Cipher: c}
	f := i.Fields()[1]
	if Mask(f.Value) != "••••••" {
		t.Fatal("mask")
	}
	got, err := f.Reveal(c)
	if err != nil || got != "s3cret" {
		t.Fatal(got, err)
	}
	plain, err := PlainMap(c, raw)
	if err != nil || plain["login"].(map[string]any)["password"] != "s3cret" {
		t.Fatal("serialization")
	}
	c.Close()
	if _, err = c.Open(f.Value); err == nil {
		t.Fatal("open after close")
	}
}
func TestStageCoalescingAndFields(t *testing.T) {
	c, _ := NewCipher()
	orig := map[string]any{"id": "id-1", "name": "Test", "type": float64(1), "login": map[string]any{"username": "A", "password": "secret"}}
	_ = SealItem(orig, c)
	originals := map[string]map[string]any{"id-1": Clone(orig)}
	stage := NewStage()
	item := Item{Raw: Clone(orig), Cipher: c}
	_ = item.Set([]any{"login", "username"}, "B")
	stage.Record("item", item.Raw, "Login", originals)
	if stage.Len() != 1 || stage.FieldKind("id-1", []any{"login", "username"}, c) != Modified || stage.FieldKind("id-1", []any{"login", "password"}, c) != "" {
		t.Fatal("incorrect field deltas")
	}
	_ = item.Set([]any{"login", "username"}, "A")
	stage.Record("item", item.Raw, "Login", originals)
	if stage.Len() != 0 {
		t.Fatal("reverted edit should disappear")
	}
	newItem := NewItem(1, "A", LocalID(), c)
	stage.Record("item", newItem.Raw, "Login", originals)
	stage.Delete("item", newItem.ID(), "Login", originals)
	if stage.Len() != 0 {
		t.Fatal("new then deleted should cancel")
	}
	stage.Delete("item", "id-1", "Login", originals)
	if len(stage.Changes()) != 1 || stage.Changes()[0].Kind != Deleted {
		t.Fatal("delete")
	}
}
func TestFuzzyNameOnly(t *testing.T) {
	yes, pos, _ := FuzzyMatch("GitHub", "gh")
	if !yes || !reflect.DeepEqual(pos, []int{0, 3}) {
		t.Fatal(pos)
	}
	yes, _, _ = FuzzyMatch("GitHub", "password")
	if yes {
		t.Fatal("false match")
	}
}
func TestSortedAndIcons(t *testing.T) {
	c, _ := NewCipher()
	a := NewItem(1, "zulu", "a", c)
	b := NewItem(2, "Alpha", "b", c)
	items := []*Item{a, b}
	SortedItems(items)
	if items[0] != b || TypeIcons[1] == "" {
		t.Fatal("sort or icons")
	}
}

func TestReencryptedSecretReversion(t *testing.T) {
	c, _ := NewCipher()
	defer c.Close()
	it := NewItem(Login, "A", "id", c)
	_ = it.Set([]any{"login", "password"}, "old")
	original := map[string]map[string]any{"id": Clone(it.Raw)}
	stage := NewStage()
	_ = it.Set([]any{"login", "password"}, "new")
	stage.Record("item", it.Raw, "Login", original)
	if stage.Len() != 1 {
		t.Fatal("expected modified")
	}
	_ = it.Set([]any{"login", "password"}, "old")
	stage.Record("item", it.Raw, "Login", original)
	if stage.Len() != 0 {
		t.Fatal("reverted hidden value incorrectly staged")
	}
}
func TestCustomFieldNewStatus(t *testing.T) {
	c, _ := NewCipher()
	defer c.Close()
	it := NewItem(Login, "A", "id", c)
	original := map[string]map[string]any{"id": Clone(it.Raw)}
	it.Raw["fields"] = []any{map[string]any{"name": "API key", "value": "", "type": float64(1)}}
	stage := NewStage()
	stage.Record("item", it.Raw, "Login", original)
	if got := stage.FieldKind("id", []any{"fields", 0, "value"}, c); got != New {
		t.Fatalf("expected N, got %q", got)
	}
}
