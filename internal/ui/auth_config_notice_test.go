package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
)

// TestSavedCredentialsFailuresRemainVisible checks the initial form's errors.
func TestSavedCredentialsFailuresRemainVisible(t *testing.T) {
	for _, tc := range []struct {
		name       string
		content    string
		missing    bool
		pathNull   bool
		expected   string
		loginCalls int
	}{
		{name: "unset path", pathNull: true},
		{name: "missing file", missing: true, expected: "Cannot read client_env_path"},
		{name: "missing ID", content: "BW_CLIENTSECRET=test-secret\n", expected: "lacks a client ID"},
		{name: "missing secret", content: "BW_CLIENTID=user.test\n", expected: "lacks a client ID or client secret"},
		{name: "invalid credentials", content: "BW_CLIENTID=user.test\nBW_CLIENTSECRET=wrong\n",
			expected: "Bitwarden rejected", loginCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "api-key.env")
			if !tc.pathNull && !tc.missing {
				if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.pathNull {
				path = ""
			}
			runner := &serverFlowRunner{}
			cli, err := bitwarden.NewClient(runner)
			if err != nil {
				t.Fatal(err)
			}
			state := NewState(config.Config{
				Directory: dir,
				Authentication: config.Authentication{
					ClientEnvPath: path, DefaultServer: "us",
				},
			}, cli)
			app := New(state)
			app.startupWithUpdates(context.Background(), applyStartupUpdateNow, true)
			if state.Screen != AuthClient || state.Busy {
				t.Fatalf("unexpected screen=%v busy=%v", state.Screen, state.Busy)
			}
			if !strings.Contains(state.Notice, tc.expected) ||
				(tc.expected == "" && state.Notice != "") {
				t.Fatalf("notice=%q, expected substring=%q", state.Notice, tc.expected)
			}
			count := 0
			for _, call := range runner.calls {
				if call == "login --apikey" {
					count++
				}
			}
			if count != tc.loginCalls {
				t.Fatalf("login attempts=%d, expected=%d", count, tc.loginCalls)
			}
		})
	}
}

// TestServerChangeRechecksConfiguredCredentials verifies the EU failure notice.
func TestServerChangeRechecksConfiguredCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api-key.env")
	if err := os.WriteFile(path, []byte(
		"BW_CLIENTID=user.test\nBW_CLIENTSECRET=test-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &serverFlowRunner{}
	cli, err := bitwarden.NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(config.Config{
		Directory: dir,
		Authentication: config.Authentication{
			ClientEnvPath: path, DefaultServer: "us",
		},
	}, cli)
	app := New(state)
	app.startupWithUpdates(context.Background(), applyStartupUpdateNow, true)
	if state.Screen != AuthMaster || state.Notice != "" {
		t.Fatalf("US login should succeed: screen=%v notice=%q", state.Screen, state.Notice)
	}
	state.Mode = DialogServer
	state.ServerReady = [2]bool{true, true}
	state.ServerChoice = 1
	if err := app.enter(); err != nil || state.Server != "eu" ||
		state.Screen != AuthChecking {
		t.Fatal("switch to EU should enter the checking screen")
	}
	// In production enter() schedules this work on gocui's event loop.
	app.startupWithUpdates(context.Background(), applyStartupUpdateNow, true)
	if state.Screen != AuthClient || state.Busy ||
		!strings.Contains(state.Notice, "Bitwarden rejected") {
		t.Fatalf("EU failure should be visible: screen=%v notice=%q",
			state.Screen, state.Notice)
	}
	found := false
	for _, call := range runner.calls {
		if call == "config server https://vault.bitwarden.eu" {
			found = true
		}
	}
	if !found {
		t.Fatal("EU server was not configured before login")
	}
}

// TestClientIdUpArrowReturnsToAuthButton checks reverse auth navigation.
func TestClientIdUpArrowReturnsToAuthButton(t *testing.T) {
	state := setup(t)
	state.Screen = AuthClient
	app := New(state)
	state.AuthFocus = 2
	_ = app.arrow(1, 0)
	if state.AuthFocus != 0 {
		t.Fatal("Down should move from AUTH to Client ID")
	}
	_ = app.arrow(-1, 0)
	if state.AuthFocus != 2 {
		t.Fatal("Up should move from Client ID to AUTH")
	}
	state.AuthFocus = 1
	_ = app.arrow(-1, 0)
	if state.AuthFocus != 0 {
		t.Fatal("Up should move from Client secret to Client ID")
	}
}

// TestAuthHintHasNoFinalPeriod checks the first auth help panel.
// TestAuthHintText checks the exact first-screen authentication hint.
func TestAuthHintText(t *testing.T) {
	state := setup(t)
	state.Screen = AuthClient
	canvas := NewCanvas(120, 40)
	New(state).authRender(canvas)
	g := authGeometry(canvas.W, canvas.H, AuthClient)
	const expected = "Hint. Sign in automatically configuring API credentials in settings.json"
	row := stringCellRow(canvas, g.tipY+1)
	if !strings.Contains(row, expected) {
		t.Fatalf("missing expected authentication hint: %q", row)
	}
	if strings.Contains(row, "settings.jsonc") || strings.Contains(row, "settings.json.") {
		t.Fatalf("incorrect auth hint suffix: %q", row)
	}
}
