package launcher

import (
	"context"
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