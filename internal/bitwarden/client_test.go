package bitwarden

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/miguelmnts/lazybw-go/internal/vault"
	"strings"
	"testing"
)

type mock struct {
	calls [][]string
	input [][]byte
}

func (m *mock) Run(_ context.Context, args []string, in []byte, _ []string) ([]byte, []byte, int, error) {
	m.calls = append(m.calls, append([]string(nil), args...))
	m.input = append(m.input, append([]byte(nil), in...))
	switch strings.Join(args, " ") {
	case "get template item":
		return []byte(`{"type":1,"name":"","login":null}`), nil, 0, nil
	case "get template item.login":
		return []byte(`{"username":"EXAMPLE","password":""}`), nil, 0, nil
	case "create item":
		return []byte(`{"id":"new-uuid","type":1,"name":"Hello","login":{"password":"pass"}}`), nil, 0, nil
	case "edit item id":
		return []byte(`{"id":"id","type":1,"name":"Hello","login":{"password":"pass"}}`), nil, 0, nil
	default:
		return []byte(`[]`), nil, 0, nil
	}
}
func TestCreateUsesStdinAndEncryptsResponse(t *testing.T) {
	m := &mock{}
	client, _ := NewClient(m)
	after := vault.NewItem(1, "Hello", vault.LocalID(), client.Cipher)
	_ = after.Set([]any{"login", "password"}, "pass")
	ch := &vault.Change{Entity: "item", Kind: vault.New, After: after.Raw}
	v, err := client.Save(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if v["id"] != "new-uuid" {
		t.Fatal(v)
	}
	in := m.input[len(m.input)-1]
	raw, err := base64.StdEncoding.DecodeString(string(in))
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	if data["login"].(map[string]any)["password"] != "pass" {
		t.Fatal("secret not serialized")
	}
	if _, ok := v["login"].(map[string]any)["password"].(*vault.Sealed); !ok {
		t.Fatal("server response must be encrypted")
	}
}
func TestSanitizedError(t *testing.T) {
	e := (&CLIError{"bw save", "validation", 1}).Error()
	if strings.Contains(e, "password") {
		t.Fatal(e)
	}
}

func TestEnvironmentOverridesRemoveInheritedSecrets(t *testing.T) {
	environment := overriddenEnvironment(
		[]string{"PATH=/bin", "BW_SESSION=foreign", "LAZYBW_MASTER_PASSWORD=stale"},
		[]string{"BW_SESSION=", "LAZYBW_MASTER_PASSWORD=current"},
	)
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "BW_SESSION=foreign") ||
		strings.Contains(joined, "LAZYBW_MASTER_PASSWORD=stale") {
		t.Fatalf("inherited secret survived override: %v", environment)
	}
	if !strings.Contains(joined, "PATH=/bin") ||
		!strings.Contains(joined, "LAZYBW_MASTER_PASSWORD=current") {
		t.Fatalf("expected environment entries are missing: %v", environment)
	}
}
