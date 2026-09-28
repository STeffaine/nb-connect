package launcher

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/steffaine/nb-connect/internal/config"
	"github.com/steffaine/nb-connect/internal/netbox"
)

func presetTestServices() []netbox.Service {
	return []netbox.Service{
		{Server: "production", Device: "vpn-a", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}, Site: "site-a", Tenant: "team-a", Role: "vpn-server"},
		{Server: "lab", Device: "vpn-b", Name: "sshd", IPs: []string{"192.0.2.11"}, Ports: []int{22}, Site: "site-b", Tenant: "team-b", Role: "vpn-server"},
		{Server: "production", Device: "web-a", Name: "sshd", IPs: []string{"192.0.2.12"}, Ports: []int{22}, Site: "site-a", Tenant: "team-b", Role: "web"},
	}
}

func TestFiltersFromPresetAppliesValues(t *testing.T) {
	choices, err := choicesForServices(presetTestServices())
	if err != nil {
		t.Fatal(err)
	}
	preset := config.FilterPreset{Name: "vpn", Server: "PRODUCTION", Match: "any", Tenants: []string{"team-a"}, Sites: []string{"site-a"}, Roles: []string{"vpn-server"}}.Normalized()
	filters := filtersFromPreset(preset, choices)
	if filters.matchAll {
		t.Fatal("matchAll = true, want false")
	}
	if !filters.values[filterServer]["production"] {
		t.Fatalf("server not canonicalized: %#v", filters.values[filterServer])
	}
	if !filters.values[filterTenant]["team-a"] || !filters.values[filterSite]["site-a"] || !filters.values[filterRole]["vpn-server"] {
		t.Fatalf("preset values missing: %#v", filters.values)
	}
}

func TestPresetFromFiltersRoundTrip(t *testing.T) {
	filters := newServiceFilters()
	filters.matchAll = false
	filters.toggle(filterServer, "production")
	filters.toggle(filterTenant, "team-a")
	preset := presetFromFilters("  vpn ", filters)
	if preset.Name != "vpn" || preset.Server != "production" || preset.Match != "any" {
		t.Fatalf("preset = %#v", preset)
	}
	if len(preset.Tenants) != 1 || preset.Tenants[0] != "team-a" {
		t.Fatalf("tenants = %#v", preset.Tenants)
	}
	choices, err := choicesForServices(presetTestServices())
	if err != nil {
		t.Fatal(err)
	}
	roundTrip := filtersFromPreset(preset, choices)
	if roundTrip.matchAll || !roundTrip.values[filterServer]["production"] || !roundTrip.values[filterTenant]["team-a"] {
		t.Fatalf("round trip filters = %#v", roundTrip.values)
	}
}

func TestInitialPresetAppliesAndServerFilterOverrides(t *testing.T) {
	presets := []config.FilterPreset{
		{Name: "vpn", Server: "lab", Tenants: []string{"team-a"}, Match: "all"},
	}
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{Presets: presets, PresetName: "vpn"})
	if err != nil {
		t.Fatal(err)
	}
	if selector.activePreset != "vpn" {
		t.Fatalf("activePreset = %q", selector.activePreset)
	}
	if !selector.filters.values[filterServer]["lab"] || !selector.filters.values[filterTenant]["team-a"] {
		t.Fatalf("preset not applied: %#v", selector.filters.values)
	}
	overridden, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{Presets: presets, PresetName: "vpn", ServerFilter: "production"})
	if err != nil {
		t.Fatal(err)
	}
	if !overridden.filters.values[filterServer]["production"] || overridden.filters.values[filterServer]["lab"] {
		t.Fatalf("server override failed: %#v", overridden.filters.values[filterServer])
	}
	if !overridden.filters.values[filterTenant]["team-a"] {
		t.Fatalf("preset tenant lost after override: %#v", overridden.filters.values)
	}
}

func TestPresetPickerAppliesHoveredPresetAndReturnsHome(t *testing.T) {
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{
		Presets: []config.FilterPreset{{Name: "first", Sites: []string{"site-a"}}, {Name: "second", Sites: []string{"site-b"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	if !selector.presetPicker {
		t.Fatal("preset picker did not open")
	}
	if view := selector.View(); !strings.Contains(view, "first") || !strings.Contains(view, "second") || !strings.Contains(view, "1-9") {
		t.Fatalf("picker view missing presets or shortcuts:\n%s", view)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selector = updated.(model)
	if selector.presetPicker || selector.filtering {
		t.Fatalf("expected return to main page: picker=%v filtering=%v", selector.presetPicker, selector.filtering)
	}
	if selector.activePreset != "second" || !selector.filters.values[filterSite]["site-b"] {
		t.Fatalf("second preset not applied: %#v %q", selector.filters.values, selector.activePreset)
	}
	if !strings.Contains(selector.presetNote, "second") {
		t.Fatalf("preset note = %q", selector.presetNote)
	}
}

func TestPresetPickerQuickApplyWithNumberKey(t *testing.T) {
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{
		Presets: []config.FilterPreset{{Name: "first", Sites: []string{"site-a"}}, {Name: "second", Sites: []string{"site-b"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	selector = updated.(model)
	if selector.presetPicker || selector.filtering {
		t.Fatalf("expected return to main page: picker=%v filtering=%v", selector.presetPicker, selector.filtering)
	}
	if selector.activePreset != "second" || !selector.filters.values[filterSite]["site-b"] {
		t.Fatalf("second preset not applied: %#v %q", selector.filters.values, selector.activePreset)
	}
}

func TestPresetPickerNavigatesClampsAndCancels(t *testing.T) {
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{
		Presets: []config.FilterPreset{{Name: "first", Sites: []string{"site-a"}}, {Name: "second", Sites: []string{"site-b"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selector = updated.(model)
	if selector.presetCursor != 1 {
		t.Fatalf("preset cursor = %d, want clamped to 1", selector.presetCursor)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	selector = updated.(model)
	if selector.presetCursor != 0 {
		t.Fatalf("preset cursor = %d, want 0", selector.presetCursor)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	if selector.presetPicker || !selector.filtering {
		t.Fatalf("p should close picker back to filter menu: picker=%v filtering=%v", selector.presetPicker, selector.filtering)
	}
	if selector.activePreset != "" || len(selector.filters.values) != 0 {
		t.Fatalf("cancel must not apply a preset: active=%q filters=%#v", selector.activePreset, selector.filters.values)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	selector = updated.(model)
	if selector.presetPicker || selector.filtering {
		t.Fatalf("q should close picker to main page: picker=%v filtering=%v", selector.presetPicker, selector.filtering)
	}
}

func TestManualFilterChangeClearsActivePreset(t *testing.T) {
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{
		Presets:    []config.FilterPreset{{Name: "first", Sites: []string{"site-a"}}},
		PresetName: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selector.activePreset != "first" {
		t.Fatalf("activePreset = %q", selector.activePreset)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	selector = updated.(model)
	if selector.activePreset != "" {
		t.Fatalf("activePreset not cleared after manual change: %q", selector.activePreset)
	}
}

func TestSavePresetFlowStoresAndUpdates(t *testing.T) {
	var saved []config.FilterPreset
	saver := func(preset config.FilterPreset) error {
		saved = append(saved, preset)
		return nil
	}
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{SavePreset: saver})
	if err != nil {
		t.Fatal(err)
	}
	selector.filters.toggle(filterSite, "site-a")
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	selector = updated.(model)
	if !selector.savingPreset {
		t.Fatal("savingPreset not entered")
	}
	selector.presetNameInput.SetValue("my preset")
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selector = updated.(model)
	if selector.savingPreset || len(saved) != 1 || saved[0].Name != "my preset" {
		t.Fatalf("save result = %#v saving=%v", saved, selector.savingPreset)
	}
	if len(saved[0].Sites) != 1 || saved[0].Sites[0] != "site-a" {
		t.Fatalf("saved sites = %#v", saved[0].Sites)
	}
	if !strings.Contains(selector.presetNote, "Saved") || selector.activePreset != "my preset" {
		t.Fatalf("save note = %q active=%q", selector.presetNote, selector.activePreset)
	}
	if len(selector.presets) != 1 {
		t.Fatalf("presets not stored: %#v", selector.presets)
	}
}

func TestSavePresetAcceptsLowercaseKeyAfterTenantFilter(t *testing.T) {
	var saved []config.FilterPreset
	saver := func(preset config.FilterPreset) error {
		saved = append(saved, preset)
		return nil
	}
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{SavePreset: saver})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyTab})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	selector = updated.(model)
	if len(selector.filters.values[filterTenant]) != 1 {
		t.Fatalf("tenant filter not applied: %#v", selector.filters.values)
	}
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	selector = updated.(model)
	if !selector.savingPreset {
		t.Fatal("lowercase s did not enter preset save mode")
	}
	selector.presetNameInput.SetValue("team filter")
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selector = updated.(model)
	if selector.savingPreset || len(saved) != 1 || saved[0].Name != "team filter" {
		t.Fatalf("save result = %#v saving=%v", saved, selector.savingPreset)
	}
	if len(saved[0].Tenants) != 1 || saved[0].Tenants[0] != "team-a" {
		t.Fatalf("saved tenants = %#v", saved[0].Tenants)
	}
}

func TestPresetPickerWithoutPresetsHintsAtSaving(t *testing.T) {
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	if selector.presetError == "" {
		t.Fatal("expected a preset error when no presets exist")
	}
	if !strings.Contains(strings.ToLower(selector.presetError), "save") {
		t.Fatalf("preset error should hint at saving, got %q", selector.presetError)
	}
}

func TestPresetPickerDeleteAsksForConfirmation(t *testing.T) {
	var deleted []string
	deleter := func(name string) error {
		deleted = append(deleted, name)
		return nil
	}
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{
		Presets:      []config.FilterPreset{{Name: "first", Sites: []string{"site-a"}}, {Name: "second", Sites: []string{"site-b"}}},
		DeletePreset: deleter,
	})
	if err != nil {
		t.Fatal(err)
	}
	press := func(key string) {
		t.Helper()
		updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		selector = updated.(model)
	}

	press("f")
	press("p")
	if view := selector.View(); !strings.Contains(view, "r delete") {
		t.Fatalf("picker footer = %q, want r delete hint", view)
	}
	press("j")
	press("r")
	if !selector.deletingPreset {
		t.Fatal("r did not ask for delete confirmation")
	}
	if !strings.Contains(selector.presetNote, "second") {
		t.Fatalf("confirm note = %q, want preset name", selector.presetNote)
	}
	if len(deleted) != 0 || len(selector.presets) != 2 {
		t.Fatalf("r must not delete before confirmation: deleted=%v presets=%#v", deleted, selector.presets)
	}
	press("n")
	if selector.deletingPreset || len(deleted) != 0 || len(selector.presets) != 2 {
		t.Fatalf("n must cancel the delete: confirming=%v deleted=%v presets=%#v", selector.deletingPreset, deleted, selector.presets)
	}
	press("r")
	press("q")
	if selector.filtering || selector.presetPicker || selector.deletingPreset {
		t.Fatal("q must cancel the delete and return home")
	}
	press("f")
	press("p")
	press("j")
	press("r")
	press("y")
	if selector.deletingPreset || len(deleted) != 1 || deleted[0] != "second" {
		t.Fatalf("y must delete the preset: confirming=%v deleted=%v", selector.deletingPreset, deleted)
	}
	if len(selector.presets) != 1 || selector.presets[0].Name != "first" {
		t.Fatalf("presets after delete = %#v", selector.presets)
	}
	if selector.presetCursor != 0 {
		t.Fatalf("preset cursor = %d, want clamped to 0", selector.presetCursor)
	}
	if !strings.Contains(selector.presetNote, "Deleted") {
		t.Fatalf("delete note = %q", selector.presetNote)
	}
}

func TestPresetPickerDeleteLastPresetClosesPicker(t *testing.T) {
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{
		Presets:      []config.FilterPreset{{Name: "only", Sites: []string{"site-a"}}},
		PresetName:   "only",
		DeletePreset: func(name string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	press := func(key string) {
		t.Helper()
		updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		selector = updated.(model)
	}

	press("f")
	press("p")
	press("r")
	press("y")
	if len(selector.presets) != 0 {
		t.Fatalf("presets after delete = %#v", selector.presets)
	}
	if selector.activePreset != "" {
		t.Fatalf("active preset after delete = %q, want empty", selector.activePreset)
	}
	if selector.presetPicker || !selector.filtering {
		t.Fatalf("empty picker should close to filter menu: picker=%v filtering=%v", selector.presetPicker, selector.filtering)
	}
}

func TestPresetPickerDeleteReportsErrors(t *testing.T) {
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{
		Presets: []config.FilterPreset{{Name: "first", Sites: []string{"site-a"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	selector = updated.(model)
	if selector.deletingPreset || selector.presetError == "" {
		t.Fatalf("missing deleter should report error: %#v", selector)
	}
	if len(selector.presets) != 1 {
		t.Fatalf("presets after failed delete = %#v", selector.presets)
	}

	failing, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{
		Presets:      []config.FilterPreset{{Name: "first", Sites: []string{"site-a"}}},
		DeletePreset: func(name string) error { return errors.New("disk full") },
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	failing = updated.(model)
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	failing = updated.(model)
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	failing = updated.(model)
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	failing = updated.(model)
	if failing.deletingPreset || !strings.Contains(failing.presetError, "disk full") {
		t.Fatalf("delete error not reported: %#v", failing)
	}
	if len(failing.presets) != 1 {
		t.Fatalf("presets after failed delete = %#v", failing.presets)
	}
}

func TestSavePresetRequiresNameAndReportsErrors(t *testing.T) {
	selector, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{SavePreset: func(config.FilterPreset) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	selector = updated.(model)
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	selector = updated.(model)
	selector.presetNameInput.SetValue("   ")
	updated, _ = selector.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selector = updated.(model)
	if !selector.savingPreset || selector.presetError == "" {
		t.Fatalf("empty name should stay in save mode with error: %#v", selector)
	}

	failing, err := newModelWithOptions(context.Background(), presetTestServices(), modelOptions{SavePreset: func(config.FilterPreset) error { return errors.New("disk full") }})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	failing = updated.(model)
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	failing = updated.(model)
	failing.presetNameInput.SetValue("vpn")
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyEnter})
	failing = updated.(model)
	if failing.savingPreset || !strings.Contains(failing.presetError, "disk full") {
		t.Fatalf("save error not reported: %#v", failing)
	}

	unavailable, err := newModel(context.Background(), presetTestServices(), nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ = unavailable.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	unavailable = updated.(model)
	updated, _ = unavailable.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	unavailable = updated.(model)
	if unavailable.savingPreset || unavailable.presetError == "" {
		t.Fatalf("missing saver should report error: %#v", unavailable)
	}
}
