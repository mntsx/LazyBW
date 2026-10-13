package bitwarden

import (
	"context"
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

type unreliableTemplateRunner struct{ createCalled bool }

func (r *unreliableTemplateRunner) Run(_ context.Context, args []string, in []byte, _ []string) ([]byte, []byte, int, error) {
	switch strings.Join(args, " ") {
	case "get template item", "get template item.login":
		return []byte("i template information, not JSON"), nil, 0, nil
	case "create item":
		r.createCalled = true
		if len(in) == 0 {
			return nil, []byte("missing input"), 1, nil
		}
		return []byte(`{"id":"remote-id","type":1,"name":"Test"}`), nil, 0, nil
	default:
		return nil, nil, 0, nil
	}
}
func TestSaveToleratesNonJSONTemplates(t *testing.T) {
	r := &unreliableTemplateRunner{}
	c, _ := NewClient(r)
	it := vault.NewItem(vault.Login, "Test", vault.LocalID(), c.Cipher)
	remote, err := c.Save(context.Background(), &vault.Change{Entity: "item", Kind: vault.New, After: it.Raw})
	if err != nil {
		t.Fatal(err)
	}
	if !r.createCalled || remote["id"] != "remote-id" {
		t.Fatalf("save did not reach create: %#v", remote)
	}
}
func TestFriendlySafeAuthenticationMessages(t *testing.T) {
	cases := []struct {
		err    error
		master bool
		want   string
	}{
		{&CLIError{"bw unlock", "unknown", 1}, true, "Incorrect master password"},
		{&CLIError{"bw login --apikey", "authentication", 1}, false, "not authenticated"},
		{&CLIError{"bw unlock", "network", 1}, true, "Connection failed"},
		{&CLIError{"bw login --apikey", "execution", -1}, false, "could not be started"},
	}
	for _, tc := range cases {
		got := AuthenticationMessage(tc.err, tc.master)
		if !strings.Contains(got, tc.want) {
			t.Errorf("got %q want %q", got, tc.want)
		}
		if strings.Contains(got, "bw unlock") {
			t.Fatalf("technical error escaped: %q", got)
		}
	}
}
