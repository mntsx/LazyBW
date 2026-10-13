package bitwarden

import (
	"context"
	"strings"
	"testing"
)

type authDiagnosticsRunner struct {
	out, errOut []byte
	code        int
}

func (r authDiagnosticsRunner) Run(
	_ context.Context, _ []string, _ []byte, _ []string,
) ([]byte, []byte, int, error) {
	return r.out, r.errOut, r.code, nil
}

// TestAPIErrorClassificationNeverMislabelsUnknownErrors checks safe login errors.
func TestAPIErrorClassificationNeverMislabelsUnknownErrors(t *testing.T) {
	cases := []struct {
		name       string
		stdout     string
		stderr     string
		want       string
		mustNotSay string
	}{
		{"unknown", "", "Some unexpected exception", "unknown reason", "Invalid client"},
		{"reported in stdout", `{"error":"invalid_client"}`, "", "rejected the client ID", "unknown reason"},
		{"network", "", "getaddrinfo ENOTFOUND identity.bitwarden.com", "Connection failed", "client ID"},
		{"rate limiting", "", "429 Too Many Requests", "limiting login", "client ID"},
		{"auth required", "", "not logged in", "not authenticated", "Invalid client"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := authDiagnosticsRunner{
				out: []byte(tc.stdout), errOut: []byte(tc.stderr), code: 1,
			}
			c, err := NewClient(r)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.call(context.Background(), "bw login --apikey", []string{"login", "--apikey"}, nil, nil, false)
			message := AuthenticationMessage(err, false)
			if !strings.Contains(message, tc.want) || strings.Contains(message, tc.mustNotSay) {
				t.Fatalf("wrong public message: %q", message)
			}
			if strings.Contains(message, tc.stderr) && tc.stderr != "" {
				t.Fatal("raw Bitwarden output escaped to the user")
			}
		})
	}
}
