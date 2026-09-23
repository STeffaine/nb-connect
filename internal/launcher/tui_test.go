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
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
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

func TestModelEscCancelsFromSearchMode(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selector.searching = true
	selector.filter.Focus()
	selector.filter.SetValue("router")
	updated, command := selector.Update(tea.KeyMsg{Type: tea.KeyEsc})
	selector = updated.(model)
	if !selector.cancelled || command == nil {
		t.Fatalf("Esc in search mode should cancel = %#v, command=%v", selector, command)
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

func TestModelSearchMatchesDescriptionAndRole(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{Device: "edge-01", Name: "sshd", IPs: []string{"192.0.2.31"}, Ports: []int{22}, Role: "edge-router", Description: "primary transit gateway"},
		{Device: "app-01", Name: "https", IPs: []string{"192.0.2.32"}, Ports: []int{443}, Role: "application", Description: "frontend"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	selector.filter.SetValue("transit")
	if got := selector.visibleChoices(); len(got) != 1 || got[0].Service.TargetName() != "edge-01" {
		t.Fatalf("description search visible choices = %#v", got)
	}

	selector.filter.SetValue("edge-router")
	if got := selector.visibleChoices(); len(got) != 1 || got[0].Service.TargetName() != "edge-01" {
		t.Fatalf("role search visible choices = %#v", got)
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
		{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}, Role: "Router", Tenant: "Operations", Status: "active", Description: "Edge router"},
		{Device: "application-server-very-long", Name: "https", IPs: []string{"192.0.2.20"}, Ports: []int{443}, Role: "Application", Tenant: "Platform", Status: "planned", Description: "Frontend service"},
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
	if !strings.Contains(view, "Details: favorite: no | role: Router | tenant: Operations | status: active | description: Edge router") {
		t.Fatalf("view does not show selected details: %q", view)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyDown})
	selector = updated.(model)
	if view = stripANSI(selector.View()); !strings.Contains(view, "Details: favorite: no | role: Application | tenant: Platform | status: planned | description: Frontend service") {
		t.Fatalf("view does not update selected details: %q", view)
	}
}

func TestModelOpensInfoPanelByDefaultWhenWideEnough(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{
		Server: "production",
		Device: "router-01",
		VM: "router-vm-01",
		Name: "sshd",
		Protocol: "tcp",
		Ports: []int{22, 2222},
		IPs: []string{"192.0.2.10", "192.0.2.11"},
		Site: "dc1",
		Role: "Router",
		Tenant: "Operations",
		Platform: "Linux",
		Tags: []string{"core", "edge"},
		Description: "Edge router",
		Status: "active",
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	selector = updated.(model)
	if !selector.infoOpen || selector.infoOverlay {
		t.Fatalf("wide viewport should open docked info panel: %#v", selector)
	}
	view := stripANSI(selector.View())
	for _, want := range []string{"TARGET", "Service information", "server: production", "target: router-01", "ports: 22, 2222", "favorite: no"} {
		if !strings.Contains(view, want) {
			t.Fatalf("split info panel missing %q: %q", want, view)
		}
	}
}

func TestModelKeepsDockedInfoPanelWidthStableAcrossSelectionChanges(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{
			Server: "production",
			Device: "router-01-long-name",
			Name: "sshd",
			Protocol: "tcp",
			Ports: []int{22},
			IPs: []string{"192.0.2.10"},
			Description: "primary transit gateway",
		},
		{
			Server: "production",
			Device: "r2",
			Name: "https",
			Protocol: "tcp",
			Ports: []int{443},
			IPs: []string{"192.0.2.11"},
			Description: "web",
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.WindowSizeMsg{Width: 180, Height: 24})
	selector = updated.(model)
	if !selector.infoOpen || selector.infoOverlay {
		t.Fatalf("wide viewport should dock info panel: %#v", selector)
	}
	view := stripANSI(selector.View())
	title := rowLine(t, view, "Service information")
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyDown})
	selector = updated.(model)
	viewAfter := stripANSI(selector.View())
	titleAfter := rowLine(t, viewAfter, "Service information")
	if len(title) != len(titleAfter) {
		t.Fatalf("docked panel width changed: before=%d after=%d", len(title), len(titleAfter))
	}
}

func TestModelKeepsDockedInfoPanelHeightStableAcrossSelectionChanges(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{
		{
			Server: "production",
			Device: "router-01",
			Name: "sshd",
			Protocol: "tcp",
			Ports: []int{22},
			IPs: []string{"192.0.2.10"},
			Description: strings.Repeat("primary transit gateway ", 4),
		},
		{
			Server: "production",
			Device: "r2",
			Name: "https",
			Protocol: "tcp",
			Ports: []int{443},
			IPs: []string{"192.0.2.11"},
			Description: "web",
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.WindowSizeMsg{Width: 180, Height: 30})
	selector = updated.(model)
	if !selector.infoOpen || selector.infoOverlay {
		t.Fatalf("wide viewport should dock info panel: %#v", selector)
	}
	view := stripANSI(selector.View())
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyDown})
	selector = updated.(model)
	viewAfter := stripANSI(selector.View())
	if got, want := len(strings.Split(view, "\n")), len(strings.Split(viewAfter, "\n")); got != want {
		t.Fatalf("docked panel height changed: before=%d after=%d", got, want)
	}
}

func TestModelDoesNotDockInfoPanelWhenItWouldTruncateList(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{
		Server: "production",
		Device: strings.Repeat("router-", 5) + "01",
		Name: strings.Repeat("service-", 4) + "ssh",
		Protocol: "tcp",
		Ports: []int{22, 2222},
		IPs: []string{strings.Repeat("192.0.2.", 3) + "10"},
		Description: strings.Repeat("long description ", 4),
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	selector = updated.(model)
	if selector.infoOpen || selector.infoOverlay {
		t.Fatalf("panel should not dock when it would truncate the list: %#v", selector)
	}
	view := stripANSI(selector.View())
	if strings.Contains(view, "Service information") {
		t.Fatalf("unexpected docked info panel in narrow split: %q", view)
	}
	if !strings.Contains(view, "i info") {
		t.Fatalf("browse footer should still advertise info panel: %q", view)
	}
}

func TestModelShowsInfoPanelOverlayAndConnectsFromNarrowViewport(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{
		Device: "router-01",
		Name: "sshd",
		IPs: []string{"192.0.2.10"},
		Ports: []int{22},
		Description: "Edge router",
	}, {
		Device: "router-02",
		Name: "sshd",
		IPs: []string{"192.0.2.11"},
		Ports: []int{22},
		Description: "Backup router",
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.WindowSizeMsg{Width: 52, Height: 18})
	selector = updated.(model)
	if selector.infoOpen || selector.infoOverlay {
		t.Fatalf("narrow viewport should not auto-open info panel: %#v", selector)
	}
	selector.cursor = 1
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	selector = updated.(model)
	if !selector.infoOverlay || selector.infoOpen {
		t.Fatalf("narrow viewport should open overlay info panel: %#v", selector)
	}
	view := stripANSI(selector.View())
	if strings.Contains(view, "TARGET") || !strings.Contains(view, "Service information") {
		t.Fatalf("overlay should hide the list and show info: %q", view)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	selector = updated.(model)
	if selector.infoOverlay {
		t.Fatalf("i should close the overlay: %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	selector = updated.(model)
	updated, command := selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selected := updated.(model).selection
	if command == nil || selected == nil || selected.Service.TargetName() != "router-02" {
		t.Fatalf("overlay enter should connect to hovered entry: selection=%#v command=%v", selected, command)
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
	if selector.selection != nil || !selector.pinging || !selector.tracing || !selector.networkOverlay {
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
	if !strings.Contains(viewWithPingOutput, "Ping in progress") || !strings.Contains(viewWithPingOutput, "Traceroute") {
		t.Fatalf("view does not show ping popup: %q", viewWithPingOutput)
	}
	if strings.Contains(viewWithPingOutput, "TARGET") || strings.Contains(viewWithPingOutput, "Service information") {
		t.Fatalf("network overlay should render over the list and info panel: %q", viewWithPingOutput)
	}
	if strings.Contains(viewBeforePingOutput, "TARGET") {
		t.Fatalf("network overlay should hide list immediately: %q", viewBeforePingOutput)
	}
	updated, _ = selector.Update(pingMessage{done: true})
	selector = updated.(model)
	updated, _ = selector.Update(traceMessage{done: true})
	selector = updated.(model)
	if selector.pinging || selector.tracing {
		t.Fatalf("completed ping model = %#v", selector)
	}
	if view := stripANSI(selector.View()); !strings.Contains(view, "Ping results") {
		t.Fatalf("view does not retain ping results popup: %q", view)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	selector = updated.(model)
	if selector.networkOverlay || selector.pinging || selector.tracing {
		t.Fatalf("network overlay did not close on q: %#v", selector)
	}
}

func TestModelShowsPingPopupOverSplitInfoPanel(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{
		Server: "production",
		Device: "router-01",
		Name: "sshd",
		IPs: []string{"192.0.2.10"},
		Ports: []int{22},
		Description: "Edge router",
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	selector = updated.(model)
	if !selector.infoOpen {
		t.Fatalf("expected docked info panel to be open: %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	if !selector.pinging {
		t.Fatalf("expected pinging state: %#v", selector)
	}
	updated, _ = selector.Update(pingMessage{line: "64 bytes from 192.0.2.10"})
	selector = updated.(model)
	view := stripANSI(selector.View())
	for _, want := range []string{"Ping in progress", "Traceroute", "┌", "┐", "│", "└", "┘"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view does not contain %q: %q", want, view)
		}
	}
	if strings.Contains(view, "Service information") {
		t.Fatalf("network overlay should hide info panel while active: %q", view)
	}
}

func TestModelClosingNetworkOverlayStopsRunningCommandsOnQ(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{
		Device: "router-01",
		Name: "sshd",
		IPs: []string{"192.0.2.10"},
		Ports: []int{22},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	if !selector.networkOverlay || !selector.pinging || !selector.tracing {
		t.Fatalf("network overlay did not start: %#v", selector)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	selector = updated.(model)
	if selector.networkOverlay || selector.pinging || selector.tracing {
		t.Fatalf("network overlay did not stop active commands: %#v", selector)
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

func TestModelQReturnsHomeFromBrowseMode(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selector.searching = true
	selector.filter.SetValue("router")
	selector.filter.Focus()
	updated, command := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	selector = updated.(model)
	if selector.cancelled || command != nil || selector.searching || selector.filter.Value() != "" {
		t.Fatalf("q should return home without quitting: %#v, command=%v", selector, command)
	}
}

func TestModelEscCancelsFromBrowseMode(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, command := selector.Update(tea.KeyMsg{Type: tea.KeyEsc})
	selector = updated.(model)
	if !selector.cancelled || command == nil {
		t.Fatalf("browse Esc cancellation = %#v, command=%v", selector, command)
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
