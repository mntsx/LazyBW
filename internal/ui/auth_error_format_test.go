package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
)

// TestAuthFormErrorLabelsFitEvenWidthScreens checks form-specific errors.
func TestAuthFormErrorLabelsFitEvenWidthScreens(t *testing.T) {
	labels := []string{
		"Cannot read client_env_path. Check the configured file",
		"client_env_path lacks a client ID or client secret",
		"Enter your client ID and client secret, or check client_env_path",
	}
	for _, label := range labels {
		if strings.HasSuffix(label, ".") || utf8.RuneCountInString(label)%2 != 0 {
			t.Errorf("invalid authentication error label: %q", label)
		}
	}
}

// TestManualAuthWithoutCredentialsUsesCenterableMessage checks validation.
func TestManualAuthWithoutCredentialsUsesCenterableMessage(t *testing.T) {
	cli, err := bitwarden.NewClient(&serverFlowRunner{})
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(config.Config{Directory: t.TempDir()}, cli)
	state.Screen = AuthClient
	state.Busy = false
	app := New(state)
	app.submitAuth()
	if state.Notice != "Enter your client ID and client secret, or check client_env_path" {
		t.Fatalf("unexpected error notice: %q", state.Notice)
	}
	if strings.HasSuffix(state.Notice, ".") ||
		utf8.RuneCountInString(state.Notice)%2 != 0 {
		t.Fatalf("invalid error notice formatting: %q", state.Notice)
	}
}
