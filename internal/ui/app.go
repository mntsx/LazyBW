package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/awesome-gocui/gocui"
	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/localvault"
	"github.com/miguelmnts/lazybw-go/internal/terminalkeys"
	"github.com/miguelmnts/lazybw-go/internal/terminalsecurity"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

type App struct {
	busyFlag, backupActive, noticeActive      atomic.Bool
	authReconnectActive                       atomic.Bool
	feedbackActive, exitRequested             atomic.Bool
	jobsPending                               atomic.Int32
	quitDiscardConfirmed                      atomic.Bool
	networkChecking                           atomic.Bool
	S                                         *State
	gui                                       *gocui.Gui
	cancel                                    context.CancelFunc
	closed                                    chan struct{}
	dialogY, dialogX, dialogW, dialogH        int
	sideY                                     [4]int
	sideH                                     [4]int
	leftWidth                                 int
	itemScroll, contentScroll                 int
	sideScroll                                [4]int
	itemX, itemW, contentX, contentW, detailX int
	renderWidth, renderHeight                 int
	detailY, detailH                          int
	terminalBS                                byte
	nativeWordKeys                            []terminalkeys.Binding
	submitFlash                               submitFlashTarget
	backupJobs                                chan struct{}
	backupMu                                  sync.Mutex
	backupPending                             *backupJob
	registrationInProgress                    bool
	backupDue                                 time.Time
	endpointChecker                           func(context.Context, string) bool
	connectionFailureSince                    time.Time
	connectionFailureRegion                   string
	pickerCancel                              context.CancelFunc
	initialBackupPending                      bool
	// Shutdown is the final queued operation, after remote and local writes.
	shutdownRequested atomic.Bool
	closingSession    atomic.Bool
	mainLoopRunning   atomic.Bool
}

func New(s *State) *App { return &App{S: s, closed: make(chan struct{})} }

// Run owns terminal initialization and all direct gocui operations.
func (a *App) Run() error {
	a.nativeWordKeys = terminalkeys.Register()
	g, err := gocui.NewGui(gocui.OutputTrue, true)
	if err != nil {
		return err
	}
	// tcell has already put the terminal in raw mode (no canonical buffering,
	// echo, or signal-generating control characters). Reject later opens of the
	// same TTY while the UI is active as an additional best-effort barrier.
	releaseTTY := terminalsecurity.ProtectTTY()
	// DECSCUSR: blinking bar cursor on terminals that support it (Alacritty).
	_, _ = os.Stdout.Write([]byte("\x1b[5 q"))
	var terminalCleanupOnce sync.Once
	terminalCleanup := func() {
		terminalCleanupOnce.Do(func() {
			g.Close()
			_, _ = os.Stdout.Write([]byte("\x1b[0 q"))
			releaseTTY()
		})
	}
	stopSignals := terminalsecurity.RestoreOnTermination(terminalCleanup)
	defer stopSignals()
	defer terminalCleanup()
	a.gui = g
	if err := a.S.ProtectMaster(); err != nil {
		return err
	}
	defer func() {
		// Restore terminal state before the potentially slow bw lock cleanup.
		terminalCleanup()
		a.S.ClearMaster()
		if a.S.LocalSession != nil {
			a.S.LocalSession.Close()
		}
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelClose()
		a.S.CLI.Close(closeCtx)
	}()
	g.Mouse = true
	g.Cursor = false
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	defer cancel()
	g.SetManagerFunc(a.layout)
	if err := a.bind(g); err != nil {
		return err
	}
	a.backupJobs = make(chan struct{}, 1)
	go a.backupWorker(ctx)
	go a.checkOfflineStartup(ctx)
	go a.monitorConnection(ctx)
	go func() {
		tick := time.NewTicker(110 * time.Millisecond)
		ticks := 0
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				ticks++
				if ticks%5 != 0 && !a.busyFlag.Load() &&
					!a.authReconnectActive.Load() &&
					!a.backupActive.Load() && !a.noticeActive.Load() &&
					!a.feedbackActive.Load() {
					continue
				}
				g.Update(func(*gocui.Gui) error {
					if a.feedbackActive.Load() &&
						time.Now().After(a.S.KeyFeedbackExpires) {
						a.feedbackActive.Store(false)
					}
					if !a.S.NoticeUntil.IsZero() && time.Now().After(a.S.NoticeUntil) {
						a.S.Notice = ""
						a.S.NoticeUntil = time.Time{}
					}
					a.S.Spinner++
					if ticks%5 == 0 {
						a.S.Blink = !a.S.Blink
					}
					return nil
				})
			}
		}
	}()
	a.mainLoopRunning.Store(true)
	err = g.MainLoop()
	a.mainLoopRunning.Store(false)
	if errors.Is(err, gocui.ErrQuit) {
		return nil
	}
	return err
}

// startup queues state changes on gocui's event loop in the running application.
func (a *App) startup(ctx context.Context, g *gocui.Gui, autoLogin bool) {
	a.startupWithUpdates(ctx, g.Update, autoLogin)
}

// startupWithUpdates also supports a synchronous updater in unit tests.
// The production updater is gocui.Update, which queues callbacks asynchronously.
func (a *App) startupWithUpdates(
	ctx context.Context,
	update func(func(*gocui.Gui) error),
	autoLogin bool,
) {
	s := a.S
	// The neutral checking screen is the only visible screen until the CLI
	// has determined whether an existing session or API credentials work.
	update(func(*gocui.Gui) error {
		s.Screen = AuthChecking
		s.Busy = true
		s.BusyMessage = "Checking Bitwarden session"
		s.Notice = ""
		return nil
	})
	if err := s.CLI.ConfigureRegion(ctx, s.Config.Directory, s.Server); err != nil {
		update(func(*gocui.Gui) error {
			s.Screen = AuthClient
			s.Busy = false
			s.Notice = bitwarden.AuthenticationMessage(err, false)
			return nil
		})
		return
	}
	version := s.CLI.Version(ctx)
	update(func(*gocui.Gui) error { s.Version = version; return nil })
	status, err := s.CLI.Status(ctx)
	if err != nil {
		update(func(*gocui.Gui) error {
			s.Screen = AuthClient
			s.Busy = false
			s.Notice = bitwarden.AuthenticationMessage(err, false)
			return nil
		})
		return
	}
	state := fmt.Sprint(status["status"])
	if state == "unlocked" {
		// A shell BW_SESSION belongs to an unknown profile: never reuse it.
		state = "locked"
	}
	if state == "locked" {
		update(func(*gocui.Gui) error {
			s.Screen = AuthMaster
			s.LocalWarning = a.localWarning()
			s.AuthFocus = 0
			s.Caret = len([]rune(s.MasterText()))
			s.Busy = false
			return nil
		})
		return
	}
	if path := s.Config.Authentication.ClientEnvPath; autoLogin && path != "" {
		id, secret, readErr := bitwarden.ReadEnvCredentials(path)
		switch {
		case readErr != nil:
			// Keep filesystem diagnostics generic: never expose credentials or paths.
			update(func(*gocui.Gui) error {
				s.Notice = "Cannot read client_env_path. Check the configured file"
				return nil
			})
		case id == "" || secret == "":
			update(func(*gocui.Gui) error {
				s.Notice = "client_env_path lacks a client ID or client secret"
				return nil
			})
		default:
			update(func(*gocui.Gui) error {
				s.BusyMessage = "Signing in with API credentials"
				return nil
			})
			loginErr := s.CLI.Login(ctx, id, secret)
			if loginErr == nil {
				update(func(*gocui.Gui) error {
					s.Screen = AuthMaster
					s.LocalWarning = a.localWarning()
					s.AuthFocus = 0
					s.Caret = len([]rune(s.MasterText()))
					s.Busy = false
					return nil
				})
				return
			}
			// Do not display secrets or raw CLI responses in the UI.
			update(func(*gocui.Gui) error {
				s.Notice = bitwarden.AuthenticationMessage(loginErr, false)
				return nil
			})
		}
	}
	update(func(*gocui.Gui) error { s.Screen = AuthClient; s.Busy = false; return nil })
}
func (a *App) loadVault(ctx context.Context, g *gocui.Gui) {
	g.Update(func(*gocui.Gui) error { a.S.Busy = true; a.S.BusyMessage = "Loading vault"; return nil })
	items, folders, _, err := a.S.CLI.Snapshot(ctx)
	g.Update(func(*gocui.Gui) error {
		a.S.Busy = false
		if err != nil {
			// If an existing session failed on startup, the neutral loading
			// screen must not be left visible with no action available.
			if a.S.Screen == AuthChecking {
				a.S.Screen = AuthMaster
				a.S.AuthFocus = 0
			}
			if a.S.Screen == Workspace {
				a.S.SetWorkspaceNotice("Vault loading failed", CRed)
			} else {
				a.S.Notice = err.Error()
			}
		} else {
			a.S.Loaded(items, folders)
			a.S.ClearWorkspaceNotice()
			a.initialBackupPending = a.backupAllowed()
		}
		return nil
	})
}
func (a *App) layout(g *gocui.Gui) error {
	w, h := g.Size()
	if w < 8 || h < 8 {
		return nil
	}
	// gocui reserves a one-cell inner margin. A frameless view with
	// coordinates [-1, -1, width, height] has exactly width × height
	// writable cells; mouse coordinates then match the canvas directly.
	v, err := g.SetView("canvas", -1, -1, w, h, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}
	if errors.Is(err, gocui.ErrUnknownView) {
		v.Frame = false
		v.Editable = true
		v.Wrap = false
		v.Editor = gocui.EditorFunc(a.editEvent)
		_, _ = g.SetCurrentView("canvas")
	}
	a.busyFlag.Store(a.S.Busy)
	a.authReconnectActive.Store((a.S.Screen == AuthClient ||
		a.S.Screen == AuthMaster) && !a.S.LocalMode && !a.S.ServerConnected)
	a.backupActive.Store(a.S.BackupBusy)
	a.noticeActive.Store(!a.S.NoticeUntil.IsZero())
	c := NewCanvas(w, h)
	c.Theme(a.S.Config.Colors)
	if a.S.QuitWhenSaved && a.S.Popup != nil && a.S.Popup.Blocking {
		drawMainBorders(c, nil, nil, -1)
		a.popupRender(c)
		v.Clear()
		_, _ = fmt.Fprint(v, c.Render())
		g.Cursor = false
		return nil
	}
	// Never render protected content unless this opening was verified.
	if a.S.Screen == Workspace && a.S.Mode != DialogReprompt &&
		a.S.Focus >= PaneContent && a.S.SelectedID != "" {
		if it := a.S.SelectedItem(); it != nil &&
			(it.Raw["reprompt"] == 1 || it.Raw["reprompt"] == float64(1)) &&
			a.S.RepromptApprovedID != it.ID() {
			a.startReprompt(it.ID())
		}
	}
	switch a.S.Screen {
	case AuthChecking:
		a.startupRender(c)
	case AuthOffline:
		a.offlineRender(c)
	case AuthClient, AuthMaster:
		a.authRender(c)
	case Workspace:
		a.mainRender(c)
	}
	if a.S.Popup != nil {
		a.popupRender(c)
	}
	if a.S.Mode == DialogServer &&
		(a.S.Screen == AuthClient || a.S.Screen == AuthMaster || a.S.Screen == AuthOffline) {
		a.serverPickerRender(c)
	}
	if a.S.Popup == nil && a.S.Screen == Workspace && a.S.Mode >= DialogCreate && a.S.Mode <= DialogReprompt {
		a.createRender(c)
	}
	v.Clear()
	_, _ = fmt.Fprint(v, c.Render())
	if a.initialBackupPending && a.S.Screen == Workspace {
		a.initialBackupPending = false
		go func() {
			// Allow the first workspace frame to reach the terminal before
			// starting encryption and displaying the backup spinner.
			time.Sleep(40 * time.Millisecond)
			g.Update(func(*gocui.Gui) error {
				if !a.exitRequested.Load() && !a.shutdownRequested.Load() {
					a.queueBackup()
				}
				return nil
			})
		}()
	}
	cx, cy, visible := a.editCaretPoint(w, h)
	g.Cursor = visible && cx >= 0 && cx < w && cy >= 0 && cy < h
	if g.Cursor {
		_ = v.SetCursorUnrestricted(cx, cy)
	}
	return nil
}

// authLayout holds the same geometry for drawing and mouse hit-testing.
// The full-width footer and optional API hint are anchored near the bottom;
// the form itself stays centered independently of terminal height.
type authLayout struct {
	fieldX, fieldWidth                  int
	firstY, secondY                     int
	enterX, quitX, buttonY, buttonWidth int
	footerY, tipY                       int
}

func authGeometry(w, h int, screen Screen) authLayout {
	fieldWidth := min(54, max(12, w-8))
	buttonWidth := 12
	// Keep the button pair *exactly* symmetric, even on terminals with an
	// odd column count. The only variable is the 3/4-column center gap.
	gap := 4
	if w%2 != 0 {
		gap = 3
	}
	buttonsX := (w - (2*buttonWidth + gap)) / 2
	spacing := 4
	if screen == AuthClient && h < 26 {
		spacing = 3
	}
	formHeight := 7
	if screen == AuthClient {
		formHeight = 3 + 2*spacing
	}
	top := max(3, (h-formHeight)/2)
	first := top
	second := first + spacing
	buttonY := first + 4
	if screen == AuthClient {
		buttonY = second + spacing
	}
	// Reserve the three-line API tip and the footer at the bottom.
	footerY := max(0, h-3)
	tipY := footerY - 5
	// Keep room for a three-row spinner between the buttons and the API tip
	// even in an ordinary 25–30-row terminal. Do not center the form so low
	// that the spinner obscures the credential fields or the help panel.
	if screen == AuthClient && buttonY+4 > tipY-3 {
		first = max(2, tipY-7-2*spacing)
		second = first + spacing
		buttonY = second + spacing
	}
	// Move Client ID one row closer to Client Secret, without moving Client
	// Secret or the buttons. The heading follows firstY so the heading-to-input
	// spacing remains unchanged.
	if screen == AuthClient {
		first++
	}
	return authLayout{
		fieldX: (w - fieldWidth) / 2, fieldWidth: fieldWidth,
		firstY: first, secondY: second, enterX: buttonsX,
		quitX: w - buttonsX - buttonWidth, buttonY: buttonY,
		buttonWidth: buttonWidth, footerY: footerY, tipY: tipY,
	}
}

// loadingPanel renders the identical informational spinner used by startup
// checks and by interactive authentication. No credential forms are drawn
// during the initial, still-undetermined startup state.
func (a *App) loadingPanel(c *Canvas, message string) {
	a.loadingPanelAt(c, message, c.H/2-1)
}

// loadingPanelAt is shared by the neutral startup screen and both credential
// forms. Credential forms place it below the buttons, not over the inputs.
func (a *App) loadingPanelAt(c *Canvas, message string, y int) {
	if message == "" {
		message = "Checking credentials"
	}
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	width := min(c.W-2, max(32, len([]rune(message))+9))
	if width < 8 {
		return
	}
	x := (c.W - width) / 2
	y = max(0, min(c.H-3, y))
	c.Fill(x, y, width, 3, ' ', CBlue, CBlack)
	textWidth := len([]rune(message)) + 3
	start := x + (width-textWidth)/2
	c.Text(start, y+1, string(frames[a.S.Spinner%len(frames)]), CBlue, CBlack)
	c.Text(start+2, y+1, minWidth(message, width-6), CBlue, CBlack)
}

// authHints renders key combinations and descriptions with deliberately
// different grayscale intensities while keeping the entire row centered.
func authHints(c *Canvas, y int, hint string) {
	groups := strings.Split(hint, "    ")
	x := max(0, (c.W-utf8.RuneCountInString(hint))/2)
	for i, group := range groups {
		parts := strings.SplitN(group, " ", 2)
		c.Text(x, y, parts[0], CHintKey, CDefault)
		x += utf8.RuneCountInString(parts[0])
		if len(parts) == 2 {
			x++
			c.Text(x, y, parts[1], CHintAction, CDefault)
			x += utf8.RuneCountInString(parts[1])
		}
		if i+1 < len(groups) {
			x += 4
		}
	}
}

// serverPickerGeometry matches the centered list used for creating items.
func serverPickerGeometry(w, h int) (int, int, int, int) {
	width := min(56, w-6)
	height := min(10, h-4)
	return (w - width) / 2, max(1, (h-height)/2), width, height
}

// serverPickerRender only enables endpoints whose HTTPS checks completed.
func (a *App) serverPickerRender(c *Canvas) {
	x, y, w, h := serverPickerGeometry(c.W, c.H)
	c.Fill(x, y, w, h, ' ', CWhite, CDefault)
	drawDialogBorder(c, x, y, w, h)
	c.Text(x+2, y+1, "Select Bitwarden server", CBBlue, CDefault)
	for i, label := range []string{"United States (US)", "European Union (EU)", "Local Vault (READ ONLY)"} {
		if i == 2 && !localvault.Available(a.S.Config.Local.VaultPath) {
			continue
		}
		row := y + 3 + i
		if row >= y+h-2 {
			continue
		}
		bg := CDefault
		if i == a.S.ServerChoice {
			bg = CBlack
		}
		fg, icon := CWhite, CBlue
		if i == 2 {
			icon = CBlue
		} else if !a.S.ServerReady[i] {
			icon = CRed
			fg = CBBlack
		}
		c.Fill(x+3, row, w-6, 1, ' ', fg, bg)
		glyph := websiteFieldOption.Icon
		if i == 2 {
			glyph = "\U000F0379"
		}
		c.Text(x+4, row, glyph, icon, bg)
		c.Text(x+6, row, label, fg, bg)
		if i == 2 {
			c.Text(x+6+utf8.RuneCountInString("Local Vault "), row, "(READ ONLY)", CRed, bg)
		}
	}
	authHints(c, y+h-2, joinConfiguredHints(
		a.configuredMoveHint("server_picker", false),
		a.configuredHint("server_picker", "confirm", "select"),
		a.configuredHint("server_picker", "cancel", "cancel")))
}

func (a *App) startupRender(c *Canvas) {
	a.loadingPanel(c, a.S.BusyMessage)
	drawMainBorders(c, nil, nil, -1)
}

// authOfflineButtons centers the pair while preserving the original gap.
func authOfflineButtons(w int) (int, int, int, int) {
	leftWidth := symmetricButtonWidth("ACCESS LOCAL VAULT", 20)
	rightWidth := 12
	gap := 4
	if w%2 != 0 {
		gap = 3
	}
	leftX := (w - leftWidth - rightWidth - gap) / 2
	return leftX, leftWidth, leftX + leftWidth + gap, rightWidth
}

func (a *App) authRender(c *Canvas) {
	s := a.S
	g := authGeometry(c.W, c.H, s.Screen)
	connectionLost := !s.LocalMode && !s.ServerConnected
	title := "Bitwarden API credentials"
	if s.Screen == AuthMaster {
		title = "Unlock Bitwarden"
		if s.LocalMode {
			title = "Unlock local vault"
		}
	}
	// Show which isolated account region authentication will use.
	iconColor := CBlue
	if connectionLost {
		iconColor = CRed
	}
	serverIcon := websiteFieldOption.Icon
	if s.LocalMode {
		serverIcon = "\U000F0379"
	}
	c.Text(2, 1, serverIcon, iconColor, CDefault)
	serverDomain := "bitwarden.com"
	if s.Server == "eu" {
		serverDomain = "bitwarden.eu"
	}
	label := strings.ToUpper(s.Server) + " (" + serverDomain + ")"
	cryptoLabel := "cipher: AES-256-GCM / " + s.CLI.Cipher.Backend()
	cryptoX := c.W - 2 - utf8.RuneCountInString(cryptoLabel)
	if s.LocalMode {
		path := localvault.FilePath(s.Config.Local.VaultPath)
		alg := "unknown"
		if h, err := localvault.Inspect(s.Config.Local.VaultPath); err == nil {
			alg = h.Algorithm
		}
		full := "Local (" + path + ") " + alg
		if 4+utf8.RuneCountInString(full)+2 < cryptoX {
			c.Text(4, 1, full, CWhite, CDefault)
		} else {
			c.Text(4, 1, "Local", CWhite, CDefault)
			second := "(" + path + ") " + alg
			if 4+utf8.RuneCountInString(second) < c.W-2 {
				c.Text(4, 2, second, CWhite, CDefault)
			} else {
				c.Text(4, 2, minWidth("("+path+")", c.W-6), CWhite, CDefault)
				c.Text(4, 3, alg, CWhite, CDefault)
			}
		}
	} else {
		c.Text(4, 1, label, CWhite, CDefault)
	}
	if cryptoX > 4+len("Local")+2 && (s.LocalMode || cryptoX > 4+len(label)+2) {
		c.Text(cryptoX, 1, cryptoLabel, CWhite, CDefault)
	}

	// Anchor the informational heading to the upper input, not the screen.
	// It starts at the left edge of the upper field on either auth form.
	title = minWidth(title, g.fieldWidth)
	if !connectionLost {
		c.Text(g.fieldX, max(1, g.firstY-2), title, CBlue, CDefault)
	}

	if connectionLost {
		// The unavailable endpoint must never receive credentials.
	} else if s.Screen == AuthClient {
		a.authField(c, g.fieldX, g.firstY, g.fieldWidth,
			"Client id", s.AuthID, s.AuthFocus == 0, false)
		a.authField(c, g.fieldX, g.secondY, g.fieldWidth,
			"Client secret", s.AuthSecret, s.AuthFocus == 1, !s.AuthVisible)
	} else {
		a.authField(c, g.fieldX, g.firstY, g.fieldWidth,
			"Master password", s.MasterText(), s.AuthFocus == 0, !s.AuthVisible)
	}
	mx, my := -1, -1
	if a.gui != nil {
		mx, my = a.gui.MousePosition()
	}
	if connectionLost {
		leftX, leftW, rightX, rightW := authOfflineButtons(c.W)
		hovered := func(x, width int) bool {
			return mx >= x && mx < x+width && my >= g.buttonY-1 && my <= g.buttonY+1
		}
		a.authButton(c, leftX, g.buttonY, leftW, "ACCESS LOCAL VAULT", CBlue,
			s.AuthFocus == 2, hovered(leftX, leftW))
		a.authButton(c, rightX, g.buttonY, rightW, "QUIT", CRed,
			s.AuthFocus == 3, hovered(rightX, rightW))
	} else {
		hovered := func(x int) bool {
			return mx >= x && mx < x+g.buttonWidth && my >= g.buttonY-1 && my <= g.buttonY+1
		}
		a.authButton(c, g.enterX, g.buttonY, g.buttonWidth, "AUTH", CGreen,
			s.AuthFocus == 2 || a.submitFlash == submitFlashAuth,
			hovered(g.enterX) && a.submitFlash != submitFlashAuth)
		a.authButton(c, g.quitX, g.buttonY, g.buttonWidth, "QUIT", CRed,
			s.AuthFocus == 3, hovered(g.quitX))
	}

	section, revealAction := "authentication", "reveal_secret"
	if s.Screen == AuthMaster {
		section, revealAction = "master_password", "reveal_password"
	}
	hints := joinConfiguredHints(
		a.configuredMoveHint(section, s.AuthFocus >= 2),
		a.configuredHint(section, "submit", "select"),
		a.configuredHint(section, revealAction, "reveal"),
		a.configuredHint(section, "server", "server"))

	if !a.textEntryActive() && !s.KeyFeedbackExpires.IsZero() &&
		time.Now().Before(s.KeyFeedbackExpires) {
		c.Text(1, g.footerY, s.KeyFeedback, CHintKey, CDefault)
		color := CHintAction
		if s.KeyFeedbackAction == "unbound" {
			color = CRed
		}
		c.Text(2+utf8.RuneCountInString(s.KeyFeedback), g.footerY,
			s.KeyFeedbackAction, color, CDefault)
	} else {
		authHints(c, g.footerY, hints)
	}

	if !connectionLost && s.Screen == AuthMaster && g.tipY >= 0 {
		if s.LocalWarning != "" {
			authInfoBox(c, g.tipY, s.LocalWarning, CRed)
		} else if !s.LocalMode && !s.Config.Local.SaveVault {
			authInfoBox(c, g.tipY,
				"Hint. Configure local replication in settings.jsonc for offline passwords",
				CWhite)
		}
	}
	if !connectionLost && s.Screen == AuthClient && g.tipY >= 0 {
		hint := "Hint. Sign in automatically configuring API credentials in settings.json"
		authInfoBox(c, g.tipY, hint, CWhite)
	}
	if s.Notice != "" {
		noticeY := min(c.H-4, g.buttonY+3)
		c.Text(max(0, (c.W-len([]rune(s.Notice)))/2), noticeY,
			minWidth(s.Notice, c.W-2), CRed, CDefault)
	}
	if s.Busy || connectionLost {
		limitY := g.footerY
		if s.Screen == AuthClient {
			limitY = g.tipY
		}
		// Place the three-line spinner in the available band between the
		// bottom of the buttons and the top of the hints/API tip.
		startY := g.buttonY + 4
		endY := limitY - 3
		spinnerY := startY
		if endY >= startY {
			spinnerY = (startY + endY) / 2
		}
		message := s.BusyMessage
		if connectionLost {
			message = "Trying to reconnect with server"
		}
		a.loadingPanelAt(c, message, spinnerY)
	}
	// Keep the frame flush against the terminal edges; the form remains
	// independent of these single-cell borders.
	drawMainBorders(c, nil, nil, -1)
}

// authInfoBox draws the shared three-row information box below authentication.
func authInfoBox(c *Canvas, y int, message string, foreground int) {
	boxWidth := min(c.W-2, utf8.RuneCountInString(message)+4)
	if boxWidth < 4 {
		return
	}
	boxX := (c.W - boxWidth) / 2
	c.Fill(boxX, y, boxWidth, 3, ' ', CWhite, CBlack)
	c.Text(boxX+max(1, (boxWidth-utf8.RuneCountInString(message))/2), y+1,
		minWidth(message, boxWidth-2), foreground, CBlack)
}

func (a *App) authField(c *Canvas, x, y, w int, hint, value string, focused, mask bool) {
	if w < 2 {
		return
	}
	text := value
	foreground := CWhite
	if mask && value != "" {
		text = strings.Repeat("•", len([]rune(value)))
	}
	if value == "" {
		text = hint
		foreground = CBlack
	} // normal.black placeholder
	if focused && value != "" {
		text, _ = visibleText(text, a.S.Caret, w)
	}
	c.Text(x, y, minWidth(text, w), foreground, CDefault)
	underline := CBBlack
	if focused {
		underline = CBlue
	}
	c.Line(x, y+1, w, '─', underline, CDefault)
	// The terminal's hardware blinking bar is positioned by layout(),
	// so no cell is overwritten by a simulated caret.
}

const (
	buttonTopCapGlyph = '▂'
	buttonAccentGlyph = '▃'
)

// buttonStyle centralizes the visual treatment shared by every button. Color
// remains per-instance because it conveys each action's meaning.
type buttonStyle struct {
	color  int
	topCap bool
}

func (style buttonStyle) draw(c *Canvas, x, y, w int, label string, selected, hover bool) {
	bg, fg := buttonColors(style.color, selected, hover)
	if style.topCap {
		// A default foreground is the terminal TEXT color, not the global
		// background. An empty cell inherits the real terminal background.
		if hover {
			c.Line(x, y-1, w, ' ', CDefault, CDefault)
		} else {
			c.Line(x, y-1, w, buttonTopCapGlyph, bg, CDefault)
		}
	}
	c.Fill(x, y, w, 1, ' ', fg, bg)
	c.Text(x+max(1, (w-len(label))/2), y, label, fg, bg)
	c.Line(x, y+1, w, buttonAccentGlyph, buttonAccent(style.color, hover), bg)
}

// authButton adds the authentication form's fractional top padding to the
// shared button design.
func (a *App) authButton(c *Canvas, x, y, w int, label string, color int, selected, hover bool) {
	buttonStyle{color: color, topCap: true}.draw(c, x, y, w, label, selected, hover)
}

// symmetricButtonWidth preserves equal whitespace on both sides of a label.
// When the requested width has the wrong parity, only the right edge shrinks.
func symmetricButtonWidth(label string, requested int) int {
	labelWidth := utf8.RuneCountInString(label)
	width := max(requested, labelWidth+2)
	if (width-labelWidth)%2 != 0 {
		width--
	}
	return width
}

// button uses the shared design without the authentication form's top cap.
func (a *App) button(c *Canvas, x, y, w int, label string, color int, selected, hover bool) {
	buttonStyle{color: color}.draw(c, x, y, symmetricButtonWidth(label, w), label, selected, hover)
}

// buttonColors derives the body palette from the button's interaction state.
func buttonColors(color int, selected, hover bool) (int, int) {
	if hover {
		return CDefault, CWhite
	}
	if selected {
		return color, CBlack
	}
	return CBlack, color
}

// buttonAccent makes hover independent of the button's original hue.
func buttonAccent(color int, hover bool) int {
	if hover {
		return CWhite
	}
	return color
}

// drawDialogBorder draws a four-sided border without touching the content.
func drawDialogBorder(c *Canvas, x, y, w, h int) {
	if w < 2 || h < 2 {
		return
	}
	c.Line(x+1, y, w-2, '━', CBlue, CDefault)
	c.Line(x+1, y+h-1, w-2, '━', CBlue, CDefault)
	for yy := y + 1; yy < y+h-1; yy++ {
		c.Put(x, yy, '┃', CBlue, CDefault)
		c.Put(x+w-1, yy, '┃', CBlue, CDefault)
	}
	c.Put(x, y, '┏', CBlue, CDefault)
	c.Put(x+w-1, y, '┓', CBlue, CDefault)
	c.Put(x, y+h-1, '┗', CBlue, CDefault)
	c.Put(x+w-1, y+h-1, '┛', CBlue, CDefault)
}

func popupConfirmationButtonBounds(p *Popup, x, width int) (yesX, yesWidth, noX, noWidth int) {
	yesLabel, noLabel := "YES", "NO"
	if p.Quit {
		yesLabel = "DISCARD AND QUIT"
		noLabel = "CANCEL"
	}
	yesX = x + 4
	yesWidth = symmetricButtonWidth(yesLabel, max(14, utf8.RuneCountInString(yesLabel)+4))
	noX = x + width - max(17, utf8.RuneCountInString(noLabel)+6)
	noWidth = symmetricButtonWidth(noLabel, max(13, utf8.RuneCountInString(noLabel)+4))
	return
}

func (a *App) popupRender(c *Canvas) {
	p := a.S.Popup
	if p != nil && p.RemoteReview != nil {
		a.remoteReviewRender(c)
		return
	}
	if p == nil {
		return
	}
	w := min(max(50, c.W*2/3), c.W-4)
	contentH := min(10, len(p.Lines))
	height := max(11, 9+contentH)
	if contentH > 1 {
		height++ // Blank row between final item and the buttons.
	}
	if len(p.Lines) > 10 {
		height += 3 // Keep blank rows around pagination and above buttons.
	}
	// The workspace footer occupies the penultimate terminal row. Keep every
	// popup above it, including paginated dialogs on the minimum screen size.
	height = min(height, max(1, c.H-3))
	x := (c.W - w) / 2
	y := (c.H - height) / 2
	a.dialogX, a.dialogY, a.dialogW, a.dialogH = x, y, w, height
	c.Fill(x, y, w, height, ' ', CWhite, CDefault)
	drawDialogBorder(c, x, y, w, height)
	c.Text(x+2, y+1, p.Title, CBBlue, CDefault)
	c.Text(x+2, y+3, minWidth(p.Question, w-4), CWhite, CDefault)
	if p.Acknowledge {
		by := y + height - 4
		bx := x + (w-10)/2
		mx, my := -1, -1
		if a.gui != nil {
			mx, my = a.gui.MousePosition()
		}
		a.button(c, bx, by, 10, "OK", CBlue, true, my >= by && my <= by+1 && mx >= bx && mx < bx+10)
		return
	}
	if p.Blocking {
		spinner := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		message, color := shutdownProgress(a.S, a.closingSession.Load())
		c.Text(x+3, y+5, fmt.Sprintf("%c %s", spinner[a.S.Spinner%len(spinner)], message), color, CDefault)
		return
	}
	start := p.Page * 10
	for k := 0; k < contentH; k++ {
		idx := start + k
		if idx >= len(p.Lines) {
			break
		}
		line := p.Lines[idx]
		printed := minWidth(line, w-6)
		// Preserve the operation marker if the item name is too long.
		if utf8.RuneCountInString(line) > w-6 && len([]rune(line)) >= 3 {
			all := []rune(line)
			marker := all[len(all)-1]
			if (marker == 'N' || marker == 'M' || marker == 'D') &&
				string(all[len(all)-3:len(all)-1]) == "  " {
				printed = minWidth(line, w-9) + "  " + string(marker)
			}
		}
		c.Text(x+3, y+5+k, printed, CWhite, CDefault)
		// Confirmation summaries end in N/M/D. Color only that marker.
		runes := []rune(printed)
		if len(runes) >= 3 && string(runes[len(runes)-3:len(runes)-1]) == "  " {
			marker := runes[len(runes)-1]
			fg := CDefault
			switch marker {
			case 'N':
				fg = CGreen
			case 'M':
				fg = CYellow
			case 'D':
				fg = CRed
			}
			if fg != CDefault {
				c.Put(x+3+len(runes)-1, y+5+k, marker, fg, CDefault)
			}
		}
	}
	if len(p.Lines) > 10 {
		c.Text(x+3, y+height-6, fmt.Sprintf("? ...   page %d/%d", p.Page+1, (len(p.Lines)+9)/10), CBBlack, CDefault)
	}
	yes := CGreen
	no := CRed
	yesLabel, noLabel := "YES", "NO"
	if p.Destructive {
		yes = CRed
		no = CGreen
	}
	if p.Quit {
		yesLabel = "DISCARD AND QUIT"
		noLabel = "CANCEL"
	}
	buttonY := y + height - 4
	mx, my := -1, -1
	if a.gui != nil {
		mx, my = a.gui.MousePosition()
	}
	yesX, yesWidth, noX, noWidth := popupConfirmationButtonBounds(p, x, w)
	a.button(c, yesX, buttonY, yesWidth, yesLabel, yes, p.Yes,
		my >= buttonY && my <= buttonY+1 && mx >= yesX && mx < yesX+yesWidth)
	a.button(c, noX, buttonY, noWidth, noLabel, no, !p.Yes,
		my >= buttonY && my <= buttonY+1 && mx >= noX && mx < noX+noWidth)
}
func (a *App) createRender(c *Canvas) {
	s := a.S
	if s.Mode == DialogReprompt {
		w := min(66, c.W-4)
		x, y := (c.W-w)/2, max(2, (c.H-12)/2)
		c.Fill(x, y, w, 12, ' ', CWhite, CDefault)
		drawDialogBorder(c, x, y, w, 12)
		c.Text(x+2, y+1, "Master password re-prompt", CBBlue, CDefault)
		message := "Verify the master password to open this item"
		if s.RepromptDeleteID != "" {
			message = "Verify the master password to delete this item"
		}
		c.Text(x+3, y+3, message, CWhite, CDefault)
		a.authField(c, x+4, y+5, w-8, "Master password", s.MasterText(), true, !s.AuthVisible)
		if s.RepromptError != "" {
			c.Text(x+4, y+8, s.RepromptError, CRed, CDefault)
		}
		authHints(c, y+10, joinConfiguredHints(
			a.configuredHint("reprompt_popup", "reveal_password", "reveal"),
			a.configuredHint("reprompt_popup", "confirm", "confirm"),
			a.configuredHint("reprompt_popup", "cancel", "cancel")))
		return
	}
	w := min(65, c.W-6)
	x := (c.W - w) / 2
	y := max(2, (c.H-16)/2)
	h := 15
	if s.Mode == DialogWebsite || s.Mode == DialogFoldersChoice ||
		s.Mode == DialogCardBrand || s.Mode == DialogIdentityTitle {
		count := 0
		switch s.Mode {
		case DialogWebsite:
			count = len(s.CurrentField().Values) + 1
		case DialogFoldersChoice:
			count = len(s.Folders)
		case DialogCardBrand:
			count = len(cardBrands)
		case DialogIdentityTitle:
			count = len(identityTitles)
		}
		h = min(max(9, count+5), c.H-4)
		y = max(2, (c.H-h)/2)
	}
	c.Fill(x, y, w, h, ' ', CWhite, CDefault)
	drawDialogBorder(c, x, y, w, h)
	c.Text(x+2, y+1, func() string {
		if s.Mode == DialogFoldersChoice {
			return "Select folder"
		}
		if s.Mode == DialogCardBrand {
			return "Card brand"
		}
		if s.Mode == DialogIdentityTitle {
			return "Identity title"
		}
		if s.Mode == DialogLinkedChoice {
			return "Link field to"
		}
		if s.Mode == DialogWebsite {
			return "Edit website"
		}
		return "Create " + func() string {
			if s.Mode == DialogFolder {
				return "folder"
			}
			if s.Mode == DialogField || s.Mode == DialogFieldType {
				return "field"
			}
			return "item"
		}()
	}(), CBBlue, CDefault)
	if s.Mode == DialogCreate || s.Mode == DialogFieldType || s.Mode == DialogWebsite ||
		s.Mode == DialogLinkedChoice || s.Mode == DialogFoldersChoice ||
		s.Mode == DialogCardBrand || s.Mode == DialogIdentityTitle {
		types := make([]string, 0)
		icons := make([]string, 0)
		for _, typeID := range vault.Schema().CreationOrder {
			types = append(types, vault.TypeNames[typeID])
			icons = append(icons, vault.TypeIcons[typeID])
		}
		types = append(types, "Cancel")
		icons = append(icons, "\U000F073A")
		selected := s.NewType
		if s.Mode == DialogLinkedChoice {
			types = nil
			icons = nil
			for _, target := range vault.Schema().LinkedTargets {
				types = append(types, target.Name)
				icons = append(icons, "\U000F0339")
			}
			selected = s.LinkedIndex
		}
		if s.Mode == DialogFoldersChoice {
			types = nil
			icons = nil
			for _, folder := range s.Folders {
				id := fmt.Sprint(folder["id"])
				mark := "[ ]"
				if s.FolderChoiceID == id {
					mark = "[x]"
				}
				types = append(types, mark+" "+fmt.Sprint(folder["name"]))
				icons = append(icons, " ")
			}
			selected = s.ChoiceIndex
		} else if s.Mode == DialogCardBrand {
			types = cardBrands
			icons = make([]string, len(types))
			selected = s.ChoiceIndex
		} else if s.Mode == DialogIdentityTitle {
			types = identityTitles
			icons = make([]string, len(types))
			selected = s.ChoiceIndex
		}
		if s.Mode == DialogFieldType {
			options := fieldOptions(s.SelectedItem())
			types = make([]string, 0, len(options)+1)
			icons = make([]string, 0, len(options)+1)
			for _, option := range options {
				types = append(types, option.Name)
				icons = append(icons, option.Icon)
			}
			types = append(types, "Cancel")
			icons = append(icons, "\U000F073A")
			selected = s.CustomType
		} else if s.Mode == DialogWebsite {
			types = append([]string(nil), s.CurrentField().Values...)
			icons = make([]string, 0, len(types)+1)
			for range types {
				icons = append(icons, websiteFieldOption.Icon)
			}
			types = append(types, "Cancel")
			icons = append(icons, "\U000F073A")
			selected = s.WebsiteIndex
		}
		first := 0
		if s.Mode == DialogWebsite || s.Mode == DialogFoldersChoice ||
			s.Mode == DialogCardBrand || s.Mode == DialogIdentityTitle {
			capacity := max(1, h-4)
			if selected >= capacity {
				first = selected - capacity + 1
			}
		}
		for idx, name := range types {
			if idx < first || idx-first >= h-4 {
				continue
			}
			ry := y + 3 + idx - first
			bg, fg := CDefault, CWhite
			if idx == selected {
				bg = CBlack
			}
			if idx == len(types)-1 && (s.Mode == DialogCreate || s.Mode == DialogFieldType || s.Mode == DialogWebsite) {
				fg = CRed
				if idx == selected {
					bg = CRed
					fg = CBlack
				}
			}
			c.Fill(x+3, ry, w-6, 1, ' ', fg, bg)
			iconFG := CBlue
			if idx == len(types)-1 && (s.Mode == DialogCreate || s.Mode == DialogFieldType || s.Mode == DialogWebsite) {
				iconFG = fg
			}
			c.Text(x+5, ry, icons[idx], iconFG, bg)
			c.Text(x+8, ry, minWidth(name, max(0, w-11)), fg, bg)
		}

	} else {
		title := "Name"
		if s.Mode == DialogField {
			title = "Field name"
		}
		a.authField(c, x+4, y+5, w-8, title, s.Input, s.FormFocus == 0, false)
		if s.FormError != "" {
			message := minWidth(s.FormError, w-8)
			messageX := x + max(1, (w-utf8.RuneCountInString(message))/2)
			c.Text(messageX, y+7, message, CRed, CDefault)
		}
		mx, my := -1, -1
		if a.gui != nil {
			mx, my = a.gui.MousePosition()
		}
		enterX := x + 9
		enterWidth := symmetricButtonWidth("SUBMIT", 16)
		a.button(c, enterX, y+10, enterWidth, "SUBMIT", CBlue,
			s.FormFocus == 1 || a.submitFlash == submitFlashDialog,
			my >= y+10 && my <= y+11 && mx >= enterX && mx < enterX+enterWidth &&
				a.submitFlash != submitFlashDialog)
		cancelX := x + w - 26
		cancelWidth := symmetricButtonWidth("CANCEL", 16)
		a.button(c, cancelX, y+10, cancelWidth, "CANCEL", CBlue, s.FormFocus == 2,
			my >= y+10 && my <= y+11 && mx >= cancelX && mx < cancelX+cancelWidth)
	}
}

// modalOpen prevents global shortcuts from modifying state behind a popup.
func (a *App) modalOpen() bool {
	if a.S.Popup != nil {
		return true
	}
	return a.S.Mode == DialogServer ||
		(a.S.Screen == Workspace && a.S.Mode >= DialogCreate && a.S.Mode <= DialogReprompt)
}

// shutdownProgress is shared by waiting, backup and session-lock dialogs.
// The text and spinner always have the same task-specific color.
func shutdownProgress(s *State, locking bool) (string, int) {
	if locking {
		return "Locking Bitwarden session", CRed
	}
	if s.BackupBusy {
		return "Encrypting local vault", CBlue
	}
	if s.Busy {
		return "Pushing edits", CGreen
	}
	return "Waiting for queued operations", CBlue
}

func (a *App) requestQuit() {
	s := a.S
	if a.shutdownRequested.Load() || a.modalOpen() {
		return
	}
	if s.Screen != Workspace {
		if !s.Busy || s.Screen == AuthChecking {
			a.close()
		}
		return
	}
	if !s.Busy && a.jobsPending.Load() == 0 && s.Stage.Len() > 0 {
		a.confirm("Quit LazyBW?", "Pending edits will be lost when you exit.",
			s.Stage.Changes(), false, true, func(ok bool) {
				if ok {
					a.quitDiscardConfirmed.Store(true)
					a.beginWorkspaceShutdown()
				}
			})
		return
	}
	a.beginWorkspaceShutdown()
}

// beginWorkspaceShutdown queues the session lock after all existing writes.
// Authentication and Local mode have no running server session to lock.
func (a *App) beginWorkspaceShutdown() {
	if a.S.LocalMode || !a.mainLoopRunning.Load() {
		a.close()
		return
	}
	if !a.shutdownRequested.CompareAndSwap(false, true) {
		return
	}
	a.S.QuitWhenSaved = true
	a.S.Popup = &Popup{
		Title: "Finishing operations", Blocking: true,
		Question: "Waiting for pending writes and session cleanup",
	}
	a.S.KeyFeedbackExpires = time.Time{}
	a.feedbackActive.Store(false)
	go a.shutdownWorker()
}

// shutdownWorker is the final queued job. It polls the UI state through
// gocui's event loop, so it never reads UI-owned fields from a worker thread.
func (a *App) shutdownWorker() {
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	for range tick.C {
		if a.exitRequested.Load() {
			return
		}
		if a.jobsPending.Load() != 0 {
			continue
		}
		ready := make(chan bool, 1)
		a.gui.Update(func(*gocui.Gui) error {
			if a.S.Busy || a.S.BackupBusy || a.jobsPending.Load() != 0 {
				ready <- false
				return nil
			}
			if a.S.Stage.Len() != 0 && !a.quitDiscardConfirmed.Load() {
				// A partially failed registration may leave unsaved changes.
				// Confirm discarding them instead of quietly losing them.
				a.shutdownRequested.Store(false)
				a.S.QuitWhenSaved = false
				a.confirm("Quit LazyBW?",
					"Pending edits will be lost when you exit.",
					a.S.Stage.Changes(), false, true, func(ok bool) {
						if ok {
							a.beginWorkspaceShutdown()
						}
					})
				ready <- false
				return nil
			}
			a.closingSession.Store(true)
			a.S.BusyMessage = "Locking Bitwarden session"
			ready <- true
			return nil
		})
		if !<-ready {
			if !a.shutdownRequested.Load() {
				return
			}
			continue
		}
		break
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	a.S.CLI.Close(ctx)
	cancel()
	a.gui.Update(func(*gocui.Gui) error {
		a.S.QuitWhenSaved = false
		a.S.Popup = nil
		a.close()
		return a.quitResult(nil)
	})
}

// close requests an immediate exit. The input callback (or save completion)
// returns ErrQuit to the event loop, without queuing another UI update.
func (a *App) close() {
	if !a.exitRequested.CompareAndSwap(false, true) {
		return
	}
	if a.cancel != nil {
		a.cancel()
	}
	// Transient input hints must never survive or postpone a quit request.
	a.S.KeyFeedbackExpires = time.Time{}
	a.feedbackActive.Store(false)
}

// quitResult propagates a confirmed quit directly to gocui's main loop.
// While a save/confirmation is still pending, the original result is kept.
func (a *App) quitResult(err error) error {
	if err != nil {
		return err
	}
	if a.exitRequested.Load() && a.gui != nil {
		return gocui.ErrQuit
	}
	return nil
}
func (a *App) confirm(title, question string, changes []*vault.Change, save, quit bool, action func(bool)) {
	p := &Popup{Title: title, Question: question, Save: save, Destructive: !save, Quit: quit, Yes: save, Action: action}
	for _, ch := range changes {
		icon := vault.IconFolder
		if ch.Entity == "item" {
			for typ, name := range vault.TypeNames {
				if name == ch.TypeName {
					icon = vault.TypeIcons[typ]
					break
				}
			}
		}
		p.Lines = append(p.Lines, fmt.Sprintf("%s %s  %s", icon, ch.Name, ch.Kind))
	}
	a.S.Popup = p
}

// showRemoteReview opens the manual synchronization review.
func (a *App) showRemoteReview(review *bitwarden.RemoteReview) {
	a.showRemoteReviewWithContext(review, false)
}

// showRemoteReviewWithContext distinguishes manual sync from blocked registration.
func (a *App) showRemoteReviewWithContext(
	review *bitwarden.RemoteReview, registrationBlocked bool,
) {
	p := &Popup{
		Title: "Remote changes detected", RemoteReview: review,
		RemoteChoice: 2, RemoteFromRegistration: registrationBlocked,
	}
	for _, change := range review.Differences {
		icon := vault.IconFolder
		if change.Entity == "item" {
			for typ, name := range vault.TypeNames {
				if name == change.TypeName {
					icon = vault.TypeIcons[typ]
					break
				}
			}
		}
		p.Lines = append(p.Lines, fmt.Sprintf("%s %s  %s", icon, change.Name, change.Kind))
	}
	p.Action = func(track bool) {
		a.S.ApplyRemoteReview(review, track)
		a.S.SetWorkspaceNotice("Remote vault synchronized", CBlue)
	}
	a.S.Popup = p
}

// remoteReviewPageSize reserves space for the explanation and three action buttons.
func remoteReviewPageSize(terminalHeight, count int) int {
	return max(1, min(10, count, max(1, terminalHeight-20)))
}

// reviewPageSize uses the active terminal size for picker navigation.
func (a *App) reviewPageSize() int {
	if a.gui == nil {
		return 10
	}
	return remoteReviewPageSize(a.height(), len(a.S.Popup.Lines))
}

// remoteReviewButton specifies the exact width and hit area of a sync action.
type remoteReviewButton struct {
	x, width int
	label    string
	color    int
}

// remoteReviewButtons keeps labels symmetrical and mouse hitboxes aligned.
func remoteReviewButtons(x, width int) [3]remoteReviewButton {
	buttonWidth, abortWidth := 13, 14
	if width < 55 {
		buttonWidth, abortWidth = 11, 12
	}
	return [3]remoteReviewButton{
		{x + 3, buttonWidth, "SOFT SYNC", CGreen},
		{x + (width-buttonWidth)/2, buttonWidth, "HARD SYNC", CBlue},
		{x + width - abortWidth - 3, abortWidth, "ABORT SYNC", CRed},
	}
}

// remoteReviewRender explains that both sync choices import the remote state.
func (a *App) remoteReviewRender(c *Canvas) {
	p := a.S.Popup
	if p == nil || p.RemoteReview == nil {
		return
	}
	w := min(max(86, c.W*3/4), c.W-4)
	if w < 20 {
		return
	}
	visible := remoteReviewPageSize(c.H, len(p.Lines))
	paginated := len(p.Lines) > visible
	pageRows := 0
	if paginated {
		pageRows = 1
	}
	// The explanation starts exactly one empty row below the last entry
	// (or the pagination line). The button row follows the explanation.
	// Reserve the workspace footer below even on the minimum terminal height.
	height := min(max(1, c.H-3), visible+18+pageRows)
	x, y := (c.W-w)/2, (c.H-height)/2
	a.dialogX, a.dialogY, a.dialogW, a.dialogH = x, y, w, height
	c.Fill(x, y, w, height, ' ', CWhite, CDefault)
	drawDialogBorder(c, x, y, w, height)
	c.Text(x+2, y+1, "Remote changes detected", CBBlue, CDefault)
	if p.RemoteFromRegistration {
		c.Text(x+2, y+3,
			minWidth("Cannot push edits to Bitwarden: remote vault has changed", w-4),
			CWhite, CDefault)
		c.Text(x+2, y+4,
			minWidth("No local edits were sent. Remote changes:", w-4),
			CWhite, CDefault)
	} else {
		c.Text(x+2, y+3,
			minWidth("Bitwarden remote has changes not yet shown in LazyBW", w-4),
			CWhite, CDefault)
		c.Text(x+2, y+4,
			minWidth("New, modified or deleted items and folders:", w-4),
			CWhite, CDefault)
	}
	start := p.Page * visible
	for i := 0; i < visible && start+i < len(p.Lines); i++ {
		line := []rune(p.Lines[start+i])
		if len(line) == 0 {
			continue
		}
		if len(line) > w-6 {
			line = append([]rune(minWidth(string(line[:len(line)-1]), w-9)), ' ', ' ', line[len(line)-1])
		}
		c.Text(x+3, y+6+i, string(line), CWhite, CDefault)
		last := line[len(line)-1]
		color := CWhite
		switch last {
		case 'N':
			color = CGreen
		case 'M':
			color = CYellow
		case 'D':
			color = CRed
		}
		c.Put(x+3+len(line)-1, y+6+i, last, color, CDefault)
	}
	lineAfterList := y + 6 + visible
	if paginated {
		c.Text(x+3, lineAfterList,
			fmt.Sprintf("? next page  %d/%d", p.Page+1,
				(len(p.Lines)+visible-1)/visible), CBBlack, CDefault)
		lineAfterList++
	}
	// Exactly one blank row separates the diff list from its explanation.
	infoY := lineAfterList + 1
	if a.S.Stage.Len() == 0 {
		c.Text(x+3, infoY, "SYNC imports all remote updates into local ground values.", CWhite, CDefault)
		c.Text(x+3, infoY+1, "No local edits exist; nothing is pushed to Bitwarden.", CWhite, CDefault)
		c.Text(x+3, infoY+3, "SYNC        Apply remote changes to local ground values", CWhite, CDefault)
		c.Text(x+3, infoY+4, minWidth("ABORT SYNC  Leave local ground values unchanged; does not resolve conflict", w-6), CWhite, CDefault)
	} else {
		c.Text(x+3, infoY,
			minWidth("Both options import remote data; neither uploads local edits", w-6),
			CWhite, CDefault)
		c.Text(x+3, infoY+1,
			minWidth("Only pending edits and the Edits panel are handled differently:", w-6),
			CWhite, CDefault)
		c.Text(x+3, infoY+3,
			minWidth("SOFT SYNC   Update ground values; keep local edits, including conflicts", w-6),
			CWhite, CDefault)
		c.Text(x+3, infoY+4,
			minWidth("HARD SYNC   Update ground values; discard conflicting local edits", w-6),
			CWhite, CDefault)
		c.Text(x+3, infoY+5, minWidth("ABORT SYNC  Leave both local ground values and local edits unchanged; Does not resolve conflict", w-6), CWhite, CDefault)
	}
	by := y + height - 4
	mx, my := -1, -1
	if a.gui != nil {
		mx, my = a.gui.MousePosition()
	}
	buttons := remoteReviewButtons(x, w)
	if a.S.Stage.Len() == 0 {
		buttons[0] = remoteReviewButton{x + (w-30)/3, 13, "SYNC", CGreen}
		buttons[2] = remoteReviewButton{x + (w-30)*2/3 + 13, 14, "ABORT SYNC", CRed}
	}
	for i, b := range buttons {
		if a.S.Stage.Len() == 0 && i == 1 {
			continue
		}
		a.button(c, b.x, by, b.width, b.label, b.color, p.RemoteChoice == i,
			my >= by && my <= by+1 && mx >= b.x && mx < b.x+b.width)
	}
}

// checkRemote opens a review of server changes without registering writes.
func (a *App) checkRemote() {
	s := a.S
	if s.Screen != Workspace || s.LocalMode {
		if !s.Busy && s.Popup == nil {
			s.Popup = &Popup{Title: "Synchronization unavailable", Question: "Unlock an online vault to synchronize", Acknowledge: true}
		}
		return
	}
	if s.Mode != Browse && s.Mode != Find {
		if s.Popup == nil && !s.Busy {
			s.Popup = &Popup{Title: "Finish current action", Question: "Complete or cancel the current edit before synchronizing", Acknowledge: true}
		}
		return
	}
	if !s.ServerConnected {
		// The shortcut and its hint are disabled while disconnected.
		return
	}
	if s.Busy || s.Popup != nil {
		return
	}
	expectedItems := make(map[string]map[string]any, len(s.Originals))
	expectedFolders := make(map[string]map[string]any, len(s.FolderOriginals))
	for id, v := range s.Originals {
		expectedItems[id] = vault.Clone(v)
	}
	for id, v := range s.FolderOriginals {
		expectedFolders[id] = vault.Clone(v)
	}
	s.Busy = true
	s.BusyMessage = "Checking remote vault"
	go func() {
		review, err := s.CLI.CompareRemoteState(context.Background(), expectedItems, expectedFolders)
		a.gui.Update(func(*gocui.Gui) error {
			s.Busy = false
			s.BusyMessage = ""
			if err != nil {
				s.Popup = &Popup{Title: "Synchronization failed", Question: bitwarden.VerificationFailure(err), Acknowledge: true}
			} else if len(review.Differences) == 0 {
				s.UnverifiedPush = false
				s.Popup = &Popup{Title: "Vault already synchronized", Question: "No remote changes detected", Acknowledge: true}
			} else {
				a.showRemoteReview(review)
			}
			return nil
		})
	}()
}

// registrationBlocked leaves all staged changes intact when the remote vault
// changed or its current state could not be verified. No backup is queued
// because this attempt did not write anything to Bitwarden.
func (a *App) registrationBlocked(message string) {
	a.S.Busy = false
	a.S.BusyMessage = ""
	if !a.shutdownRequested.Load() {
		// Verification failures need an actionable, persistent explanation.
		// A one-second footer notice can be truncated and missed.
		a.S.Popup = &Popup{
			Title: "Push blocked", Question: message,
			Acknowledge: true,
		}
	}
	a.setRegistrationInProgress(false)
}

// registrationFailed reports remote errors before any queued local backup.
// A partial remote batch may still need an updated local snapshot.
func (a *App) registrationFailed() {
	a.S.Busy = false
	if !a.shutdownRequested.Load() {
		a.S.Popup = nil
	}
	a.S.SetWorkspaceNotice("Edits push failed", CRed)
	if !a.S.UnverifiedPush {
		a.queueBackupAfterRegistration()
	}
	a.setRegistrationInProgress(false)
}

// planPush preserves the selected subset while ordering an entire staged batch.
// It rejects unresolved temporary folder references before any remote writes.
func planPush(stage *vault.Stage, changes []*vault.Change) ([]*vault.Change, bool) {
	if len(changes) == stage.Len() {
		changes = stage.OrderedForSave()
	}
	newFolders := make(map[string]bool)
	for _, ch := range changes {
		if ch.Entity == "folder" && ch.Kind == vault.New && ch.After != nil {
			id, _ := ch.After["id"].(string)
			newFolders[id] = true
		}
	}
	for _, ch := range changes {
		if ch.Entity != "item" || ch.After == nil {
			continue
		}
		folder, _ := ch.After["folderId"].(string)
		if strings.HasPrefix(folder, "local-") && !newFolders[folder] {
			return nil, false
		}
	}
	return changes, true
}

func (a *App) saveChanges(changes []*vault.Change) {
	s := a.S
	if s.Busy || s.LocalMode || !s.ServerConnected || len(changes) == 0 {
		return
	}
	if s.UnverifiedPush {
		s.Popup = &Popup{Title: "Push blocked", Question: "Confirm the last push with SYNC before pushing again", Acknowledge: true}
		return
	}
	planned, ok := planPush(s.Stage, changes)
	if !ok {
		s.Popup = &Popup{Title: "Push blocked", Question: "Push the referenced folder before its items", Acknowledge: true}
		return
	}
	changes = planned
	for _, change := range changes {
		if change.Entity == "item" && !vault.CanPushItemChange(change.Before, change.After) {
			s.Popup = &Popup{Title: "Push blocked", Question: "Item permissions do not allow this edit", Acknowledge: true}
			return
		}
	}
	// Capture the last confirmed remote snapshot on the UI goroutine.
	// The staging area is intentionally excluded from the comparison.
	expectedItems := make(map[string]map[string]any, len(s.Originals))
	for id, item := range s.Originals {
		expectedItems[id] = vault.Clone(item)
	}
	expectedFolders := make(map[string]map[string]any, len(s.FolderOriginals))
	for id, folder := range s.FolderOriginals {
		expectedFolders[id] = vault.Clone(folder)
	}
	s.Busy = true
	a.setRegistrationInProgress(true)
	s.BusyMessage = "Checking remote vault"
	s.ClearWorkspaceNotice()
	// This starts from a UI callback; returning redraws automatically.
	// Do not call gocui.Update on the event-loop goroutine.
	go func() {
		ctx := context.Background()
		// A sync and complete comparison must succeed before the first write.
		// Bitwarden CLI has no atomic compare-and-swap for an entire batch:
		// changes occurring *after* this check cannot be prevented here.
		review, err := s.CLI.CompareRemoteState(ctx, expectedItems, expectedFolders)
		if err != nil {
			a.gui.Update(func(*gocui.Gui) error {
				a.registrationBlocked(bitwarden.VerificationFailure(err))
				return a.quitResult(nil)
			})
			return
		}
		if len(review.Differences) > 0 {
			a.gui.Update(func(*gocui.Gui) error {
				s.Busy = false
				s.BusyMessage = ""
				if a.shutdownRequested.Load() {
					a.shutdownRequested.Store(false)
					s.QuitWhenSaved = false
				}
				a.showRemoteReviewWithContext(review, true)
				a.setRegistrationInProgress(false)
				return nil
			})
			return
		}
		a.gui.Update(func(*gocui.Gui) error {
			s.BusyMessage = fmt.Sprintf("Pushing %d edit(s)", len(changes))
			return nil
		})
		folderIDs := map[string]string{}
		pushed := make([]pushedEntry, 0, len(changes))
		for _, old := range changes {
			ch := *old
			if ch.After != nil {
				ch.After = vault.Clone(ch.After)
				if folder, ok := ch.After["folderId"].(string); ok && folderIDs[folder] != "" {
					ch.After["folderId"] = folderIDs[folder]
				}
			}
			remote, err := s.CLI.Save(ctx, &ch)
			if err != nil {
				a.gui.Update(func(*gocui.Gui) error {
					a.registrationFailed()
					return a.quitResult(nil)
				})
				return
			}
			if ch.Entity == "folder" && ch.Kind == vault.New && remote != nil {
				folderIDs[fmt.Sprint(ch.After["id"])] = fmt.Sprint(remote["id"])
			}
			id := ""
			if ch.Kind == vault.Deleted && ch.Before != nil {
				id = fmt.Sprint(ch.Before["id"])
			} else if remote != nil {
				id = fmt.Sprint(remote["id"])
			}
			pushed = append(pushed, pushedEntry{entity: ch.Entity, id: id, change: old})
			done := make(chan struct{})
			a.gui.Update(func(*gocui.Gui) error {
				s.CommitRemote(old, remote)
				s.UnverifiedPush = true
				if ch.Entity == "folder" && ch.Kind == vault.New && remote != nil {
					s.Stage.RemapFolder(fmt.Sprint(ch.After["id"]), fmt.Sprint(remote["id"]))
				}
				close(done)
				return nil
			})
			<-done
		}
		// Confirm the write using bw sync + bw list, rather than treating
		// bw edit/create's response as the canonical post-write snapshot.
		a.gui.Update(func(*gocui.Gui) error {
			s.BusyMessage = "Confirming remote push"
			return nil
		})
		confirmed, refreshErr := s.CLI.SyncedSnapshot(ctx)
		a.gui.Update(func(*gocui.Gui) error {
			s.Busy = false
			if refreshErr != nil || !s.AcceptPushedSnapshot(confirmed, pushed) {
				// A successful remote write cannot be safely retried without sync.
				s.Popup = &Popup{Title: "Push confirmation pending",
					Question: "Edits were sent, but their remote state could not be verified. Use SYNC before another push.", Acknowledge: true}
				a.setRegistrationInProgress(false)
				return a.quitResult(nil)
			}
			if !a.shutdownRequested.Load() {
				s.Popup = nil
			}
			s.SetWorkspaceNotice("Edits pushed", CGreen)
			a.queueBackupAfterRegistration()
			a.setRegistrationInProgress(false)
			return a.quitResult(nil)
		})
	}()
}
func (a *App) confirmStage(save, all bool) {
	s := a.S
	if a.modalOpen() || s.Busy || s.LocalMode || (save && !s.ServerConnected) {
		return
	}
	changes := s.Stage.Changes()
	if !all {
		if len(changes) == 0 {
			return
		}
		if s.ImportedChangeAt(s.ChangeIndex) {
			return
		}
		if s.ChangeIndex >= len(changes) {
			s.ChangeIndex = len(changes) - 1
		}
		if s.ChangeIndex < 0 {
			return
		}
		changes = []*vault.Change{changes[s.ChangeIndex]}
	}
	if len(changes) == 0 {
		return
	}
	verb := "Discard"
	if save {
		verb = "Push"
	}
	a.confirm(verb+" edits?", fmt.Sprintf("%s the selected pending edit(s)?", verb), changes, save, false, func(ok bool) {
		if !ok {
			return
		}
		if save {
			a.saveChanges(changes)
		} else {
			for _, ch := range changes {
				s.Restore(ch)
			}
			s.SetWorkspaceNotice("Edits discarded", CWhite)
		}
	})
}
func (a *App) clipboard(value string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd.exe", "/c", "clip")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy", "--type", "text/plain")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("clip.exe"); err == nil {
			cmd = exec.Command("clip.exe")
		} else {
			return fmt.Errorf("clipboard tool unavailable")
		}
	}
	cmd.Stdin = strings.NewReader(value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard unavailable")
	}
	return nil
}
