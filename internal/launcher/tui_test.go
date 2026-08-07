package launcher

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestModelSearchesFiltersAndSelectsService(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}},
		{Device: "netbox-01", Name: "https", IPs: []string{"192.0.2.20"}, Ports: []int{443}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	if !selector.searching || !selector.filter.Focused() {
		t.Fatalf("search mode = %#v", selector)
	}
	selector.filter.SetValue("router")
	if got := selector.visibleChoices(); len(got) != 1 || got[0].Service.TargetName() != "router-01" {
		t.Fatalf("visible choices = %#v", got)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selector = updated.(model)
	if selector.searching {
		t.Fatal("search mode remains active after Enter")
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selected := updated.(model).selection
	if selected == nil || selected.Endpoint != "192.0.2.10:22" {
		t.Fatalf("selection = %#v", selected)
	}
}

func TestModelEscClearsSearchBeforeCancelling(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selector.searching = true
	selector.filter.Focus()
	selector.filter.SetValue("router")
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyEsc})
	selector = updated.(model)
	if selector.searching || selector.filter.Value() != "" || selector.cancelled {
		t.Fatalf("Esc in search mode = %#v", selector)
	}
}

func TestModelFuzzySearchMatchesAbbreviatedServiceName(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{Device: "zammad-01", Name: "sshd", IPs: []string{"192.0.2.30"}, Ports: []int{22}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"zmmd", "zamad"} {
		selector.filter.SetValue(query)
		if got := selector.visibleChoices(); len(got) != 1 || got[0].Service.TargetName() != "zammad-01" {
			t.Fatalf("visible choices for %q = %#v", query, got)
		}
	}
}

func TestModelNumberShortcutSelectsVisibleService(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}},
		{Device: "netbox-01", Name: "sshd", IPs: []string{"192.0.2.20"}, Ports: []int{22}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selector.filter.SetValue("netbox")
	updated, command := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	selector = updated.(model)
	if selector.selection == nil || selector.selection.Service.TargetName() != "netbox-01" {
		t.Fatalf("selection = %#v", selector.selection)
	}
	if command == nil {
		t.Fatal("number shortcut does not quit")
	}
	updated, command = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")})
	selector = updated.(model)
	if selector.selection == nil || selector.selection.Service.TargetName() != "netbox-01" || command != nil {
		t.Fatalf("out-of-range shortcut changed selection = %#v", selector.selection)
	}
}

func TestModelViewShowsCompactAlignedColumnsAndSelectedDetails(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}, Role: "Router", Tenant: "Operations", Status: "active"},
		{Device: "application-server-very-long", Name: "https", IPs: []string{"192.0.2.20"}, Ports: []int{443}, Role: "Application", Tenant: "Platform", Status: "planned"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	view := selector.View()
	if !strings.Contains(view, "> 1   router-01") || !strings.Contains(view, "  2   application-server-very-long") {
		t.Fatalf("view does not show numbered rows: %q", view)
	}
	header := rowLine(t, view, "TARGET")
	shortRow := rowLine(t, view, "router-01")
	longRow := rowLine(t, view, "application-server-very-long")
	columnValues := map[string][2]string{
		"SERVICE":  {"sshd", "https"},
		"ENDPOINT": {"192.0.2.10:22", "192.0.2.20:443"},
	}
	for column, values := range columnValues {
		headerIndex := strings.Index(header, column)
		shortIndex := strings.Index(shortRow, values[0])
		longIndex := strings.Index(longRow, values[1])
		if headerIndex != shortIndex || headerIndex != longIndex {
			t.Fatalf("%s column offsets header=%d short=%d long=%d", column, headerIndex, shortIndex, longIndex)
		}
	}
	for _, unwanted := range []string{"ROLE", "TENANT", "STATUS"} {
		if strings.Contains(header, unwanted) {
			t.Fatalf("header unexpectedly contains %q: %q", unwanted, header)
		}
	}
	if !strings.Contains(view, "Details: favorite: no | role: Router | tenant: Operations | status: active") {
		t.Fatalf("view does not show selected details: %q", view)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyDown})
	selector = updated.(model)
	if view = selector.View(); !strings.Contains(view, "Details: favorite: no | role: Application | tenant: Platform | status: planned") {
		t.Fatalf("view does not update selected details: %q", view)
	}
}

func TestStatusStyleUsesSemanticColors(t *testing.T) {
	tests := []struct {
		status string
		color  lipgloss.Color
	}{
		{status: "active", color: lipgloss.Color("42")},
		{status: "planned", color: lipgloss.Color("214")},
		{status: "offline", color: lipgloss.Color("196")},
	}
	for _, test := range tests {
		if got := statusStyle(test.status).GetForeground(); got != test.color {
			t.Errorf("statusStyle(%q) foreground = %#v, want %#v", test.status, got, test.color)
		}
	}
}

func TestModelNavigatesToRecentsAndSelectsLastUsed(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}},
		{Device: "db-01", Name: "sshd", IPs: []string{"192.0.2.20"}, Ports: []int{22}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selector.recents = []string{"db-01::sshd::192.0.2.20:22"}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	selected := updated.(model).selection
	if selected == nil || selected.Service.TargetName() != "db-01" {
		t.Fatalf("last used selection = %#v", selected)
	}
}

func TestModelPingsSelectedEndpoint(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, command := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	if selector.selection != nil || !selector.pinging {
		t.Fatalf("ping model = %#v", selector)
	}
	if command == nil {
		t.Fatal("ping shortcut does not start")
	}
	updated, _ = selector.Update(pingMessage{line: "64 bytes from 192.0.2.10"})
	selector = updated.(model)
	if !selector.pinging || !strings.Contains(selector.View(), "64 bytes from 192.0.2.10") {
		t.Fatalf("live ping model = %#v", selector)
	}
	updated, _ = selector.Update(pingMessage{done: true})
	selector = updated.(model)
	if selector.pinging {
		t.Fatalf("completed ping model = %#v", selector)
	}
}

func TestModelPrioritizesFavoritesAndRecentsInView(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}},
		{Device: "db-01", Name: "sshd", IPs: []string{"192.0.2.20"}, Ports: []int{22}},
		{Device: "jump-01", Name: "sshd", IPs: []string{"192.0.2.30"}, Ports: []int{22}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selector.favorites = map[string]bool{"db-01::sshd::192.0.2.20:22": true}
	selector.recents = []string{"jump-01::sshd::192.0.2.30:22"}
	visible := selector.visibleChoices()
	if len(visible) != 3 {
		t.Fatalf("visible choices = %#v", visible)
	}
	if got := visible[0].Service.TargetName(); got != "db-01" {
		t.Fatalf("first visible choice = %q", got)
	}
	if got := visible[1].Service.TargetName(); got != "jump-01" {
		t.Fatalf("second visible choice = %q", got)
	}
	if got := visible[2].Service.TargetName(); got != "router-01" {
		t.Fatalf("third visible choice = %q", got)
	}
	view := selector.View()
	if !strings.Contains(view, "Favorites") || !strings.Contains(view, "Recents") {
		t.Fatalf("view does not show favorite/recent grouping: %q", view)
	}
	if !strings.Contains(view, "* db-01") || !strings.Contains(view, "Details: favorite: yes") {
		t.Fatalf("view does not mark the favorite: %q", view)
	}
	selector.cursor = 2
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyUp})
	selector = updated.(model)
	if got := selector.visibleChoices()[selector.cursor].Service.TargetName(); got != "jump-01" {
		t.Fatalf("up navigation selected %q, want recent jump-01", got)
	}
}

func TestModelSyncRefreshesChoices(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}, func(context.Context) ([]netbox.Service, error) {
		return []netbox.Service{{Device: "netbox-01", Name: "sshd", IPs: []string{"192.0.2.20"}, Ports: []int{22}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, command := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	selector = updated.(model)
	if !selector.syncing {
		t.Fatal("sync does not start")
	}
	updated, _ = selector.Update(command())
	selector = updated.(model)
	if selector.syncing || selector.syncError != "" || selector.choices[0].Service.TargetName() != "netbox-01" {
		t.Fatalf("sync result model = %#v", selector)
	}
	if !strings.Contains(selector.View(), "Synced 1 services") {
		t.Fatalf("view does not show sync confirmation: %q", selector.View())
	}
}

func rowLine(t *testing.T, view, contains string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, contains) {
			return line
		}
	}
	t.Fatalf("view %q does not contain row %q", view, contains)
	return ""
}
