package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// readbackRunner models bw edit returning fewer fields than a later bw list.
type readbackRunner struct {
	items map[string]map[string]any
}

func (r *readbackRunner) Run(_ context.Context, args []string, _ []byte,
	_ []string) ([]byte, []byte, int, error) {
	switch strings.Join(args, " ") {
	case "sync":
		return []byte(`{}`), nil, 0, nil
	case "list folders":
		return []byte(`[]`), nil, 0, nil
	case "list items":
		list := make([]map[string]any, 0, len(r.items))
		for _, raw := range r.items {
			list = append(list, raw)
		}
		data, err := json.Marshal(list)
		if err != nil {
			return nil, nil, 1, err
		}
		return data, nil, 0, nil
	default:
		return nil, []byte("unrecognized command"), 1, nil
	}
}

// TestPushReadbackAvoidsFalseConflicts checks two pushed items do not become
// unexpected remote modifications when a third item is subsequently edited.
func TestPushReadbackAvoidsFalseConflicts(t *testing.T) {
	ctx := context.Background()
	runner := &readbackRunner{items: map[string]map[string]any{}}
	client, err := bitwarden.NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cipher.Close()
	s := NewState(config.Config{Colors: config.DefaultColors}, client)
	items := make([]*vault.Item, 0, 3)
	for n := 1; n <= 3; n++ {
		id := fmt.Sprintf("id-%d", n)
		item := vault.NewItem(vault.Login, fmt.Sprintf("Item %d", n), id, client.Cipher)
		item.Raw["revisionDate"] = "before"
		// Actual Bitwarden JSON numbers are decoded as float64.
		item.Raw["type"] = float64(vault.Login)
		items = append(items, item)
	}
	s.Loaded(items, nil)
	afterReadback := make([]pushedEntry, 0, 2)
	for _, it := range s.Items {
		if it.ID() == "id-3" {
			continue
		}
		id := it.ID()
		original := vault.Clone(s.Originals[id])
		if err := it.Set([]any{"login", "username"}, "updated"); err != nil {
			t.Fatal(err)
		}
		s.Stage.Record("item", it.Raw, "Login", s.Originals)
		change := s.Stage.Get(vault.Key("item", id))
		// Simulate bw edit output without canonical metadata or full item fields.
		provisional := map[string]any{"id": id, "name": it.Name(),
			"login": map[string]any{"username": "updated"}}
		// The synced list, however, includes all fields and the new revision.
		canonical := vault.Clone(it.Raw)
		canonical["revisionDate"] = "after"
		runner.items[id] = canonical
		s.CommitRemote(change, provisional)
		s.UnverifiedPush = true
		afterReadback = append(afterReadback, pushedEntry{
			entity: "item", id: id, change: &vault.Change{Entity: "item",
				Key: vault.Key("item", id), Kind: vault.Modified,
				Before: original, After: vault.Clone(it.Raw)},
		})
	}
	// Include an unchanged third item in the remote snapshot.
	runner.items["id-3"] = vault.Clone(s.Originals["id-3"])
	canonical, err := client.SyncedSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !s.AcceptPushedSnapshot(canonical, afterReadback) {
		t.Fatal("readback rejected")
	}
	if s.UnverifiedPush {
		t.Fatal("push remained unverified")
	}
	if len(s.Stage.Changes()) != 0 {
		t.Fatal("pushed edits remained staged")
	}
	if s.InitialGrounds != nil {
		t.Fatal("spurious ground tracking created by the push")
	}
	// A later preflight must see no changes to the two previously pushed items.
	review, err := client.CompareRemoteState(ctx, s.Originals, s.FolderOriginals)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Differences) != 0 {
		t.Fatalf("false remote conflict after push: %+v", review.Differences)
	}
}

// TestGroundItemChangedIgnoresServerMetadata avoids arrows on items whose
// revision or other server-generated fields changed without user edits.
func TestGroundItemChangedIgnoresServerMetadata(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	id := s.Items[0].ID()
	s.InitialGrounds = map[string]map[string]any{id: vault.Clone(s.Originals[id])}
	s.Originals[id]["revisionDate"] = "new server revision"
	s.Originals[id]["passwordRevisionDate"] = "new password revision"
	if s.GroundItemChanged(id) {
		t.Fatal("metadata-only change created a green arrow")
	}
	s.Originals[id]["favorite"] = true
	if !s.GroundItemChanged(id) {
		t.Fatal("visible field modification ignored")
	}
}
