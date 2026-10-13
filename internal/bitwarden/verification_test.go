package bitwarden

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

type verificationRunner struct {
	mu      sync.Mutex
	calls   []string
	items   string
	folders string
	fail    string
}

func (r *verificationRunner) Run(
	_ context.Context, args []string, _ []byte, _ []string,
) ([]byte, []byte, int, error) {
	name := strings.Join(args, " ")
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name)
	if name == r.fail {
		return nil, []byte("network unavailable"), 1, nil
	}
	switch name {
	case "sync":
		return []byte(`{}`), nil, 0, nil
	case "list items":
		return []byte(r.items), nil, 0, nil
	case "list folders":
		return []byte(r.folders), nil, 0, nil
	default:
		return nil, []byte("unexpected command"), 1, nil
	}
}

func (r *verificationRunner) clearCalls() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = nil
}

func (r *verificationRunner) getCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

// TestVerifyRemoteState protects both visible data and encrypted secret fields.
func TestVerifyRemoteState(t *testing.T) {
	ctx := context.Background()
	r := &verificationRunner{
		items: `[{
            "id":"a", "name":"A", "revisionDate":"2026-01-01",
            "login":{"password":"secret"}, "_unused":null
        }]`,
		folders: `[{"id":"folder", "name":"Personal"}]`,
	}
	c, err := NewClient(r)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Cipher.Close()
	originalItems, err := c.verificationList(ctx, "items", true)
	if err != nil {
		t.Fatal(err)
	}
	originalFolders, err := c.verificationList(ctx, "folders", false)
	if err != nil {
		t.Fatal(err)
	}
	originalItems["a"]["_owner_name"] = "Local decoration"
	r.clearCalls()
	if err := c.VerifyRemoteState(ctx, originalItems, originalFolders); err != nil {
		t.Fatalf("same state must pass: %v", err)
	}
	calls := r.getCalls()
	if len(calls) != 3 || calls[0] != "sync" ||
		!((calls[1] == "list items" && calls[2] == "list folders") ||
			(calls[1] == "list folders" && calls[2] == "list items")) {
		t.Fatalf("unexpected verification commands: %v", calls)
	}
	// A locally staged modification has no effect on confirmed originals.
	staged := vault.Clone(originalItems["a"])
	staged["name"] = "Unregistered local change"
	if err := c.VerifyRemoteState(ctx, originalItems, originalFolders); err != nil {
		t.Fatalf("staged changes must not create a false conflict: %v", err)
	}

	cases := []struct {
		name    string
		items   string
		folders string
	}{
		{"modified item", `[{"id":"a","name":"Different","revisionDate":"2026-01-01","login":{"password":"secret"},"_unused":null}]`, r.folders},
		{"secret changed", `[{"id":"a","name":"A","revisionDate":"2026-01-01","login":{"password":"new secret"},"_unused":null}]`, r.folders},
		{"revision changed", `[{"id":"a","name":"A","revisionDate":"2026-01-02","login":{"password":"secret"},"_unused":null}]`, r.folders},
		{"item added", `[{"id":"a","name":"A","revisionDate":"2026-01-01","login":{"password":"secret"},"_unused":null},{"id":"b","name":"B"}]`, r.folders},
		{"item deleted", `[]`, r.folders},
		{"folder changed", r.items, `[{"id":"folder","name":"Changed"}]`},
		{"folder deleted", r.items, `[]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r.items, r.folders = tc.items, tc.folders
			err := c.VerifyRemoteState(ctx, originalItems, originalFolders)
			if !errors.Is(err, ErrRemoteChanged) {
				t.Fatalf("expected remote conflict, got %v", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("an error message leaked a secret")
			}
		})
	}
	r.items = `[{"id":"a","name":"A","revisionDate":"2026-01-01","login":{"password":"secret"},"_unused":null}]`
	r.folders = `[{"id":"folder","name":"Personal"}]`
	for _, command := range []string{"sync", "list items", "list folders"} {
		t.Run("fail "+command, func(t *testing.T) {
			r.fail = command
			err := c.VerifyRemoteState(ctx, originalItems, originalFolders)
			if err == nil {
				t.Fatal("an incomplete verification must fail closed")
			}
			r.fail = ""
		})
	}
}

// TestVerificationRejectsMalformedLists prevents a partial response from passing.
func TestVerificationRejectsMalformedLists(t *testing.T) {
	c, err := NewClient(&verificationRunner{
		items: `[{"id":"same"},{"id":"same"}]`, folders: `[]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Cipher.Close()
	if err := c.VerifyRemoteState(context.Background(), nil, nil); err == nil {
		t.Fatal("duplicate identifiers must prevent writes")
	}
}

// TestCompareRemoteStateReturnsDiffsWithoutExtraCalls checks the popup input.
func TestCompareRemoteStateReturnsDiffsWithoutExtraCalls(t *testing.T) {
	runner := &verificationRunner{
		items: `[{
            "id":"updated", "name":"Changed", "type":1,
            "login":{"password":"changed secret"}
        }, {"id":"added", "name":"New", "type":1}]`,
		folders: `[]`,
	}
	client, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cipher.Close()
	before := map[string]map[string]any{
		"updated": {"id": "updated", "name": "Previous", "type": float64(1)},
		"removed": {"id": "removed", "name": "Old", "type": float64(1)},
	}
	review, err := client.CompareRemoteState(context.Background(), before, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Differences) != 3 {
		t.Fatalf("expected N/M/D, got %+v", review.Differences)
	}
	expected := map[string]vault.Kind{
		"added":   vault.New,
		"updated": vault.Modified,
		"removed": vault.Deleted,
	}
	for _, change := range review.Differences {
		if change.Kind != expected[change.ID] {
			t.Fatalf("wrong kind for %s: %s", change.ID, change.Kind)
		}
		if strings.Contains(change.Name, "secret") {
			t.Fatal("sensitive field in popup label")
		}
	}
	if len(runner.getCalls()) != 3 {
		t.Fatalf("conflict review must reuse verification calls: %v", runner.getCalls())
	}
}
