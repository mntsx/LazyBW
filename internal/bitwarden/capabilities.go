package bitwarden

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const capabilityCacheFile = "bw-capabilities.json"
const capabilityHashLimit = 8 << 20

type executableFingerprint struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time_ns"`
	Mode    uint32 `json:"mode"`
	Hash    string `json:"hash,omitempty"`
}

type capabilityCache struct {
	Version        string                 `json:"version"`
	ServeChecked   bool                   `json:"serve_checked"`
	ServeSupported bool                   `json:"serve_supported"`
	UnixSockets    bool                   `json:"unix_sockets"`
	Executable     executableFingerprint  `json:"executable"`
	Interpreter    *executableFingerprint `json:"interpreter,omitempty"`
}

func cacheHome() (string, error) {
	if value := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); value != "" {
		return filepath.Join(value, "lazybw"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "lazybw"), nil
}

func executableBinary(r Runner) (string, bool) {
	switch value := r.(type) {
	case ExecRunner:
		if value.Binary != "" {
			return value.Binary, true
		}
		return "bw", true
	case *ExecRunner:
		if value == nil {
			return "", false
		}
		if value.Binary != "" {
			return value.Binary, true
		}
		return "bw", true
	default:
		// Test and embedding runners do not necessarily execute a local bw binary.
		return "", false
	}
}

func fingerprintExecutable(binary string) (executableFingerprint, *executableFingerprint, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return executableFingerprint{}, nil, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return executableFingerprint{}, nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return executableFingerprint{}, nil, err
	}
	fingerprint, err := fingerprintFile(path)
	if err != nil {
		return executableFingerprint{}, nil, err
	}
	interpreter, _ := scriptInterpreter(path)
	return fingerprint, interpreter, nil
}

func fingerprintFile(path string) (executableFingerprint, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = os.ErrInvalid
		}
		return executableFingerprint{}, err
	}
	result := executableFingerprint{
		Path: path, Size: info.Size(), ModTime: info.ModTime().UnixNano(), Mode: uint32(info.Mode()),
	}
	// Hash scripts and small wrappers. Large native executables are identified by
	// their resolved path and immutable installation metadata to avoid a more
	// expensive startup check than the CLI process this cache replaces.
	if info.Size() <= capabilityHashLimit {
		file, err := os.Open(path)
		if err != nil {
			return executableFingerprint{}, err
		}
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			return executableFingerprint{}, err
		}
		result.Hash = hex.EncodeToString(hash.Sum(nil))
	}
	return result, nil
}

func scriptInterpreter(path string) (*executableFingerprint, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	line := make([]byte, 512)
	n, _ := file.Read(line)
	line = line[:n]
	if !strings.HasPrefix(string(line), "#!") {
		return nil, nil
	}
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(line), "\n", 2)[0], "#!")))
	if len(fields) == 0 {
		return nil, nil
	}
	binary := fields[0]
	if filepath.Base(binary) == "env" && len(fields) > 1 {
		binary = fields[1]
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return nil, err
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return nil, err
	}
	fingerprint, err := fingerprintFile(resolved)
	if err != nil {
		return nil, err
	}
	return &fingerprint, nil
}

func sameFingerprint(left, right executableFingerprint) bool {
	return left.Path == right.Path && left.Size == right.Size && left.ModTime == right.ModTime &&
		left.Mode == right.Mode && left.Hash == right.Hash
}

func (c *Client) loadPersistentCapabilities() {
	binary, ok := executableBinary(c.runner)
	if !ok {
		return
	}
	executable, interpreter, err := fingerprintExecutable(binary)
	if err != nil {
		return
	}
	dir, err := cacheHome()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(dir, capabilityCacheFile))
	if err != nil {
		return
	}
	var cached capabilityCache
	if json.Unmarshal(data, &cached) != nil || cached.Version == "" || !sameFingerprint(cached.Executable, executable) {
		return
	}
	if (cached.Interpreter == nil) != (interpreter == nil) ||
		cached.Interpreter != nil && !sameFingerprint(*cached.Interpreter, *interpreter) {
		return
	}
	c.version, c.versionReady = cached.Version, true
	if cached.ServeChecked {
		c.serveHostname, c.serveHelpReady = cached.UnixSockets, true
	}
}

func (c *Client) savePersistentCapabilities() {
	binary, ok := executableBinary(c.runner)
	if !ok {
		return
	}
	executable, interpreter, err := fingerprintExecutable(binary)
	if err != nil {
		return
	}
	c.mu.Lock()
	cached := capabilityCache{
		Version: c.version, ServeChecked: c.serveHelpReady,
		ServeSupported: c.versionReady && serveVersionSupported(c.version) && c.serveHostname,
		UnixSockets:    c.serveHostname, Executable: executable, Interpreter: interpreter,
	}
	ready := c.versionReady
	c.mu.Unlock()
	if !ready {
		return
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return
	}
	dir, err := cacheHome()
	if err != nil || os.MkdirAll(dir, 0700) != nil {
		return
	}
	_ = os.Chmod(dir, 0700)
	path := filepath.Join(dir, capabilityCacheFile)
	if current, err := os.ReadFile(path); err == nil && string(current) == string(data) {
		return
	}
	file, err := os.CreateTemp(dir, ".bw-capabilities-")
	if err != nil {
		return
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return
	}
	if _, err := file.Write(data); err != nil || file.Close() != nil {
		return
	}
	_ = os.Rename(temporary, path)
}
