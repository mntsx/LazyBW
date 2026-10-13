package bitwarden

import (
	"context"
	"strings"
	"testing"
)

// TestVerificationFailureReportsSafeCLIStage checks actionable error diagnostics.
func TestVerificationFailureReportsSafeCLIStage(t *testing.T) {
	r := &verificationRunner{items: "[]", folders: "[]"}
	c, err := NewClient(r)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Cipher.Close()
	for _, command := range []string{"sync", "list items", "list folders"} {
		r.fail = command
		_, err := c.CompareRemoteState(context.Background(), nil, nil)
		if err == nil {
			t.Fatalf("expected failure at %s", command)
		}
		msg := VerificationFailure(err)
		if !strings.Contains(msg, "bw "+command) ||
			!strings.Contains(msg, "network") || strings.Contains(msg, "secret") {
			t.Fatalf("bad diagnostic for %s: %q", command, msg)
		}
		r.fail = ""
	}
}
