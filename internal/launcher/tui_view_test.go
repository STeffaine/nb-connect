package launcher

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestSearchInputEditsUnicodeRunes(t *testing.T) {
	input := searchInput{value: "rout\u00e9"}
	updated, _ := input.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got, want := updated.Value(), "rout"; got != want {
		t.Fatalf("backspace value = %q, want %q", got, want)
	}
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\u00e9")})
	if got, want := updated.Value(), "rout\u00e9"; got != want {
		t.Fatalf("typed value = %q, want %q", got, want)
	}
}

func TestPingPopupRespectsTerminalWidth(t *testing.T) {
	model := model{width: 16, pinging: true, pingNote: "a very long ping result"}
	view := model.View()
	for _, line := range strings.Split(view, "\n") {
		if (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "|")) && len(line) > 16 {
			t.Fatalf("line exceeds terminal width: %q", line)
		}
	}
}

func TestServiceViewShowsServerColumnOnlyForMultipleServers(t *testing.T) {
	newSelector := func(services []netbox.Service) model {
		t.Helper()
		selector, err := newModel(context.Background(), services, nil)
		if err != nil {
			t.Fatal(err)
		}
		return selector
	}
	productionServices := []netbox.Service{{Server: "production", Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}
	if view := stripANSI(newSelector(productionServices).View()); strings.Contains(rowLine(t, view, "TARGET"), "SERVER") {
		t.Fatalf("single-server header contains SERVER: %q", view)
	}

	services := append(productionServices, netbox.Service{Server: "lab", Device: "router-02", Name: "sshd", IPs: []string{"192.0.2.11"}, Ports: []int{22}})
	view := stripANSI(newSelector(services).View())
	header := rowLine(t, view, "TARGET")
	row := rowLine(t, view, "router-01")
	if strings.Index(header, "SERVER") < strings.Index(header, "ENDPOINT") {
		t.Fatalf("server column is not rightmost: %q", header)
	}
	if strings.Index(row, "production") < strings.Index(row, "192.0.2.10:22") {
		t.Fatalf("server value is not rightmost: %q", row)
	}
}
