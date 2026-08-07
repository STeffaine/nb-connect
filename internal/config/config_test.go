package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	path := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: \" https://netbox.example.test/ \"\nservices:\n  enabled: [sshd, https]\nssh:\n  default_user: ops\n  keys:\n    ops:\n      identity_file: /home/ops/.ssh/id_ops\ncache:\n  ttl: 30m\nping:\n  count: 2\n")

	configuration, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := configuration.NetBox.Servers, []NetBoxServer{{Name: "production", URL: "https://netbox.example.test"}}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("Servers = %#v, want %#v", got, want)
	}
	if configuration.SSH.DefaultUser != "ops" {
		t.Fatalf("DefaultUser = %q", configuration.SSH.DefaultUser)
	}
	if got := configuration.SSH.Keys["ops"].IdentityFile; got != "/home/ops/.ssh/id_ops" {
		t.Fatalf("identity file = %q", got)
	}
	if configuration.Cache.TTL != 30*time.Minute {
		t.Fatalf("TTL = %s", configuration.Cache.TTL)
	}
	if configuration.Ping.Count != 2 {
		t.Fatalf("Ping.Count = %d", configuration.Ping.Count)
	}
	if strings.Join(configuration.Services.Enabled, ",") != "sshd,https" {
		t.Fatalf("Enabled = %v", configuration.Services.Enabled)
	}
}

func TestLoadUsesDefaultTTL(t *testing.T) {
	path := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: http://netbox.example.test\n")
	configuration, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := configuration.NetBox.Servers, []NetBoxServer{{Name: "production", URL: "http://netbox.example.test"}}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("Servers = %#v, want %#v", got, want)
	}
	if configuration.Cache.TTL != 15*time.Minute {
		t.Fatalf("TTL = %s", configuration.Cache.TTL)
	}
	if configuration.Ping.Count != 4 {
		t.Fatalf("Ping.Count = %d", configuration.Ping.Count)
	}
}

func TestLoadMultipleNetBoxServers(t *testing.T) {
	path := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test/\n    - name: lab\n      url: http://netbox.lab.test\n")
	configuration, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := configuration.NetBox.Servers, []NetBoxServer{{Name: "production", URL: "https://netbox.example.test"}, {Name: "lab", URL: "http://netbox.lab.test"}}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Servers = %#v, want %#v", got, want)
	}
}

func TestLoadExpandsSSHIdentityHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nssh:\n  keys:\n    ops:\n      identity_file: ~/.ssh/id_ops\n")
	configuration, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := configuration.SSH.Keys["ops"].IdentityFile, filepath.Join(home, ".ssh", "id_ops"); got != want {
		t.Fatalf("identity file = %q, want %q", got, want)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	testCases := []struct {
		name     string
		contents string
		want     string
	}{
		{name: "missing servers", contents: "ssh:\n  default_user: ops\n", want: "netbox.servers is required"},
		{name: "invalid URL scheme", contents: "netbox:\n  servers:\n    - name: production\n      url: netbox.example.test\n", want: "netbox.servers[0].url must start"},
		{name: "invalid TTL", contents: "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\ncache:\n  ttl: tomorrow\n", want: "parse cache.ttl"},
		{name: "invalid ping count", contents: "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nping:\n  count: -1\n", want: "ping.count must be greater than zero"},
		{name: "invalid YAML", contents: "netbox: [\n", want: "parse configuration"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Load(writeFile(t, "config.yaml", testCase.contents))
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Load() error = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func TestLoadRejectsLegacyNetBoxURL(t *testing.T) {
	_, err := Load(writeFile(t, "config.yaml", "netbox:\n  url: https://netbox.example.test\n"))
	if err == nil || !strings.Contains(err.Error(), "netbox.servers is required") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadCredentials(t *testing.T) {
	path := writeFile(t, "credentials.yaml", "netbox:\n  servers:\n    production:\n      token: example-token\n")
	credentials, err := LoadCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := credentials.TokenFor("production"); err != nil || got != "example-token" {
		t.Fatalf("TokenFor() = %q, %v", got, err)
	}
}

func TestCredentialsTokenForServer(t *testing.T) {
	path := writeFile(t, "credentials.yaml", "netbox:\n  servers:\n    production:\n      token: production-token\n    lab:\n      token: lab-token\n")
	credentials, err := LoadCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := credentials.TokenFor("PRODUCTION"); err != nil || got != "production-token" {
		t.Fatalf("TokenFor() = %q, %v", got, err)
	}
	if _, err := credentials.TokenFor("missing"); err == nil || !strings.Contains(err.Error(), "netbox.servers.missing.token") {
		t.Fatalf("TokenFor() error = %v", err)
	}
}

func TestLoadCredentialsRejectsMissingToken(t *testing.T) {
	path := writeFile(t, "credentials.yaml", "netbox:\n")
	_, err := LoadCredentials(path)
	if err == nil || !strings.Contains(err.Error(), "netbox.servers is required") {
		t.Fatalf("LoadCredentials() error = %v", err)
	}
}

func TestLoadCredentialsRejectsLegacyNetBoxToken(t *testing.T) {
	_, err := LoadCredentials(writeFile(t, "credentials.yaml", "netbox:\n  token: example-token\n"))
	if err == nil || !strings.Contains(err.Error(), "netbox.servers is required") {
		t.Fatalf("LoadCredentials() error = %v", err)
	}
}

func TestDefaultPaths(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	configPath, err := DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	wantConfigPath := filepath.Join(configRoot, "nb-connect", "config.yaml")
	if configPath != wantConfigPath {
		t.Fatalf("DefaultConfigPath() = %q, want %q", configPath, wantConfigPath)
	}
	if got, want := DefaultCredentialsPath(configPath), filepath.Join(configRoot, "nb-connect", "credentials.yaml"); got != want {
		t.Fatalf("DefaultCredentialsPath() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
