package bitwarden

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestAuthenticationErrorsAreEvenAndWithoutTrailingDots checks auth labels.
func TestAuthenticationErrorsAreEvenAndWithoutTrailingDots(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		master bool
	}{
		{name: "unknown error", err: errors.New("failure")},
		{name: "network", err: &CLIError{Category: "network"}},
		{name: "rate limited", err: &CLIError{Category: "rate limited"}},
		{name: "challenge", err: &CLIError{Category: "challenge"}},
		{name: "execution", err: &CLIError{Category: "execution"}},
		{name: "vault locked", err: &CLIError{Category: "vault locked"}},
		{name: "master invalid", err: &CLIError{
			Operation: "bw unlock", Category: "unknown", Code: 1,
		}, master: true},
		{name: "master fallback", err: &CLIError{
			Operation: "bw status", Category: "unknown", Code: 1,
		}, master: true},
		{name: "invalid credentials", err: &CLIError{Category: "invalid credentials"}},
		{name: "authentication", err: &CLIError{Category: "authentication"}},
		{name: "permissions", err: &CLIError{Category: "permissions"}},
		{name: "validation", err: &CLIError{Category: "validation"}},
		{name: "unknown", err: &CLIError{Category: "unknown"}},
		{name: "fallback", err: &CLIError{Category: "other"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := AuthenticationMessage(test.err, test.master)
			if strings.HasSuffix(message, ".") {
				t.Errorf("error has final period: %q", message)
			}
			if utf8.RuneCountInString(message)%2 != 0 {
				t.Errorf("error cannot center on an even-width terminal: %q", message)
			}
		})
	}
}
