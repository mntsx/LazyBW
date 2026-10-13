package bitwarden

import (
	"context"
	"testing"
)

type closingRunner struct {
	commands [][]string
}

func (r *closingRunner) Run(
	_ context.Context, args []string, _ []byte, _ []string,
) ([]byte, []byte, int, error) {
	r.commands = append(r.commands, append([]string(nil), args...))
	return nil, nil, 0, nil
}

// TestCloseSkipsLockWithoutSession avoids an unnecessary Bitwarden subprocess.
func TestCloseSkipsLockWithoutSession(t *testing.T) {
	runner := &closingRunner{}
	client, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	client.Close(context.Background())
	if len(runner.commands) != 0 {
		t.Fatalf("closing without a session called bw: %v", runner.commands)
	}
}

// TestCloseLocksActiveSession still revokes an unlocked vault on exit.
func TestCloseLocksActiveSession(t *testing.T) {
	runner := &closingRunner{}
	client, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	client.SetSession("test-session")
	client.Close(context.Background())
	if len(runner.commands) != 1 || len(runner.commands[0]) != 1 ||
		runner.commands[0][0] != "lock" {
		t.Fatalf("active session was not locked: %v", runner.commands)
	}
	if client.HasSession() {
		t.Fatal("session token was not cleared")
	}
}
