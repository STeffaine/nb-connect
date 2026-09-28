package launcher

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestFilterMenuSearchesSelectsAndMatchesAllOrAny(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{Server: "production", Device: "vpn-a", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}, Site: "site-a", Tenant: "tenant-1", Role: "vpn-server"},
		{Server: "lab", Device: "vpn-b", Name: "sshd", IPs: []string{"192.0.2.11"}, Ports: []int{22}, Site: "site-b", Tenant: "tenant-2", Role: "vpn-server"},
		{Server: "production", Device: "web-a", Name: "sshd", IPs: []string{"192.0.2.12"}, Ports: []int{22}, Site: "site-a", Tenant: "tenant-2", Role: "web"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	if !selector.filtering || selector.filterMenuSearch.Focused() {
		t.Fatalf("filter menu = %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selector = updated.(model)
	if selector.filterCategory != 1 || selector.filterCursor != 0 || selector.filterMenuSearch.Value() != "" {
		t.Fatalf("j navigation = %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	selector = updated.(model)
	if selector.filterCategory != 0 {
		t.Fatalf("k navigation = %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyTab})
	selector = updated.(model)
	if !selector.filterOptionsFocused {
		t.Fatalf("option focus = %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selector = updated.(model)
	if selector.filterCursor != 1 || selector.filterCategory != 0 {
		t.Fatalf("option navigation = %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyTab})
	selector = updated.(model)
	if selector.filterOptionsFocused {
		t.Fatalf("category focus = %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	selector = updated.(model)
	if !selector.filterSearching || !selector.filterMenuSearch.Focused() {
		t.Fatalf("filter search mode = %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("production")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selector = updated.(model)
	if selector.filterSearching {
		t.Fatalf("filter search mode remains active after Enter: %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyTab})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	selector = updated.(model)
	selector.filterMenuSearch.SetValue("site-a")
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyTab})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	selector = updated.(model)
	selector.filterMenuSearch.SetValue("vpn-server")
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selector = updated.(model)

	if got := selector.visibleChoices(); len(got) != 1 || got[0].Service.TargetName() != "vpn-a" {
		t.Fatalf("all-condition matches = %#v", got)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	selector = updated.(model)
	if got := selector.visibleChoices(); len(got) != 3 {
		t.Fatalf("any-condition matches = %#v", got)
	}
}

func TestFilterMenuDClearsAllFilters(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{Server: "production", Device: "vpn-a", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}, Site: "site-a", Tenant: "tenant-1", Role: "vpn-server"},
		{Server: "lab", Device: "vpn-b", Name: "sshd", IPs: []string{"192.0.2.11"}, Ports: []int{22}, Site: "site-b", Tenant: "tenant-2", Role: "vpn-server"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	press := func(key string) {
		t.Helper()
		updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		selector = updated.(model)
	}

	press("f")
	press("tab")
	press(" ")
	press("tab")
	press("j")
	press("tab")
	press(" ")
	if got := len(selector.filters.values); got != 2 {
		t.Fatalf("selected filter categories = %d, want 2", got)
	}
	if got := selector.visibleChoices(); len(got) != 0 {
		t.Fatalf("filtered choices = %d, want 0", len(got))
	}
	selector.activePreset = "stale preset"
	press("d")
	if len(selector.filters.values) != 0 {
		t.Fatalf("filters after d = %#v, want empty", selector.filters.values)
	}
	if selector.activePreset != "" {
		t.Fatalf("active preset after d = %q, want empty", selector.activePreset)
	}
	if got := selector.visibleChoices(); len(got) != 2 {
		t.Fatalf("visible choices after d = %d, want 2", len(got))
	}
	if view := selector.View(); !strings.Contains(view, "d clear") {
		t.Fatalf("filter menu footer = %q, want d clear hint", view)
	}
}

func TestInitialServerFilterAppliesToModel(t *testing.T) {
	services := []netbox.Service{
		{Server: "server-a", Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}},
		{Server: "server-b", Device: "router-02", Name: "sshd", IPs: []string{"192.0.2.11"}, Ports: []int{22}},
		{Server: "server-a", Device: "switch-01", Name: "telnet", IPs: []string{"192.0.2.12"}, Ports: []int{23}},
	}
	selector, err := newModelWithServerFilter(context.Background(), services, "SERVER-A", nil)
	if err != nil {
		t.Fatal(err)
	}

	if !selector.filters.values[filterServer]["server-a"] {
		t.Fatalf("server filter not applied: %#v", selector.filters)
	}

	visible := selector.visibleChoices()
	if len(visible) != 2 {
		t.Fatalf("visible choices count = %d, want 2", len(visible))
	}
	for _, choice := range visible {
		if choice.Service.Server != "server-a" {
			t.Fatalf("unexpected choice server = %q, want %q", choice.Service.Server, "server-a")
		}
	}

	// Toggle off the server-a filter in the filter menu
	selector.filters.toggle(filterServer, "server-a")
	if len(selector.visibleChoices()) != 3 {
		t.Fatalf("visible choices count after untoggling = %d, want 3", len(selector.visibleChoices()))
	}
}