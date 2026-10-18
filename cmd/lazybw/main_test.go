package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/config"
)

func replicateConfig() config.Config {
	var cfg config.Config
	cfg.Local.VaultPath = "/tmp/bw-vault"
	cfg.Authentication.ClientEnvPath = "/tmp/client.env"
	return cfg
}

func TestParseReplicateUsesConfigAndPromptsForPassword(t *testing.T) {
	options, prompt, err := parseReplicate(nil, replicateConfig(), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if options.Path != "/tmp/bw-vault" || options.ClientPath != "/tmp/client.env" {
		t.Fatalf("settings were not used: %#v", options)
	}
	if !prompt || options.Password != "" {
		t.Fatal("omitted password must use the hidden prompt")
	}
}

func TestParseReplicateAcceptsAllOverrides(t *testing.T) {
	args := []string{"--path=/tmp/replica", "--client=/tmp/account.env", "--password=secret"}
	options, prompt, err := parseReplicate(args, replicateConfig(), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if prompt || options.Path != "/tmp/replica" || options.ClientPath != "/tmp/account.env" ||
		options.Password != "secret" || !options.ClientOverride {
		t.Fatalf("overrides were not parsed: %#v, prompt=%v", options, prompt)
	}
}

func TestParseReplicateRejectsInvalidOverrideFiles(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"path extension", []string{"--path=/tmp/vault.enc"}, "must not have a file extension"},
		{"client extension", []string{"--client=/tmp/client.txt"}, "must point to a .env file"},
		{"empty path", []string{"--path="}, "--path cannot be empty"},
		{"empty client", []string{"--client="}, "--client cannot be empty"},
		{"empty password", []string{"--password="}, "--password cannot be empty"},
		{"positional", []string{"extra"}, "does not accept positional arguments"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := parseReplicate(test.args, replicateConfig(), &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v; want %q", err, test.want)
			}
		})
	}
}
