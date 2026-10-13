package bitwarden

import (
	"context"
	"strings"
	"testing"
)

// TestVirtualNoFolderIsIgnoredAcrossSnapshots checks startup and verification.
func TestVirtualNoFolderIsIgnoredAcrossSnapshots(t *testing.T) {
	ctx := context.Background()
	runner := &verificationRunner{
		items: `[{"id":"item-1","name":"Test","type":1}]`,
		folders: `[
			{"object":"folder","id":null,"name":"No Folder"},
			{"object":"folder","id":"folder-1","name":"Personal"}
		]`,
	}
	client, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cipher.Close()

	items, folders, _, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || folders[0]["id"] != "folder-1" {
		t.Fatalf("startup must exclude the virtual folder: %+v", folders)
	}
	beforeItems := map[string]map[string]any{items[0].ID(): items[0].Raw}
	beforeFolders := map[string]map[string]any{"folder-1": folders[0]}
	review, err := client.CompareRemoteState(ctx, beforeItems, beforeFolders)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Differences) != 0 || len(review.Folders) != 1 {
		t.Fatalf("synthetic folder caused a false conflict: %+v", review)
	}

	// The unfiled-items label is also emitted without an id by some clients.
	runner.folders = `[{"name":"Items with no folder"},{"id":"folder-1","name":"Personal","object":"folder"}]`
	review, err = client.CompareRemoteState(ctx, beforeItems, beforeFolders)
	if err != nil || len(review.Differences) != 0 {
		t.Fatalf("unfiled virtual folder must not cause a conflict: %v, %+v", err, review)
	}
}

// TestRealFolderWithoutIDStillBlocksWrites guards against malformed responses.
func TestRealFolderWithoutIDStillBlocksWrites(t *testing.T) {
	for _, folders := range []string{
		`[{"name":"Personal"}]`,
		`[{"id":null,"name":"Personal"}]`,
		`[{"id":"folder-1","name":"No Folder"},{"name":"Personal"}]`,
	} {
		client, err := NewClient(&verificationRunner{items: `[]`, folders: folders})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.CompareRemoteState(context.Background(), nil, nil)
		client.Cipher.Close()
		if err == nil || !strings.Contains(VerificationFailure(err), "without an ID") {
			t.Fatalf("bad real folder must be rejected, input=%q, err=%v", folders, err)
		}
	}
}

// TestRealFolderNamedNoFolderIsPreserved guards against dropping user folders.
func TestRealFolderNamedNoFolderIsPreserved(t *testing.T) {
	input := []map[string]any{
		{"id": "real-id", "name": "No Folder"},
		{"id": nil, "name": "No Folder"},
	}
	got := withoutVirtualNoFolder(input)
	if len(got) != 1 || got[0]["id"] != "real-id" {
		t.Fatalf("user folder was lost: %+v", got)
	}
}
