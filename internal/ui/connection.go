package ui

import (
	"context"
	"errors"
	"time"

	"github.com/awesome-gocui/gocui"
	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/network"
)

const (
	connectionLossCheckInterval     = 5 * time.Second
	connectionRecoveryCheckInterval = 2500 * time.Millisecond
	connectionLossThreshold         = 5 * time.Second
)

// monitorConnection periodically probes the active region without invoking bw.
func (a *App) monitorConnection(ctx context.Context) {
	ticker := time.NewTicker(connectionRecoveryCheckInterval)
	var nextLossCheck time.Time
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.gui == nil {
				continue
			}
			a.gui.Update(func(*gocui.Gui) error {
				s := a.S
				if (s.Screen != Workspace && s.Screen != AuthClient && s.Screen != AuthMaster) || s.LocalMode || a.exitRequested.Load() ||
					!a.networkChecking.CompareAndSwap(false, true) {
					return nil
				}
				if s.ServerConnected && time.Now().Before(nextLossCheck) {
					a.networkChecking.Store(false)
					return nil
				}
				if s.ServerConnected {
					nextLossCheck = time.Now().Add(connectionLossCheckInterval)
				}
				region := s.Server
				url := s.Config.Network.USPingURL
				if region == "eu" {
					url = s.Config.Network.EUPingURL
				}
				go a.probeConnection(ctx, region, url)
				return nil
			})
		}
	}
}

// monitorServe watches only the private process started by this LazyBW
// instance. It deliberately does not mistake remote Internet failures for a
// process crash: those remain handled by monitorConnection.
func (a *App) monitorServe(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.gui == nil {
				continue
			}
			a.gui.Update(func(*gocui.Gui) error {
				s := a.S
				if s.Screen != Workspace || s.LocalMode || s.BackendFatal || s.CLI.BackendMode() != bitwarden.BackendBWServe ||
					s.CLI.ServeRunning() || !a.serveRecoveryActive.CompareAndSwap(false, true) {
					return nil
				}
				s.BackendRecovering = true
				s.ServerConnected = false
				s.SetWorkspaceNotice("Server connexion lost", CRed)
				go a.recoverServe(ctx)
				return nil
			})
		}
	}
}

func (a *App) recoverServe(ctx context.Context) {
	defer a.serveRecoveryActive.Store(false)
	for {
		attempt, cancel := context.WithTimeout(ctx, 55*time.Second)
		err := a.S.CLI.RecoverServe(attempt)
		cancel()
		if ctx.Err() != nil || a.gui == nil {
			return
		}
		if err == nil {
			a.gui.Update(func(*gocui.Gui) error {
				if a.S.Screen == Workspace {
					a.S.BackendRecovering = false
					a.S.ServerConnected = true
					a.S.ClearWorkspaceNotice()
				}
				return nil
			})
			return
		}
		var cliErr *bitwarden.CLIError
		if errors.As(err, &cliErr) && cliErr.Category == "authentication" {
			a.gui.Update(func(*gocui.Gui) error {
				// A conclusively invalid token cannot be repaired without retaining
				// the master password. Stop retries and preserve the existing fatal
				// shutdown behavior instead of issuing another vault operation.
				a.S.BackendRecovering = false
				a.S.BackendFatal = true
				a.S.Popup = &Popup{Title: "Bitwarden session expired", Question: "QUIT", Acknowledge: true, AcknowledgeText: "QUIT", AcknowledgeAction: a.beginWorkspaceShutdown}
				return nil
			})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// probeConnection runs outside the event loop; its result is applied on the UI thread.
func (a *App) probeConnection(ctx context.Context, region, url string) {
	defer a.networkChecking.Store(false)
	probeCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	check := a.endpointChecker
	if check == nil {
		check = network.Reachable
	}
	checkedAt := time.Now()
	available := check(probeCtx, url)
	if ctx.Err() != nil || a.gui == nil {
		return
	}
	a.gui.Update(func(*gocui.Gui) error {
		a.applyConnectionResultAt(region, available, checkedAt)
		return nil
	})
}

// applyConnectionResult applies one observed probe to the current region.
func (a *App) applyConnectionResult(region string, available bool) {
	a.applyConnectionResultAt(region, available, time.Now())
}

// applyConnectionResultAt only reports an outage after 5 seconds of
// consecutive failed observations. A success restores availability immediately.
func (a *App) applyConnectionResultAt(region string, available bool, checkedAt time.Time) {
	s := a.S
	if (s.Screen != Workspace && s.Screen != AuthClient && s.Screen != AuthMaster) || s.LocalMode || s.Server != region || s.BackendRecovering {
		return
	}
	if available {
		a.connectionFailureSince = time.Time{}
		a.connectionFailureRegion = ""
		s.ServerConnected = true
		if s.Notice == "Server connection lost" {
			s.Notice = ""
		}
		return
	}
	if a.connectionFailureRegion != region || a.connectionFailureSince.IsZero() {
		a.connectionFailureSince = checkedAt
		a.connectionFailureRegion = region
		return
	}
	if checkedAt.Sub(a.connectionFailureSince) >= connectionLossThreshold {
		wasConnected := s.ServerConnected
		s.ServerConnected = false
		if s.Screen == AuthClient || s.Screen == AuthMaster {
			s.Notice = "Server connection lost"
			// Change focus only when first entering the offline form.
			// Subsequent HTTP probes must preserve the user's button selection.
			if wasConnected {
				s.AuthFocus = 2
			}
		}
	}
}
