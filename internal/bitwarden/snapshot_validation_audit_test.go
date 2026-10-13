package bitwarden

import (
	"context"
	"strings"
	"testing"
)

// TestSnapshotRejectsMalformedCLILists checks the initial backup's trust boundary.
func TestSnapshotRejectsMalformedCLILists(t *testing.T) {
	cases := []struct {
		name, entity, input string
		valid               bool
	}{
		{"null items", "items", `null`, false},
		{"null item entry", "items", `[null]`, false},
		{"missing item ID", "items", `[{"name":"A"}]`, false},
		{"duplicate item ID", "items", `[{"id":"1"},{"id":"1"}]`, false},
		{"null folders", "folders", `null`, false},
		{"null folder entry", "folders", `[null]`, false},
		{"missing folder ID", "folders", `[{"name":"A"}]`, false},
		{"duplicate folder ID", "folders", `[{"id":"1"},{"id":"1"}]`, false},
		{"empty items", "items", `[]`, true},
		{"virtual no folder", "folders", `[{"name":"No Folder"}]`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &verificationRunner{items: `[]`, folders: `[]`}
			if tc.entity == "items" {
				runner.items = tc.input
			} else {
				runner.folders = tc.input
			}
			client, err := NewClient(runner)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Cipher.Close()
			items, folders, _, err := client.Snapshot(context.Background())
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
			if !tc.valid {
				if items != nil || folders != nil {
					t.Fatal("partial snapshot escaped validation")
				}
				if strings.Contains(err.Error(), tc.input) && len(tc.input) > 10 {
					t.Fatal("untrusted CLI payload leaked through validation error")
				}
			}
			calls := runner.getCalls()
			if len(calls) == 0 || calls[0] != "sync" {
				t.Fatalf("snapshot must synchronize first: %v", calls)
			}
		})
	}
}

// TestSnapshotNeverUsesCacheWhenSyncFails prevents stale-backup writes.
func TestSnapshotNeverUsesCacheWhenSyncFails(t *testing.T) {
	runner := &verificationRunner{items: `[]`, folders: `[]`, fail: "sync"}
	client, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cipher.Close()
	if _, _, _, err := client.Snapshot(context.Background()); err == nil {
		t.Fatal("a failed remote sync must not authorize a snapshot")
	}
	if calls := runner.getCalls(); len(calls) != 1 || calls[0] != "sync" {
		t.Fatalf("cached lists must not be read after sync failure: %v", calls)
	}
}
