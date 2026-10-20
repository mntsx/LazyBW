package ui

import (
	"context"
	"time"

	"github.com/awesome-gocui/gocui"
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
	if (s.Screen != Workspace && s.Screen != AuthClient && s.Screen != AuthMaster) || s.LocalMode || s.Server != region {
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
