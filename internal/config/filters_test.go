package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFilterPresets(t *testing.T) {
	path := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nfilters:\n  - name: prod vpn\n    server: production\n    match: any\n    tenants: team-a\n    sites: [site-a, site-b]\n    roles:\n      - vpn-server\n  - name: multi\n    servers: [production, lab]\n    tenants: []\n")
	configuration, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(configuration.Filters) != 2 {
		t.Fatalf("Filters count = %d, want 2", len(configuration.Filters))
	}
	first := configuration.Filters[0]
	if first.Name != "prod vpn" || first.Server != "production" || first.Match != "any" {
		t.Fatalf("first preset = %#v", first)
	}
	if strings.Join(first.EffectiveTenants(), ",") != "team-a" {
		t.Fatalf("tenants = %#v", first.EffectiveTenants())
	}
	if strings.Join(first.EffectiveSites(), ",") != "site-a,site-b" {
		t.Fatalf("sites = %#v", first.EffectiveSites())
	}
	if strings.Join(first.EffectiveRoles(), ",") != "vpn-server" {
		t.Fatalf("roles = %#v", first.EffectiveRoles())
	}
	if first.MatchAll() {
		t.Fatal("MatchAll = true, want false for match any")
	}
	second := configuration.Filters[1]
	if second.Match != "all" || !second.MatchAll() {
		t.Fatalf("default match = %q", second.Match)
	}
	if strings.Join(second.EffectiveServers(), ",") != "production,lab" {
		t.Fatalf("servers = %#v", second.EffectiveServers())
	}
}

func TestLoadRejectsInvalidFilterPresets(t *testing.T) {
	testCases := []struct {
		name     string
		contents string
		want     string
	}{
		{name: "missing name", contents: "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nfilters:\n  - server: production\n", want: "preset name is required"},
		{name: "duplicate name", contents: "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nfilters:\n  - name: Foo\n  - name: foo\n", want: "duplicate preset name"},
		{name: "invalid match", contents: "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nfilters:\n  - name: foo\n    match: sometimes\n", want: "match must be"},
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

func TestPresetByNameMatchesCaseInsensitively(t *testing.T) {
	presets := []FilterPreset{{Name: "Prod VPN"}, {Name: "lab"}}
	preset, ok := PresetByName(presets, "  prod vpn ")
	if !ok || preset.Name != "Prod VPN" {
		t.Fatalf("PresetByName() = %#v, %v", preset, ok)
	}
	if _, ok := PresetByName(presets, "missing"); ok {
		t.Fatal("PresetByName() found missing preset")
	}
}

func TestSaveFilterPresetCreatesFiltersSection(t *testing.T) {
	bases := map[string]string{
		"missing section": "# leading comment\nnetbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\n",
		"empty section":   "# leading comment\nnetbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nfilters:\n",
	}
	for name, base := range bases {
		t.Run(name, func(t *testing.T) {
			path := writeFile(t, "config.yaml", base)
			preset := FilterPreset{Name: "prod vpn", Server: "production", Match: "all", Tenants: []string{"team-a"}, Sites: []string{"site-a"}, Roles: []string{"vpn-server"}}
			if err := SaveFilterPreset(path, preset); err != nil {
				t.Fatal(err)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(contents)
			if !strings.Contains(text, "# leading comment") {
				t.Fatalf("comments not preserved: %q", text)
			}
			if !strings.Contains(text, "netbox:") || !strings.Contains(text, "production") {
				t.Fatalf("existing config lost: %q", text)
			}
			if !strings.Contains(text, "filters:") || !strings.Contains(text, "prod vpn") {
				t.Fatalf("filters section missing: %q", text)
			}
			configuration, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(configuration.Filters) != 1 || configuration.Filters[0].Name != "prod vpn" {
				t.Fatalf("saved filters = %#v", configuration.Filters)
			}
		})
	}
}

func TestSaveFilterPresetAppendsAndReplaces(t *testing.T) {
	path := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nfilters:\n  - name: first\n    server: production\n")
	if err := SaveFilterPreset(path, FilterPreset{Name: "second", Sites: []string{"site-b"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveFilterPreset(path, FilterPreset{Name: "FIRST", Roles: []string{"web"}}); err != nil {
		t.Fatal(err)
	}
	configuration, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(configuration.Filters) != 2 {
		t.Fatalf("Filters = %#v, want 2 presets", configuration.Filters)
	}
	updated, ok := PresetByName(configuration.Filters, "first")
	if !ok {
		t.Fatalf("updated preset missing: %#v", configuration.Filters)
	}
	if len(updated.EffectiveServers()) != 0 || strings.Join(updated.EffectiveRoles(), ",") != "web" {
		t.Fatalf("updated preset = %#v", updated)
	}
}

func TestSaveFilterPresetWritesServerShapeAndMatch(t *testing.T) {
	path := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\n")
	if err := SaveFilterPreset(path, FilterPreset{Name: "single", Server: "production", Match: "all"}); err != nil {
		t.Fatal(err)
	}
	contents, _ := os.ReadFile(path)
	if text := string(contents); !strings.Contains(text, "server: production") || strings.Contains(text, "match:") {
		t.Fatalf("single server preset should use singular server and omit default match: %q", text)
	}
	if err := SaveFilterPreset(path, FilterPreset{Name: "multi", Servers: []string{"b", "a"}, Match: "any"}); err != nil {
		t.Fatal(err)
	}
	contents, _ = os.ReadFile(path)
	text := string(contents)
	if !strings.Contains(text, "match: any") {
		t.Fatalf("match any missing: %q", text)
	}
	configuration, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	multi, _ := PresetByName(configuration.Filters, "multi")
	if strings.Join(multi.EffectiveServers(), ",") != "a,b" && strings.Join(multi.EffectiveServers(), ",") != "b,a" {
		t.Fatalf("servers = %#v", multi.EffectiveServers())
	}
	if multi.Server != "" || len(multi.Servers) != 2 {
		t.Fatalf("multi-server preset should use servers list: %#v", multi)
	}
}

func TestDeleteFilterPresetRemovesEntry(t *testing.T) {
	path := writeFile(t, "config.yaml", "# leading comment\nnetbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nfilters:\n  - name: first\n    server: production\n  - name: second\n    sites: [site-b]\n")
	if err := DeleteFilterPreset(path, "SECOND"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "# leading comment") || !strings.Contains(text, "netbox:") {
		t.Fatalf("existing config lost: %q", text)
	}
	if strings.Contains(text, "second") {
		t.Fatalf("deleted preset still present: %q", text)
	}
	configuration, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(configuration.Filters) != 1 || configuration.Filters[0].Name != "first" {
		t.Fatalf("remaining filters = %#v", configuration.Filters)
	}
}

func TestDeleteFilterPresetRejectsInvalid(t *testing.T) {
	path := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nfilters:\n  - name: first\n    server: production\n")
	if err := DeleteFilterPreset(path, "missing"); err == nil || !strings.Contains(err.Error(), `no filter preset named "missing"`) {
		t.Fatalf("DeleteFilterPreset() error = %v", err)
	}
	if err := DeleteFilterPreset(path, "   "); err == nil {
		t.Fatal("expected error for empty preset name")
	}
	if err := DeleteFilterPreset(filepath.Join(t.TempDir(), "missing.yaml"), "first"); err == nil {
		t.Fatal("expected error for missing config file")
	}
	empty := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\n")
	if err := DeleteFilterPreset(empty, "first"); err == nil || !strings.Contains(err.Error(), "no filter preset named") {
		t.Fatalf("DeleteFilterPreset() error = %v", err)
	}
}

func TestSaveFilterPresetRejectsInvalid(t *testing.T) {
	path := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\n")
	if err := SaveFilterPreset(path, FilterPreset{Name: "  "}); err == nil {
		t.Fatal("expected error for empty preset name")
	}
	if err := SaveFilterPreset(path, FilterPreset{Name: "bad", Match: "sometimes"}); err == nil {
		t.Fatal("expected error for invalid match")
	}
	if err := SaveFilterPreset(filepath.Join(t.TempDir(), "missing.yaml"), FilterPreset{Name: "foo"}); err == nil {
		t.Fatal("expected error for missing config file")
	}
	badShape := writeFile(t, "config.yaml", "netbox:\n  servers:\n    - name: production\n      url: https://netbox.example.test\nfilters:\n  name: not-a-list\n")
	if err := SaveFilterPreset(badShape, FilterPreset{Name: "foo"}); err == nil || !strings.Contains(err.Error(), "filters section must be a list") {
		t.Fatalf("SaveFilterPreset() error = %v", err)
	}
}
