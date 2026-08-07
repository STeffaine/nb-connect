package launcher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
	view := stripANSI(selector.View())
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
	if view = stripANSI(selector.View()); !strings.Contains(view, "Details: favorite: no | role: Application | tenant: Platform | status: planned") {
		t.Fatalf("view does not update selected details: %q", view)
	}
}

func TestModelResizesListToTerminalViewport(t *testing.T) {
	services := make([]netbox.Service, 0, 12)
	for index := range 12 {
		services = append(services, netbox.Service{Device: fmt.Sprintf("router-%02d", index), Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}})
	}
	selector, err := newModel(context.Background(), services, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.WindowSizeMsg{Width: 32, Height: 12})
	selector = updated.(model)
	if selector.width != 32 || selector.height != 12 {
		t.Fatalf("terminal size = %dx%d", selector.width, selector.height)
	}
	selector.cursor = 10
	start, end := selector.visibleRange(len(selector.visibleChoices()))
	if start == 0 || end-start != 2 {
		t.Fatalf("visible range = %d:%d", start, end)
	}
	if view := selector.View(); strings.Contains(view, "router-00") || !strings.Contains(view, "router-10") {
		t.Fatalf("resized view does not follow cursor: %q", view)
	}
}

func TestStatusStyleUsesSemanticColors(t *testing.T) {
	tests := []struct {
		status string
		style  string
	}{
		{status: "active", style: statusActiveStyle},
		{status: "planned", style: statusPendingStyle},
		{status: "offline", style: statusProblemStyle},
	}
	for _, test := range tests {
		if got := statusStyle(test.status); got != test.style {
			t.Errorf("statusStyle(%q) = %q, want %q", test.status, got, test.style)
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
	viewBeforePingOutput := stripANSI(selector.View())
	updated, _ = selector.Update(pingMessage{line: "64 bytes from 192.0.2.10"})
	selector = updated.(model)
	viewWithPingOutput := stripANSI(selector.View())
	if !selector.pinging || !strings.Contains(viewWithPingOutput, "64 bytes from 192.0.2.10") {
		t.Fatalf("live ping model = %#v", selector)
	}
	if !strings.Contains(viewWithPingOutput, "Ping in progress") {
		t.Fatalf("view does not show ping popup: %q", viewWithPingOutput)
	}
	if strings.Index(viewBeforePingOutput, "TARGET") != strings.Index(viewWithPingOutput, "TARGET") {
		t.Fatalf("ping output moved the service list: before=%q after=%q", viewBeforePingOutput, viewWithPingOutput)
	}
	updated, _ = selector.Update(pingMessage{done: true})
	selector = updated.(model)
	if selector.pinging {
		t.Fatalf("completed ping model = %#v", selector)
	}
	if view := stripANSI(selector.View()); !strings.Contains(view, "Ping results") {
		t.Fatalf("view does not retain ping results popup: %q", view)
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

func TestModelOrdersRecentsByMostRecentSelection(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}},
		{Device: "db-01", Name: "sshd", IPs: []string{"192.0.2.20"}, Ports: []int{22}},
		{Device: "jump-01", Name: "sshd", IPs: []string{"192.0.2.30"}, Ports: []int{22}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selector.recents = []string{
		"jump-01::sshd::192.0.2.30:22",
		"db-01::sshd::192.0.2.20:22",
	}

	visible := selector.visibleChoices()
	if got, want := visible[0].Service.TargetName(), "jump-01"; got != want {
		t.Fatalf("most recent choice = %q, want %q", got, want)
	}
	if got, want := visible[1].Service.TargetName(), "db-01"; got != want {
		t.Fatalf("second most recent choice = %q, want %q", got, want)
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

func TestModelSyncReportsUnavailableAndFailedSync(t *testing.T) {
	services := []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}
	selector, err := newModel(context.Background(), services, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, command := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	selector = updated.(model)
	if command != nil || selector.syncing || selector.syncError != "sync is unavailable" {
		t.Fatalf("unavailable sync = %#v, command=%v", selector, command)
	}

	selector.sync = func(context.Context) ([]netbox.Service, error) {
		return nil, errors.New("NetBox unavailable")
	}
	updated, command = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	selector = updated.(model)
	if !selector.syncing || command == nil {
		t.Fatalf("failed sync does not start = %#v, command=%v", selector, command)
	}
	updated, _ = selector.Update(command())
	selector = updated.(model)
	if selector.syncing || selector.syncError != "NetBox unavailable" {
		t.Fatalf("failed sync result = %#v", selector)
	}
}

func TestModelCancelsFromBrowseMode(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, command := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	selector = updated.(model)
	if !selector.cancelled || command == nil {
		t.Fatalf("browse cancellation = %#v, command=%v", selector, command)
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

func stripANSI(value string) string {
	return strings.NewReplacer(
		headingStyle, "",
		selectedRowStyle, "",
		statusActiveStyle, "",
		statusPendingStyle, "",
		statusProblemStyle, "",
		ansiReset, "",
	).Replace(value)
}
