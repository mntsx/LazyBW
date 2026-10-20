package ui

import (
	"testing"
	"time"
)

// TestAllVaultsDefault verifies that shared entries are visible initially.
func TestAllVaultsDefault(t *testing.T) {
	s := setup(t)
	if s.FilterVault != 1 || s.Filters[0] != 1 {
		t.Fatalf("vault filter defaults: applied=%d selected=%d", s.FilterVault, s.Filters[0])
	}
}

func TestConnectionIntervals(t *testing.T) {
	if connectionLossCheckInterval != 5*time.Second || connectionLossThreshold != 5*time.Second {
		t.Fatalf("connection loss intervals = %v and %v, want 5s", connectionLossCheckInterval, connectionLossThreshold)
	}
	if connectionRecoveryCheckInterval != 2500*time.Millisecond {
		t.Fatalf("connection recovery interval = %v, want 2.5s", connectionRecoveryCheckInterval)
	}
}

// TestAuthenticationConnectionLoss verifies the monitor on both login screens.
func TestAuthenticationConnectionLoss(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		s := setup(t)
		s.Screen = screen
		s.Server = "us"
		s.ServerConnected = true
		a := New(s)
		start := time.Now()
		a.applyConnectionResultAt("us", false, start)
		if !s.ServerConnected {
			t.Fatal("connection lost before threshold")
		}
		a.applyConnectionResultAt("us", false, start.Add(connectionLossThreshold))
		if s.ServerConnected || s.Notice != "Server connection lost" || s.AuthFocus != 2 {
			t.Fatalf("failed to enter offline auth mode: screen=%d notice=%q", screen, s.Notice)
		}
		a.applyConnectionResultAt("us", true, start.Add(3*time.Second))
		if !s.ServerConnected || s.Notice != "" {
			t.Fatal("failed to restore online authentication")
		}
	}
}

// TestAuthOfflineButtons verifies the asymmetric button pair is centered.
func TestAuthOfflineButtons(t *testing.T) {
	for _, width := range []int{80, 81, 120, 121} {
		leftX, leftWidth, rightX, rightWidth := authOfflineButtons(width)
		if leftX < 0 || rightX+rightWidth > width || leftX != width-rightX-rightWidth {
			t.Fatalf("incorrect centering for terminal width %d", width)
		}
		if rightX-leftX-leftWidth != 4-width%2 {
			t.Fatalf("unexpected spacing for terminal width %d", width)
		}
	}
}
