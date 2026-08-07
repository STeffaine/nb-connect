package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steffaine/nb-connect/internal/cache"
	"github.com/steffaine/nb-connect/internal/config"
	"github.com/steffaine/nb-connect/internal/launcher"
	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestSyncAggregatesMultipleNetBoxServers(t *testing.T) {
	newServer := func(target string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/status/":
				writer.WriteHeader(http.StatusOK)
			case "/api/ipam/services/":
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"next":null,"results":[{"name":"sshd","protocol":"ssh","ports":[22],"ipaddresses":[{"address":"192.0.2.10/24"}],"device":{"name":"` + target + `"}}]}`))
			default:
				t.Errorf("unexpected request path %q", request.URL.Path)
				writer.WriteHeader(http.StatusNotFound)
			}
		}))
	}
	production := newServer("production-router")
	defer production.Close()
	lab := newServer("lab-router")
	defer lab.Close()

	credentials := config.Credentials{}
	credentials.NetBox.Servers = map[string]config.NetBoxServerCredentials{
		"production": {Token: "production-token"},
		"lab":        {Token: "lab-token"},
	}
	configuration := config.Config{NetBox: config.NetBoxConfig{Servers: []config.NetBoxServer{{Name: "production", URL: production.URL}, {Name: "lab", URL: lab.URL}}}, Services: config.ServicesConfig{Enabled: []string{"sshd"}}}
	cachePath := filepath.Join(t.TempDir(), "services.json")
	command := newRootCommand(dependencies{
		loadConfig:      func(string) (config.Config, error) { return configuration, nil },
		loadCredentials: func(string) (config.Credentials, error) { return credentials, nil },
		defaultConfig:   func() (string, error) { return "config.yaml", nil },
		defaultCache:    func() (string, error) { return cachePath, nil },
		newClient:       func(url, token string) (*netbox.Client, error) { return netbox.NewClient(url, token, nil) },
		now:             time.Now,
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"sync"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	snapshot, err := (cache.Store{Path: cachePath}).Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Services) != 2 {
		t.Fatalf("service count = %d, want 2", len(snapshot.Services))
	}
	if snapshot.Services[0].Server != "production" || snapshot.Services[1].Server != "lab" {
		t.Fatalf("server labels = %#v", snapshot.Services)
	}
}

func TestRunListReadsCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "services.json")
	services := []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10/32"}, Ports: []int{22}, Role: "Router", Tenant: "Operations", Status: "active"}}
	if err := (cache.Store{Path: cachePath}).Write(services, time.Now()); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := Run(context.Background(), []string{"--cache", cachePath, "list"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TARGET", "router-01", "sshd", "192.0.2.10:22", "Operations"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("list output %q does not contain %q", output.String(), want)
		}
	}
}

func TestRunSyncFetchesAndCachesServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Token example-token" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/api/status/":
			response.WriteHeader(http.StatusOK)
		case "/api/ipam/services/":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"next":null,"results":[{"name":"sshd","protocol":"tcp","ports":[22],"ipaddresses":[{"address":"192.0.2.10/32"}],"device":{"name":"router-01"}},{"name":"https","protocol":"tcp","ports":[443],"ipaddresses":[{"address":"192.0.2.11/32"}],"device":{"name":"netbox-01"}}]}`))
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	directory := t.TempDir()
	configPath := writeTestFile(t, directory, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: "+server.URL+"\nservices:\n  enabled:\n    - SSHD\n")
	credentialsPath := writeTestFile(t, directory, "credentials.yaml", "netbox:\n  servers:\n    production:\n      token: example-token\n")
	cachePath := filepath.Join(directory, "services.json")

	var output bytes.Buffer
	err := Run(context.Background(), []string{"--config", configPath, "--credentials", credentialsPath, "--cache", cachePath, "sync"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "Connected to NetBox") || !strings.Contains(got, "Found 1 services") {
		t.Fatalf("sync output = %q", got)
	}
	snapshot, err := (cache.Store{Path: cachePath}).Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Services) != 1 || snapshot.Services[0].Device != "router-01" || snapshot.Services[0].Name != "sshd" {
		t.Fatalf("cached services = %#v", snapshot.Services)
	}
}

func TestFilterServices(t *testing.T) {
	services := []netbox.Service{{Name: "sshd"}, {Name: "https"}, {Name: "telnet"}}
	filtered := filterServices(services, []string{" SSHD ", "TELNET"})
	if len(filtered) != 2 || filtered[0].Name != "sshd" || filtered[1].Name != "telnet" {
		t.Fatalf("filterServices() = %#v", filtered)
	}
	if got := filterServices(services, nil); len(got) != 0 {
		t.Fatalf("filterServices() with no enabled names = %#v", got)
	}
}

func TestFilterServicesByServer(t *testing.T) {
	services := []netbox.Service{{Server: "production", Device: "router-01"}, {Server: "lab", Device: "router-01"}}
	if got := filterServicesByServer(services, "LAB"); len(got) != 1 || got[0].Server != "lab" {
		t.Fatalf("filterServicesByServer() = %#v", got)
	}
	if got := filterServicesByServer(services, ""); len(got) != 2 {
		t.Fatalf("filterServicesByServer() without server = %#v", got)
	}
}

func TestRunConnectDryRunBuildsSSHCommand(t *testing.T) {
	directory := t.TempDir()
	cachePath := filepath.Join(directory, "services.json")
	services := []netbox.Service{{Device: "router-01", Name: "sshd", Protocol: "tcp", IPs: []string{"192.0.2.10/32"}, Ports: []int{22}}}
	if err := (cache.Store{Path: cachePath}).Write(services, time.Now()); err != nil {
		t.Fatal(err)
	}
	configPath := writeTestFile(t, directory, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nssh:\n  default_user: ops\n  keys:\n    ops:\n      identity_file: /home/ops/.ssh/id_ops\n")

	var output bytes.Buffer
	err := Run(context.Background(), []string{"--config", configPath, "--cache", cachePath, "connect", "--target", "router-01", "--service", "sshd", "--dry-run"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), `ssh "-i" "/home/ops/.ssh/id_ops" "ops@192.0.2.10" "-p" "22"`+"\n"; got != want {
		t.Fatalf("dry-run output = %q, want %q", got, want)
	}
}

func TestRunConnectDryRunBuildsTelnetCommand(t *testing.T) {
	directory := t.TempDir()
	cachePath := filepath.Join(directory, "services.json")
	services := []netbox.Service{{Device: "switch-01", Name: "telnet", Protocol: "tcp", IPs: []string{"192.0.2.11/32"}, Ports: []int{23}}}
	if err := (cache.Store{Path: cachePath}).Write(services, time.Now()); err != nil {
		t.Fatal(err)
	}
	configPath := writeTestFile(t, directory, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\n")

	var output bytes.Buffer
	err := Run(context.Background(), []string{"--config", configPath, "--cache", cachePath, "connect", "--target", "switch-01", "--service", "telnet", "--dry-run"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), `telnet "192.0.2.11" "23"`+"\n"; got != want {
		t.Fatalf("dry-run output = %q, want %q", got, want)
	}
}

func TestRootCommandRunsConnectByDefault(t *testing.T) {
	root := newRootCommand(dependencies{})
	if !root.SilenceUsage {
		t.Fatal("root command prints usage after an error")
	}
	connectCommand, _, err := root.Find([]string{"connect"})
	if err != nil {
		t.Fatal(err)
	}
	if root.RunE == nil {
		t.Fatal("root RunE is nil")
	}
	if fmt.Sprintf("%p", root.RunE) != fmt.Sprintf("%p", connectCommand.RunE) {
		t.Fatal("bare nbcon does not use the connect command handler")
	}
}

func TestSelectServiceRequiresBothExplicitSelectors(t *testing.T) {
	_, err := selectService(context.Background(), nil, "router-01", "", "", 4, nil)
	if err == nil || !strings.Contains(err.Error(), "must be used together") {
		t.Fatalf("selectService() error = %v", err)
	}
}

func TestSelectionCancellationIsRecognized(t *testing.T) {
	if err := ignoreSelectionCancellation(launcher.ErrSelectionCancelled); err != nil {
		t.Fatalf("cancellation error = %v, want nil", err)
	}
	other := errors.New("other failure")
	if err := ignoreSelectionCancellation(other); !errors.Is(err, other) {
		t.Fatalf("other error = %v, want %v", err, other)
	}
}

func writeTestFile(t *testing.T, directory, name, contents string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
