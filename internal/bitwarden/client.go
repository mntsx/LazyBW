// Package bitwarden is the isolated subprocess adapter for the official bw CLI.
package bitwarden

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

type Runner interface {
	Run(context.Context, []string, []byte, []string) ([]byte, []byte, int, error)
}
type ExecRunner struct{ Binary string }

func (r ExecRunner) Run(ctx context.Context, args []string, input []byte, extraEnv []string) ([]byte, []byte, int, error) {
	binary := r.Binary
	if binary == "" {
		binary = "bw"
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	// Explicit per-process environment overrides take precedence. Remove
	// inherited copies instead of relying on duplicate-variable semantics,
	// which differ across runtimes used by bw.
	cmd.Env = overriddenEnvironment(os.Environ(), extraEnv)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		var e *exec.ExitError
		if errors.As(err, &e) {
			code = e.ExitCode()
			err = nil
		}
	}
	return stdout.Bytes(), stderr.Bytes(), code, err
}

type CLIError struct {
	Operation, Category string
	Code                int
}

func (e *CLIError) Error() string {
	return fmt.Sprintf("%s failed (exit %d: %s)", e.Operation, e.Code, e.Category)
}
func category(stderr []byte) string {
	s := strings.ToLower(string(stderr))
	// CLI text is untrusted and may include secret-bearing diagnostics.
	// Inspect it for known categories, but never interpolate it into errors.
	switch {
	case strings.Contains(s, "incorrect master password") ||
		strings.Contains(s, "invalid master password") ||
		strings.Contains(s, "invalid password") ||
		strings.Contains(s, "master password is incorrect"):
		return "incorrect password"
	case strings.Contains(s, "invalid_client") ||
		strings.Contains(s, "invalid client") ||
		strings.Contains(s, "invalid_grant") ||
		strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "invalid credentials"):
		return "invalid credentials"
	case strings.Contains(s, "locked") || strings.Contains(s, "session expired"):
		return "vault locked"
	case strings.Contains(s, "too many requests") ||
		strings.Contains(s, "rate limit") || strings.Contains(s, "429"):
		return "rate limited"
	case strings.Contains(s, "captcha") ||
		strings.Contains(s, "authentication challenge"):
		return "challenge"
	case strings.Contains(s, "network") || strings.Contains(s, "connect") ||
		strings.Contains(s, "timeout") || strings.Contains(s, "econn") ||
		strings.Contains(s, "enotfound") || strings.Contains(s, "fetch failed"):
		return "network"
	case strings.Contains(s, "unauthorized") || strings.Contains(s, "not logged"):
		return "authentication"
	case strings.Contains(s, "permission") || strings.Contains(s, "forbidden"):
		return "permissions"
	case strings.Contains(s, "validation") || strings.Contains(s, "invalid") || strings.Contains(s, "required"):
		return "validation"
	default:
		return "unknown"
	}
}

func overriddenEnvironment(inherited, overrides []string) []string {
	keys := make(map[string]struct{}, len(overrides))
	for _, value := range overrides {
		if key, _, ok := strings.Cut(value, "="); ok {
			keys[key] = struct{}{}
		}
	}
	environment := make([]string, 0, len(inherited)+len(overrides))
	for _, value := range inherited {
		key, _, ok := strings.Cut(value, "=")
		if _, replaced := keys[key]; ok && replaced {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment, overrides...)
}

type Client struct {
	runner        Runner
	mu            sync.Mutex
	sessionSealed *vault.Sealed
	mode          BackendMode
	serve         *BWServeBackend
	repromptTag   [32]byte
	repromptReady bool
	profileDir    string
	sessionDir    string
	sessionLock   *os.File
	Cipher        *vault.Cipher
}

func NewClient(r Runner) (*Client, error) {
	if r == nil {
		r = ExecRunner{}
	}
	c, err := vault.NewCipher()
	if err != nil {
		return nil, err
	}
	return &Client{runner: r, Cipher: c, mode: BackendBW, serve: &BWServeBackend{}}, nil
}
func (c *Client) call(ctx context.Context, op string, args []string, input []byte, vars []string, session bool) ([]byte, error) {
	c.mu.Lock()
	mode := c.mode
	profile := c.profileDir
	serve := c.serve
	c.mu.Unlock()
	key := c.sessionValue()
	if mode == BackendBWServe {
		ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
		defer cancel()
		if profile == "" {
			return nil, &CLIError{op, "profile unavailable", 0}
		}
		if err := serve.Start(ctx, profile, key); err != nil {
			return nil, err
		}
		var payload any
		if len(input) > 0 {
			if err := json.Unmarshal(input, &payload); err != nil {
				return nil, &CLIError{op, "serialization", 0}
			}
		}
		return serve.call(ctx, args, payload)
	}
	return c.callCLI(ctx, op, args, input, vars, session, key)
}

func (c *Client) callCLI(ctx context.Context, op string, args []string, input []byte, vars []string, session bool, key string) ([]byte, error) {
	c.mu.Lock()
	profile := c.profileDir
	c.mu.Unlock()
	if profile != "" {
		vars = append(vars, "BITWARDENCLI_APPDATA_DIR="+profile)
	}
	// Clearing the inherited variable is essential even for status and login.
	if session && key != "" {
		vars = append(vars, "BW_SESSION="+key)
	} else {
		vars = append(vars, "BW_SESSION=")
	}
	ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	out, errOut, code, err := c.runner.Run(ctx, args, input, vars)
	// Output buffers are caller-owned and can contain CLI diagnostics or
	// account values. Errors expose only fixed categories, never raw bytes.
	defer func() {
		for i := range errOut {
			errOut[i] = 0
		}
	}()
	if err != nil {
		for i := range out {
			out[i] = 0
		}
		return nil, &CLIError{op, "execution", -1}
	}
	if code != 0 {
		defer func() {
			for i := range out {
				out[i] = 0
			}
		}()
		// Bitwarden CLI versions differ in where they write failure details.
		// Classify both streams without ever storing or displaying raw output,
		// which can contain confidential account information.
		kind := category(errOut)
		if kind == "unknown" {
			kind = category(out)
		}
		return nil, &CLIError{op, kind, code}
	}
	return bytes.TrimSpace(out), nil
}
func (c *Client) raw(ctx context.Context, args ...string) ([]byte, error) {
	return c.call(ctx, "bw "+strings.Join(args, " "), args, nil, nil, false)
}
func (c *Client) ses(ctx context.Context, args ...string) ([]byte, error) {
	return c.call(ctx, "bw "+strings.Join(args, " "), args, nil, nil, true)
}
func (c *Client) rawCLI(ctx context.Context, args ...string) ([]byte, error) {
	return c.callCLI(ctx, "bw "+strings.Join(args, " "), args, nil, nil, false, "")
}
func (c *Client) sesCLI(ctx context.Context, args ...string) ([]byte, error) {
	return c.callCLI(ctx, "bw "+strings.Join(args, " "), args, nil, nil, true, c.sessionValue())
}
func decodeObject(b []byte, operation string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil || v == nil {
		return nil, &CLIError{operation, "invalid JSON response", 0}
	}
	return v, nil
}
func decodeList(b []byte, operation string) ([]map[string]any, error) {
	var v []map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, &CLIError{operation, "invalid JSON response", 0}
	}
	return v, nil
}

// ConfigureRegion selects an isolated CLI profile for this LazyBW execution
// and verifies its endpoint. It does not change the user's default bw
// configuration or shell environment.
func (c *Client) ConfigureRegion(ctx context.Context, configDir, region string) error {
	url := "https://vault.bitwarden.com"
	switch region {
	case "us":
	case "eu":
		url = "https://vault.bitwarden.eu"
	default:
		return errors.New("unsupported Bitwarden region")
	}
	if c.BackendMode() == BackendBWServe && c.serve != nil {
		_ = c.serve.Stop(ctx)
	}
	profile, err := c.sessionProfile(configDir, region)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.profileDir = profile
	c.clearSessionLocked()
	c.repromptReady = false
	c.repromptTag = [32]byte{}
	c.mu.Unlock()
	b, err := c.rawCLI(ctx, "status")
	if err != nil {
		return err
	}
	status, err := decodeObject(b, "bw status")
	if err != nil {
		return err
	}
	// Preserve an existing, correctly configured profile. Re-running
	// `bw config server` on each launch needlessly writes the CLI data file
	// and may discard custom service endpoints set by the CLI itself.
	if current, ok := status["serverUrl"].(string); ok &&
		strings.TrimRight(current, "/") == url {
		return nil
	}
	// Some CLI versions return an empty serverUrl for a fresh US profile.
	// US is their built-in default and does not need reconfiguration.
	if region == "us" && status["serverUrl"] == nil &&
		status["status"] == "unauthenticated" {
		return nil
	}
	// Never change the endpoint of a logged-in profile without first
	// logging out; this is confined to LazyBW's isolated directory.
	if status["status"] != "unauthenticated" {
		if _, err := c.rawCLI(ctx, "logout"); err != nil {
			return err
		}
	}
	_, err = c.rawCLI(ctx, "config", "server", url)
	return err
}

func (c *Client) sessionProfile(configDir, region string) (string, error) {
	c.mu.Lock()
	if c.sessionDir != "" {
		sessionDir := c.sessionDir
		c.mu.Unlock()
		profile := filepath.Join(sessionDir, region)
		if err := os.MkdirAll(profile, 0700); err != nil {
			return "", fmt.Errorf("cannot create Bitwarden profile: %w", err)
		}
		if err := os.Chmod(profile, 0700); err != nil {
			return "", fmt.Errorf("cannot secure Bitwarden profile: %w", err)
		}
		return profile, nil
	}
	c.mu.Unlock()

	base, err := filepath.Abs(filepath.Join(configDir, "bitwarden", "sessions"))
	if err != nil {
		return "", fmt.Errorf("invalid Bitwarden session directory: %w", err)
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", fmt.Errorf("cannot create Bitwarden sessions directory: %w", err)
	}
	if err := os.Chmod(base, 0700); err != nil {
		return "", fmt.Errorf("cannot secure Bitwarden sessions directory: %w", err)
	}
	cleanupAbandonedSessions(base, time.Now())

	for attempts := 0; attempts < 10; attempts++ {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", fmt.Errorf("cannot generate Bitwarden session ID: %w", err)
		}
		id := fmt.Sprintf("%d-%d-%s", time.Now().UnixNano(), os.Getpid(), hex.EncodeToString(random[:]))
		sessionDir := filepath.Join(base, id)
		if err := os.Mkdir(sessionDir, 0700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", fmt.Errorf("cannot create Bitwarden session: %w", err)
		}
		lock, err := lockSession(sessionDir, false)
		if err != nil {
			_ = os.RemoveAll(sessionDir)
			return "", fmt.Errorf("cannot lock Bitwarden session: %w", err)
		}
		profile := filepath.Join(sessionDir, region)
		if err := os.Mkdir(profile, 0700); err != nil {
			releaseSessionLock(lock)
			_ = os.RemoveAll(sessionDir)
			return "", fmt.Errorf("cannot create Bitwarden profile: %w", err)
		}

		c.mu.Lock()
		if c.sessionDir == "" {
			c.sessionDir = sessionDir
			c.sessionLock = lock
			c.mu.Unlock()
			return profile, nil
		}
		activeSession := c.sessionDir
		c.mu.Unlock()
		releaseSessionLock(lock)
		_ = os.RemoveAll(sessionDir)
		profile = filepath.Join(activeSession, region)
		if err := os.MkdirAll(profile, 0700); err != nil {
			return "", fmt.Errorf("cannot create Bitwarden profile: %w", err)
		}
		return profile, nil
	}
	return "", errors.New("cannot allocate unique Bitwarden session ID")
}

// ServeRunning reports only the process owned by this client.
func (c *Client) ServeRunning() bool {
	c.mu.Lock()
	serve := c.serve
	mode := c.mode
	c.mu.Unlock()
	return mode == BackendBWServe && serve != nil && serve.running()
}

// RecoverServe starts a replacement private process with the protected session
// token and confirms it can read an authenticated vault endpoint. It never
// performs lock, logout, or an uncertain write.
func (c *Client) RecoverServe(ctx context.Context) error {
	c.mu.Lock()
	mode, profile, serve := c.mode, c.profileDir, c.serve
	c.mu.Unlock()
	if mode != BackendBWServe || serve == nil || profile == "" {
		return errors.New("bw serve recovery is unavailable")
	}
	key := c.sessionValue()
	if key == "" {
		return &CLIError{"bw serve recovery", "authentication", 0}
	}
	if err := serve.Start(ctx, profile, key); err != nil {
		return err
	}
	if _, err := serve.call(ctx, []string{"list", "folders"}, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) BackendMode() BackendMode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode
}

// SelectBackend changes only this LazyBW session. It deliberately clears the
// unlock token: an unlocked session must never be carried between transports.
func (c *Client) SelectBackend(ctx context.Context, mode BackendMode) error {
	if mode != BackendBW && mode != BackendBWServe {
		return errors.New("unsupported Bitwarden backend")
	}
	if mode == BackendBWServe {
		if err := c.BackendAvailable(ctx, mode); err != nil {
			return err
		}
	}
	previous := c.BackendMode()
	if previous == mode {
		return nil
	}
	if c.HasSession() {
		if previous == BackendBW {
			_, _ = c.sesCLI(ctx, "lock")
		} else if c.serve != nil && c.ServeRunning() {
			_, _ = c.serve.call(ctx, []string{"lock"}, nil)
		}
	}
	c.mu.Lock()
	previous = c.mode
	serve := c.serve
	c.mode = mode
	c.clearSessionLocked()
	c.repromptReady = false
	c.repromptTag = [32]byte{}
	c.mu.Unlock()
	if previous == BackendBWServe && serve != nil {
		return serve.Stop(ctx)
	}
	return nil
}

// SelectAvailableBackend applies ordered configuration fallback only before
// authentication. Later callers retain their explicit selected backend.
func (c *Client) SelectAvailableBackend(ctx context.Context, modes []string) (BackendMode, error) {
	for _, value := range modes {
		mode := BackendMode(value)
		if err := c.BackendAvailable(ctx, mode); err == nil {
			if err := c.SelectBackend(ctx, mode); err != nil {
				return "", err
			}
			return mode, nil
		}
	}
	return "", errors.New("no configured Bitwarden backend is available")
}

// BackendAvailable performs no serve start. The normal CLI remains compatible
// with every version; bw serve additionally requires 2025.9.0 and Unix help.
func (c *Client) BackendAvailable(ctx context.Context, mode BackendMode) error {
	if mode != BackendBW && mode != BackendBWServe {
		return errors.New("unsupported Bitwarden backend")
	}
	version, err := c.callCLI(ctx, "bw --version", []string{"--version"}, nil, nil, false, "")
	if err != nil {
		return err
	}
	if mode == BackendBW {
		return nil
	}
	if !serveVersionSupported(string(version)) {
		return errors.New("bw serve requires Bitwarden CLI 2025.9.0 or later")
	}
	help, err := c.callCLI(ctx, "bw serve --help", []string{"serve", "--help"}, nil, nil, false, "")
	// Recent bw releases support unix:// through --hostname but do not mention
	// that transport in their generated help text. The version gate above is
	// the compatibility guarantee; help only confirms the required option.
	if err != nil || !strings.Contains(strings.ToLower(string(help)), "--hostname") {
		return errors.New("bw serve does not support Unix sockets")
	}
	dir, err := secureServeDirectory()
	if err != nil {
		return err
	}
	return os.Remove(dir)
}

func (c *Client) clearSessionLocked() {
	if c.sessionSealed != nil {
		for i := range c.sessionSealed.Data {
			c.sessionSealed.Data[i] = 0
		}
		for i := range c.sessionSealed.Nonce {
			c.sessionSealed.Nonce[i] = 0
		}
		c.sessionSealed = nil
	}
}

func (c *Client) setSession(value string) error {
	if value == "" {
		c.mu.Lock()
		c.clearSessionLocked()
		c.mu.Unlock()
		return nil
	}
	sealed, err := c.Cipher.Seal(value)
	if err != nil {
		return err
	}
	secret, ok := sealed.(*vault.Sealed)
	if !ok {
		return errors.New("session encryption unavailable")
	}
	c.mu.Lock()
	c.clearSessionLocked()
	c.sessionSealed = secret
	c.mu.Unlock()
	return nil
}

func (c *Client) sessionValue() string {
	c.mu.Lock()
	sealed := c.sessionSealed
	c.mu.Unlock()
	if sealed == nil {
		return ""
	}
	value, err := c.Cipher.Open(sealed)
	if err != nil {
		return ""
	}
	return value
}

func (c *Client) Version(ctx context.Context) string {
	b, err := c.rawCLI(ctx, "--version")
	if err != nil {
		return "unavailable"
	}
	return strings.Split(string(b), "\n")[0]
}
func (c *Client) Status(ctx context.Context) (map[string]any, error) {
	// bw serve exits before it can expose /status for an unauthenticated
	// profile. Authentication state is not a vault operation, so inspect the
	// isolated profile with bw until unlock has established this session.
	if c.BackendMode() == BackendBWServe && !c.HasSession() {
		b, err := c.rawCLI(ctx, "status")
		if err != nil {
			return nil, err
		}
		return decodeObject(b, "bw status")
	}
	b, err := c.raw(ctx, "status")
	if err != nil {
		return nil, err
	}
	return decodeObject(b, "bw status")
}
func (c *Client) Login(ctx context.Context, id, secret string) error {
	// bw serve has no login route. Stop the private process before modifying its
	// profile, then start a fresh process for subsequent HTTP vault operations.
	if c.BackendMode() == BackendBWServe {
		_ = c.serve.Stop(ctx)
	}
	b, err := c.rawCLI(ctx, "status")
	if err != nil {
		return err
	}
	status, err := decodeObject(b, "bw status")
	if err != nil {
		return err
	}
	if status["status"] != "unauthenticated" {
		if strings.TrimPrefix(id, "user.") == fmt.Sprint(status["userId"]) {
			return nil
		}
		if _, err = c.rawCLI(ctx, "logout"); err != nil {
			return err
		}
	}
	_, err = c.callCLI(ctx, "bw login --apikey", []string{"login", "--apikey"}, nil, []string{"BW_CLIENTID=" + id, "BW_CLIENTSECRET=" + secret}, false, "")
	return err
}

// AuthenticationMessage turns CLI-specific failures into concise, safe UI text.
func AuthenticationMessage(err error, master bool) string {
	if err == nil {
		return ""
	}
	var e *CLIError
	if !errors.As(err, &e) {
		return "Unable to authenticate. Please try again"
	}
	switch e.Category {
	case "network":
		return "Connection failed. Check your network connection"
	case "rate limited":
		return "Bitwarden is limiting login attempts. Please try later"
	case "challenge":
		return "Bitwarden requires an extra authentication challenge"
	case "execution":
		return "Bitwarden CLI could not be started"
	case "vault locked":
		return "Bitwarden session expired. Please log in again"
	}
	if master {
		if e.Operation == "bw unlock" && e.Code != 0 &&
			e.Category != "permissions" {
			return "Incorrect master password. Try again"
		}
		return "Unable to unlock vault. Check your Bitwarden session"
	}
	switch e.Category {
	case "invalid credentials":
		return "Bitwarden rejected the client ID or client secret. Check the selected server"
	case "authentication":
		return "Bitwarden session is not authenticated. Try signing in again"
	case "permissions":
		return "API authentication is not permitted for this account"
	case "validation":
		return "Bitwarden rejected the authentication request format"
	case "unknown":
		return "Bitwarden CLI login failed for an unknown reason (not necessarily the API key)"
	default:
		return "Unable to sign in with API credentials"
	}
}

func (c *Client) Unlock(ctx context.Context, password string) error {
	if c.BackendMode() == BackendBWServe {
		c.mu.Lock()
		profile, serve := c.profileDir, c.serve
		c.mu.Unlock()
		if profile == "" {
			return &CLIError{"bw unlock", "profile unavailable", 0}
		}
		ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
		defer cancel()
		if err := serve.Start(ctx, profile, ""); err != nil {
			return err
		}
		b, err := serve.request(ctx, "POST", "/unlock", map[string]string{"password": password})
		if err != nil {
			return err
		}
		defer func() {
			for i := range b {
				b[i] = 0
			}
		}()
		var message struct {
			Raw string `json:"raw"`
		}
		if json.Unmarshal(b, &message) != nil || message.Raw == "" {
			return &CLIError{"bw unlock", "invalid JSON response", 0}
		}
		if err := c.setSession(message.Raw); err != nil {
			return err
		}
		return c.SetRepromptMaster(password)
	}
	b, err := c.callCLI(ctx, "bw unlock", []string{"unlock", "--raw", "--passwordenv", "LAZYBW_MASTER_PASSWORD"}, nil, []string{"LAZYBW_MASTER_PASSWORD=" + password}, false, "")
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return &CLIError{"bw unlock", "empty response", 0}
	}
	defer func() {
		for i := range b {
			b[i] = 0
		}
	}()
	if err := c.setSession(string(b)); err != nil {
		return err
	}
	return c.SetRepromptMaster(password)
}

// SetRepromptMaster retains a keyed verifier, not the cleartext master password.
func (c *Client) SetRepromptMaster(password string) error {
	if password == "" {
		return errors.New("master password is empty")
	}
	sealed, err := c.Cipher.Seal(password)
	if err != nil {
		return err
	}
	secret, ok := sealed.(*vault.Sealed)
	if !ok {
		return errors.New("master password verifier unavailable")
	}
	c.mu.Lock()
	c.repromptTag = secret.Tag
	c.repromptReady = true
	c.mu.Unlock()
	for i := range secret.Data {
		secret.Data[i] = 0
	}
	for i := range secret.Nonce {
		secret.Nonce[i] = 0
	}
	return nil
}

// VerifyReprompt compares independent authenticated tags without a CLI call.
func (c *Client) VerifyReprompt(candidate string) bool {
	c.mu.Lock()
	tag, ready := c.repromptTag, c.repromptReady
	c.mu.Unlock()
	if !ready || candidate == "" {
		return false
	}
	sealed, err := c.Cipher.Seal(candidate)
	if err != nil {
		return false
	}
	secret, ok := sealed.(*vault.Sealed)
	if !ok {
		return false
	}
	equal := hmac.Equal(tag[:], secret.Tag[:])
	for i := range secret.Data {
		secret.Data[i] = 0
	}
	for i := range secret.Nonce {
		secret.Nonce[i] = 0
	}
	return equal
}

func (c *Client) SetSession(s string) { _ = c.setSession(s) }
func (c *Client) HasSession() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionSealed != nil
}
func (c *Client) Close(ctx context.Context) {
	// An unauthenticated or locked CLI has no active BW_SESSION to revoke.
	// Spawning `bw lock` in that case only delays application shutdown.
	if c.HasSession() {
		if c.BackendMode() != BackendBWServe || c.ServeRunning() {
			_, _ = c.ses(ctx, "lock")
		}
	}
	if c.BackendMode() == BackendBWServe && c.serve != nil {
		_ = c.serve.Stop(ctx)
	}
	c.mu.Lock()
	c.clearSessionLocked()
	c.repromptTag = [32]byte{}
	c.repromptReady = false
	sessionDir := c.sessionDir
	c.sessionDir = ""
	c.profileDir = ""
	sessionLock := c.sessionLock
	c.sessionLock = nil
	c.mu.Unlock()
	if sessionDir != "" {
		_ = os.RemoveAll(sessionDir)
	}
	releaseSessionLock(sessionLock)
	c.Cipher.Close()
}

// Snapshot loads independent lists concurrently and encrypts hidden values on receipt.
func (c *Client) Snapshot(ctx context.Context) ([]*vault.Item, []map[string]any, []map[string]any, error) {
	// The CLI may otherwise return cached data and overwrite a newer backup.
	if _, err := c.ses(ctx, "sync"); err != nil {
		return nil, nil, nil, err
	}
	var wg sync.WaitGroup
	var it, fold, org []map[string]any
	var err1, err2, err3 error
	wg.Add(2)
	go func() {
		defer wg.Done()
		b, e := c.ses(ctx, "list", "items")
		if e != nil {
			err1 = e
			return
		}
		it, err1 = decodeValidatedList(b, "items")
		for idx := range b {
			b[idx] = 0
		}
		if err1 == nil {
			for _, v := range it {
				if e = vault.SealItem(v, c.Cipher); e != nil {
					err1 = e
					return
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		b, e := c.ses(ctx, "list", "folders")
		if e != nil {
			err2 = e
			return
		}
		fold, err2 = decodeValidatedList(b, "folders")

	}()
	wg.Wait()
	if err1 != nil {
		return nil, nil, nil, err1
	}
	if err2 != nil {
		return nil, nil, nil, err2
	}
	needOrgs := false
	for _, v := range it {
		if v["organizationId"] != nil && v["organizationId"] != "" {
			needOrgs = true
			break
		}
	}
	if needOrgs {
		b, e := c.ses(ctx, "list", "organizations")
		if e == nil {
			org, err3 = decodeList(b, "bw list organizations")
		}
		_ = err3
	}
	owners := map[string]string{}
	for _, v := range org {
		owners[fmt.Sprint(v["id"])] = fmt.Sprint(v["name"])
	}
	items := make([]*vault.Item, 0, len(it))
	for _, v := range it {
		if id, ok := v["organizationId"].(string); ok && id != "" {
			v["_owner_name"] = owners[id]
		}
		items = append(items, &vault.Item{Raw: v, Cipher: c.Cipher})
	}
	vault.SortedItems(items)
	return items, fold, org, nil
}
func (c *Client) encodePayload(raw map[string]any) ([]byte, error) {
	plain, err := vault.PlainMap(c.Cipher, raw)
	if err != nil {
		return nil, err
	}
	delete(plain, "_owner_name")
	b, err := json.Marshal(plain)
	if err != nil {
		return nil, &CLIError{"encode payload", "serialization", 0}
	}
	if c.BackendMode() == BackendBWServe {
		return b, nil
	}
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(b)))
	base64.StdEncoding.Encode(encoded, b)
	for i := range b {
		b[i] = 0
	}
	return encoded, nil
}
func (c *Client) template(ctx context.Context, kind string) (map[string]any, error) {
	// bw serve deliberately exposes CRUD rather than CLI template endpoints.
	// The established minimal templates preserve the existing payload contract.
	if c.BackendMode() == BackendBWServe {
		return fallbackTemplate(kind), nil
	}
	b, err := c.raw(ctx, "get", "template", kind)
	if err != nil {
		// A failed CLI command is not an unavailable template: propagate
		// authentication, network and other operational errors unchanged.
		return nil, err
	}
	template, parseErr := decodeObject(b, "bw get template "+kind)
	if parseErr != nil {
		// Some CLI installations return a non-JSON informational response
		// with exit code 0. Templates are optional; never abort staging here.
		return fallbackTemplate(kind), nil
	}
	return template, nil
}

// fallbackTemplate supplies a minimal CLI-compatible payload without a
// second command when the CLI's optional template is missing or malformed.
func fallbackTemplate(kind string) map[string]any {
	switch kind {
	case "folder":
		return map[string]any{"object": "folder", "name": ""}
	case "item":
		return map[string]any{
			"object": "item", "type": 1, "name": "", "favorite": false,
			"reprompt": 0, "organizationId": nil, "folderId": nil,
			"fields": []any{}, "collectionIds": []any{},
		}
	case "item.login":
		return map[string]any{"username": "", "password": "", "totp": nil, "uris": []any{}}
	case "item.securenote":
		return map[string]any{"type": 0}
	case "item.card", "item.identity":
		return map[string]any{}
	default:
		return map[string]any{}
	}
}

// Templates contain structures, not user values: discard example strings.
func scrubExampleStrings(values map[string]any) {
	for k, v := range values {
		switch x := v.(type) {
		case string:
			values[k] = ""
		case map[string]any:
			scrubExampleStrings(x)
		case []any:
			for j, item := range x {
				if _, ok := item.(string); ok {
					x[j] = ""
				} else if m, ok := item.(map[string]any); ok {
					scrubExampleStrings(m)
				}
			}
		}
	}
}
func mergeTemplate(dst, src map[string]any) {
	for k, v := range src {
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
}
func (c *Client) Save(ctx context.Context, ch *vault.Change) (map[string]any, error) {
	if ch == nil {
		return nil, errors.New("missing change")
	}
	entity := ch.Entity
	if entity != "item" && entity != "folder" {
		return nil, &CLIError{"bw save", "unsupported entity", 0}
	}
	if ch.Kind == vault.Deleted {
		if ch.Before == nil {
			return nil, nil
		}
		_, err := c.ses(ctx, "delete", entity, fmt.Sprint(ch.Before["id"]))
		return nil, err
	}
	if ch.After == nil {
		return nil, &CLIError{"bw save", "missing payload", 0}
	}
	payload := vault.Clone(ch.After)
	if ch.Kind == vault.New {
		delete(payload, "id")
		template, err := c.template(ctx, entity)
		if err != nil {
			return nil, err
		}
		mergeTemplate(payload, template)
		if entity == "item" {
			typ := intFrom(ch.After["type"])
			names := map[int]string{1: "item.login", 2: "item.securenote", 3: "item.card", 4: "item.identity"}
			parts := map[int]string{1: "login", 2: "secureNote", 3: "card", 4: "identity"}
			if name := names[typ]; name != "" {
				p, err := c.template(ctx, name)
				scrubExampleStrings(p)
				if err != nil {
					return nil, err
				}
				existing, _ := payload[parts[typ]].(map[string]any)
				if existing == nil {
					existing = map[string]any{}
				}
				mergeTemplate(existing, p)
				if typ == 2 {
					existing["type"] = 0
				}
				payload[parts[typ]] = existing
			}
		}
	}
	encoded, err := c.encodePayload(payload)
	if err != nil {
		return nil, err
	}
	var args []string
	op := ""
	if ch.Kind == vault.New {
		args = []string{"create", entity}
		op = "bw create " + entity
	} else {
		args = []string{"edit", entity, fmt.Sprint(ch.After["id"])}
		op = "bw edit " + entity
	}
	// stdin instead of argv: the base64 payload contains confidential values.
	b, err := c.call(ctx, op, args, encoded, nil, true)
	for i := range encoded {
		encoded[i] = 0
	}
	if err != nil {
		return nil, err
	}
	updated, err := decodeObject(b, op)
	if err != nil && ch.Kind == vault.Modified { // Do not retry an edit already accepted remotely.
		after, e := c.ses(ctx, "get", entity, fmt.Sprint(ch.After["id"]))
		if e != nil {
			return nil, &CLIError{op, "ambiguous successful response", 0}
		}
		updated, err = decodeObject(after, "bw get after edit")
	}
	if err != nil {
		return nil, &CLIError{op, "ambiguous successful response; verify remotely", 0}
	}
	if entity == "item" {
		if err = vault.SealItem(updated, c.Cipher); err != nil {
			return nil, err
		}
	}
	return updated, nil
}
func intFrom(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	default:
		return 0
	}
}

// ReadEnvCredentials reads export KEY=value lines without executing the file.
func ReadEnvCredentials(path string) (string, string, error) {
	if path == "" {
		return "", "", nil
	}
	if strings.HasPrefix(path, "~/") {
		h, _ := os.UserHomeDir()
		path = filepath.Join(h, path[2:])
	}
	path = os.ExpandEnv(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", &CLIError{"credentials file", "unreadable", 0}
	}
	vals := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		vals[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
	}
	return vals["BW_CLIENTID"], vals["BW_CLIENTSECRET"], nil
}
