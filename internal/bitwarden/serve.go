package bitwarden

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BackendMode identifies the transport used for Bitwarden operations.
type BackendMode string

const (
	BackendBW      BackendMode = "bw"
	BackendBWServe BackendMode = "bw serve"
)

const (
	serveStartupTimeout = 12 * time.Second
	serveProbeTimeout   = 2 * time.Second
	serveProbeInterval  = 100 * time.Millisecond
	serveShutdownWait   = 2 * time.Second
	serveHost           = "localhost:8087"
)

// BWServeBackend owns one private bw serve process and its Unix socket.
// It never discovers or connects to a process it did not create.
type BWServeBackend struct {
	Binary    string
	mu        sync.Mutex
	cmd       *exec.Cmd
	socketDir string
	socket    string
	profile   string
	dead      chan struct{}
	stopping  bool
}

func secureServeDirectory() (string, error) {
	base := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	info, err := os.Stat(base)
	// XDG_RUNTIME_DIR is usable only when it is a private directory. A private
	// child under /tmp is safer than trusting a shared or malformed XDG path.
	if base == "" || err != nil || !info.IsDir() || info.Mode().Perm()&007 != 0 {
		base = "/tmp"
		info, err = os.Stat(base)
	}
	if err != nil || !info.IsDir() {
		return "", errors.New("no secure runtime directory for bw serve")
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	dir := filepath.Join(base, "lazybw-"+strconv.Itoa(os.Getpid())+"-"+hex.EncodeToString(random[:]))
	// Linux sockaddr_un permits 108 bytes including the trailing NUL.
	if len(filepath.Join(dir, "bw.sock")) >= 104 && base != "/tmp" {
		dir = filepath.Join("/tmp", "lbw-"+hex.EncodeToString(random[:]))
	}
	if len(filepath.Join(dir, "bw.sock")) >= 104 {
		return "", errors.New("runtime directory path is too long for a Unix socket")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func (b *BWServeBackend) running() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cmd != nil && !b.stopping
}

func (b *BWServeBackend) socketPath() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.socket
}

func (b *BWServeBackend) Start(ctx context.Context, profile, session string) error {
	b.mu.Lock()
	if b.cmd != nil && !b.stopping && b.profile == profile {
		b.mu.Unlock()
		return nil
	}
	b.mu.Unlock()
	if err := b.Stop(context.Background()); err != nil {
		return err
	}
	dir, err := secureServeDirectory()
	if err != nil {
		return err
	}
	socket := filepath.Join(dir, "bw.sock")
	binary := b.Binary
	if binary == "" {
		binary = "bw"
	}
	cmd := exec.Command(binary, "serve", "--hostname", "unix://"+socket, "--port", "8087")
	cmd.Env = overriddenEnvironment(os.Environ(), []string{
		"BITWARDENCLI_APPDATA_DIR=" + profile,
		"BW_SESSION=" + session,
		"BW_NOINTERACTION=true",
	})
	// bw may include account details in diagnostics. Never retain or display them.
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return &CLIError{"bw serve", "execution", -1}
	}
	dead := make(chan struct{})
	b.mu.Lock()
	b.cmd, b.socketDir, b.socket, b.profile, b.dead, b.stopping = cmd, dir, socket, profile, dead, false
	b.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		b.mu.Lock()
		if b.cmd == cmd {
			b.cmd = nil
		}
		close(dead)
		b.mu.Unlock()
	}()

	deadline := time.NewTimer(serveStartupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(serveProbeInterval)
	defer ticker.Stop()
	for {
		probe, cancel := context.WithTimeout(ctx, serveProbeTimeout)
		_, err := b.request(probe, http.MethodGet, "/status", nil)
		cancel()
		if err == nil {
			_ = os.Chmod(socket, 0600)
			return nil
		}
		select {
		case <-ctx.Done():
			_ = b.Stop(context.Background())
			return ctx.Err()
		case <-dead:
			_ = b.Stop(context.Background())
			return &CLIError{"bw serve", "execution", -1}
		case <-deadline.C:
			_ = b.Stop(context.Background())
			return &CLIError{"bw serve", "startup timeout", -1}
		case <-ticker.C:
		}
	}
}

func (b *BWServeBackend) Stop(ctx context.Context) error {
	b.mu.Lock()
	cmd, dead, dir := b.cmd, b.dead, b.socketDir
	if cmd == nil {
		b.mu.Unlock()
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
		return nil
	}
	b.stopping = true
	b.mu.Unlock()
	_ = cmd.Process.Signal(os.Interrupt)
	timer := time.NewTimer(serveShutdownWait)
	defer timer.Stop()
	select {
	case <-dead:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
	case <-timer.C:
		_ = cmd.Process.Kill()
	}
	b.mu.Lock()
	if b.cmd == cmd {
		b.cmd, b.socket, b.socketDir, b.profile, b.stopping = nil, "", "", "", false
	}
	b.mu.Unlock()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	return nil
}

type serveResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (b *BWServeBackend) request(ctx context.Context, method, path string, body any) ([]byte, error) {
	socket := b.socketPath()
	if socket == "" {
		return nil, errors.New("bw serve is not running")
	}
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		input = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, input)
	if err != nil {
		return nil, err
	}
	// Current bw releases validate Host even on Unix sockets. Origin must stay absent.
	req.Host = serveHost
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return nil, &CLIError{"bw serve", "network", -1}
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return nil, &CLIError{"bw serve", "network", -1}
	}
	var envelope serveResponse
	if err := json.Unmarshal(payload, &envelope); err != nil || !envelope.Success {
		for i := range payload {
			payload[i] = 0
		}
		kind := "unknown"
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			kind = "authentication"
		}
		return nil, &CLIError{"bw serve", kind, response.StatusCode}
	}
	if len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
		for i := range payload {
			payload[i] = 0
		}
		return []byte("null"), nil
	}
	data := append([]byte(nil), envelope.Data...)
	for i := range payload {
		payload[i] = 0
	}
	return data, nil
}

func (b *BWServeBackend) call(ctx context.Context, args []string, body any) ([]byte, error) {
	if len(args) == 0 {
		return nil, &CLIError{"bw serve", "unsupported operation", 0}
	}
	method, path := "", ""
	switch args[0] {
	case "status":
		method, path = http.MethodGet, "/status"
	case "sync":
		method, path = http.MethodPost, "/sync"
	case "lock":
		method, path = http.MethodPost, "/lock"
	case "list":
		if len(args) == 2 {
			method, path = http.MethodGet, "/list/object/"+args[1]
		}
	case "get":
		if len(args) == 3 {
			method, path = http.MethodGet, "/object/"+args[1]+"/"+args[2]
		}
	case "create":
		if len(args) == 2 {
			method, path = http.MethodPost, "/object/"+args[1]
		}
	case "edit":
		if len(args) == 3 {
			method, path = http.MethodPut, "/object/"+args[1]+"/"+args[2]
		}
	case "delete":
		if len(args) == 3 {
			method, path = http.MethodDelete, "/object/"+args[1]+"/"+args[2]
		}
	}
	if method == "" {
		return nil, &CLIError{"bw " + strings.Join(args, " "), "unsupported by bw serve", 0}
	}
	response, err := b.request(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	return normalizeServeResponse(args, response)
}

// normalizeServeResponse adapts bw serve's typed HTTP response data to the
// JSON values returned by traditional CLI commands. In particular, list data
// is wrapped as {"object":"list","data":[...]}, unlike `bw list` output.
func normalizeServeResponse(args []string, payload []byte) ([]byte, error) {
	if len(args) == 0 || args[0] != "list" {
		return payload, nil
	}
	var list struct {
		Object string          `json:"object"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &list); err != nil || list.Object != "list" || len(list.Data) == 0 || bytes.Equal(list.Data, []byte("null")) {
		for i := range payload {
			payload[i] = 0
		}
		return nil, &CLIError{"bw " + strings.Join(args, " "), "invalid JSON response", 0}
	}
	var values []json.RawMessage
	if err := json.Unmarshal(list.Data, &values); err != nil || values == nil {
		for i := range payload {
			payload[i] = 0
		}
		return nil, &CLIError{"bw " + strings.Join(args, " "), "invalid JSON response", 0}
	}
	result := append([]byte(nil), list.Data...)
	for i := range payload {
		payload[i] = 0
	}
	return result, nil
}

func parseServeVersion(value string) ([3]int, bool) {
	var result [3]int
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return result, false
	}
	parts := strings.Split(fields[0], ".")
	if len(parts) < 3 {
		return result, false
	}
	for i := 0; i < 3; i++ {
		part := strings.TrimFunc(parts[i], func(r rune) bool { return r < '0' || r > '9' })
		n, err := strconv.Atoi(part)
		if err != nil {
			return result, false
		}
		result[i] = n
	}
	return result, true
}

func serveVersionSupported(value string) bool {
	v, ok := parseServeVersion(value)
	return ok && (v[0] > 2025 || v[0] == 2025 && (v[1] > 9 || v[1] == 9 && v[2] >= 0))
}
