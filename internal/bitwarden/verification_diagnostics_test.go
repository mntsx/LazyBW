package bitwarden

import (
	"context"
	"strings"
	"testing"
)

// TestVerificationDiagnosticsExposeOnlySafeFailureCategories checks diagnostic output.
func TestVerificationDiagnosticsExposeOnlySafeFailureCategories(t *testing.T) {
	cases := []struct {
		name    string
		items   string
		folders string
		want    string
	}{
		{"missing item ID", `[{"name":"private item","login":{"password":"secret"}}]`, `[]`, "bw list items returned an entry without an ID"},
		{"duplicate item ID", `[{"id":"dup","name":"secret"},{"id":"dup","name":"private"}]`, `[]`, "bw list items returned duplicate IDs"},
		{"null items list", `null`, `[]`, "bw list items returned null instead of a list"},
		{"null items entry", `[null]`, `[]`, "bw list items returned an empty entry"},
		{"missing folder ID", `[]`, `[{"name":"confidential folder"}]`, "bw list folders returned an entry without an ID"},
		{"duplicate folder ID", `[]`, `[{"id":"x"},{"id":"x"}]`, "bw list folders returned duplicate IDs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli, err := NewClient(&verificationRunner{items: tc.items, folders: tc.folders})
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Cipher.Close()
			_, err = cli.CompareRemoteState(context.Background(), nil, nil)
			if err == nil {
				t.Fatal("invalid remote state must fail closed")
			}
			got := VerificationFailure(err)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			for _, secret := range []string{"private item", "secret", "confidential folder"} {
				if strings.Contains(got, secret) {
					t.Fatalf("diagnostic leaked a value: %q", got)
				}
			}
		})
	}
}

// TestVerificationDiagnosticsReportsInMemoryCipherFailure checks secret protection.
func TestVerificationDiagnosticsReportsInMemoryCipherFailure(t *testing.T) {
	cli, err := NewClient(&verificationRunner{
		items:   `[{"id":"abc","login":{"password":"SECRET_DO_NOT_LOG"}}]`,
		folders: `[]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	cli.Cipher.Close()
	_, err = cli.CompareRemoteState(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("closed cipher must fail closed")
	}
	got := VerificationFailure(err)
	if got != "Unable to protect remote secrets in memory" {
		t.Fatalf("unexpected error: %q", got)
	}
}
