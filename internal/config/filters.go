package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FilterPreset is a named set of service filters stored under the top-level
// filters: section of config.yaml.
//
// Example:
//
//	filters:
//	  - name: prod vpn
//	    server: production
//	    match: all
//	    tenants: [team-a]
//	    sites: [site-a]
//	    roles: [vpn-server]
type FilterPreset struct {
	Name    string     `yaml:"name"`
	Server  string     `yaml:"server,omitempty"`
	Servers StringList `yaml:"servers,omitempty"`
	Match   string     `yaml:"match,omitempty"`
	Tenants StringList `yaml:"tenants,omitempty"`
	Sites   StringList `yaml:"sites,omitempty"`
	Roles   StringList `yaml:"roles,omitempty"`
}

// StringList accepts either a single YAML scalar or a sequence of scalars,
// so both `sites: site-a` and `sites: [site-a, site-b]` load.
type StringList []string

func (list *StringList) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		*list = nil
		return nil
	}
	if value.Kind == yaml.ScalarNode {
		if value.Tag == "!!null" {
			*list = nil
			return nil
		}
		var single string
		if err := value.Decode(&single); err != nil {
			return err
		}
		*list = StringList{single}
		return nil
	}
	var items []string
	if err := value.Decode(&items); err != nil {
		return err
	}
	*list = items
	return nil
}

// MatchAll reports whether the preset requires every selected category to
// match. Empty and "all" mean match-all; "any" means match-any.
func (preset FilterPreset) MatchAll() bool {
	switch strings.ToLower(strings.TrimSpace(preset.Match)) {
	case "", "all":
		return true
	case "any":
		return false
	default:
		return true
	}
}

// EffectiveServers merges the singular Server shorthand with the Servers list.
func (preset FilterPreset) EffectiveServers() []string {
	return normalizeValues(append([]string{preset.Server}, preset.Servers...))
}

// EffectiveTenants returns the trimmed, de-duplicated tenant values.
func (preset FilterPreset) EffectiveTenants() []string {
	return normalizeValues(preset.Tenants)
}

// EffectiveSites returns the trimmed, de-duplicated site values.
func (preset FilterPreset) EffectiveSites() []string {
	return normalizeValues(preset.Sites)
}

// EffectiveRoles returns the trimmed, de-duplicated role values.
func (preset FilterPreset) EffectiveRoles() []string {
	return normalizeValues(preset.Roles)
}

// Validate checks the preset name and match mode.
func (preset FilterPreset) Validate() error {
	if strings.TrimSpace(preset.Name) == "" {
		return fmt.Errorf("filters preset name is required")
	}
	switch trimmed := strings.ToLower(strings.TrimSpace(preset.Match)); trimmed {
	case "", "all", "any":
		return nil
	default:
		return fmt.Errorf("filters preset %q match must be \"all\" or \"any\", got %q", strings.TrimSpace(preset.Name), preset.Match)
	}
}

// Normalized returns a copy with trimmed values, dropped empties, and
// de-duplicated entries. Server values are folded so a single server uses the
// singular Server field and multiple servers use the Servers list. Match is
// lowercased to "all" or "any".
func (preset FilterPreset) Normalized() FilterPreset {
	normalized := FilterPreset{Name: strings.TrimSpace(preset.Name)}
	servers := normalizeValues(append([]string{preset.Server}, preset.Servers...))
	if len(servers) == 1 {
		normalized.Server = servers[0]
	} else {
		normalized.Servers = servers
	}
	switch strings.ToLower(strings.TrimSpace(preset.Match)) {
	case "any":
		normalized.Match = "any"
	default:
		normalized.Match = "all"
	}
	normalized.Tenants = normalizeValues(preset.Tenants)
	normalized.Sites = normalizeValues(preset.Sites)
	normalized.Roles = normalizeValues(preset.Roles)
	return normalized
}

// PresetByName finds a preset by case-insensitive name.
func PresetByName(presets []FilterPreset, name string) (FilterPreset, bool) {
	name = strings.TrimSpace(name)
	for _, preset := range presets {
		if strings.EqualFold(strings.TrimSpace(preset.Name), name) {
			return preset, true
		}
	}
	return FilterPreset{}, false
}

func normalizeValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizeFilterPresets(presets []FilterPreset) ([]FilterPreset, error) {
	seen := make(map[string]struct{}, len(presets))
	normalized := make([]FilterPreset, 0, len(presets))
	for index := range presets {
		preset := presets[index]
		if err := preset.Validate(); err != nil {
			return nil, err
		}
		lowered := strings.ToLower(strings.TrimSpace(preset.Name))
		if _, exists := seen[lowered]; exists {
			return nil, fmt.Errorf("filters contains duplicate preset name %q", strings.TrimSpace(preset.Name))
		}
		seen[lowered] = struct{}{}
		normalized = append(normalized, preset.Normalized())
	}
	return normalized, nil
}

// SaveFilterPreset adds or replaces the named preset in the filters: section
// of the config file at path, creating that section when it is missing. All
// other configuration content and comments are preserved.
func SaveFilterPreset(path string, preset FilterPreset) error {
	if err := preset.Validate(); err != nil {
		return err
	}
	preset = preset.Normalized()

	document, root, err := readFilterDocument(path)
	if err != nil {
		return err
	}
	sequence, err := filtersSequence(root)
	if err != nil {
		return fmt.Errorf("parse configuration %q: %w", path, err)
	}
	if sequence == nil {
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "filters"}
		sequence = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		root.Content = append(root.Content, key, sequence)
	}

	replacement := presetNode(preset)
	replaced := false
	for index, item := range sequence.Content {
		if !presetNameMatches(item, preset.Name) {
			continue
		}
		sequence.Content[index] = replacement
		replaced = true
		break
	}
	if !replaced {
		sequence.Content = append(sequence.Content, replacement)
	}
	return writeFilterDocument(path, document)
}

// DeleteFilterPreset removes the named preset from the filters: section of
// the config file at path. Matching is case-insensitive. All other
// configuration content and comments are preserved.
func DeleteFilterPreset(path string, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("filters preset name is required")
	}
	document, root, err := readFilterDocument(path)
	if err != nil {
		return err
	}
	sequence, err := filtersSequence(root)
	if err != nil {
		return fmt.Errorf("parse configuration %q: %w", path, err)
	}
	removed := false
	if sequence != nil {
		kept := make([]*yaml.Node, 0, len(sequence.Content))
		for _, item := range sequence.Content {
			if presetNameMatches(item, name) {
				removed = true
				continue
			}
			kept = append(kept, item)
		}
		sequence.Content = kept
	}
	if !removed {
		return fmt.Errorf("no filter preset named %q", name)
	}
	return writeFilterDocument(path, document)
}

func readFilterDocument(path string) (*yaml.Node, *yaml.Node, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read configuration %q: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return nil, nil, fmt.Errorf("parse configuration %q: %w", path, err)
	}
	root := documentContent(&document)
	if root == nil {
		root = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		document.Kind = yaml.DocumentNode
		document.Content = []*yaml.Node{root}
	}
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("parse configuration %q: top-level value must be a mapping", path)
	}
	return &document, root, nil
}

func writeFilterDocument(path string, document *yaml.Node) error {
	var rendered strings.Builder
	encoder := yaml.NewEncoder(&rendered)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode configuration %q: %w", path, err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("encode configuration %q: %w", path, err)
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("write configuration %q: %w", path, err)
	}
	temporaryName := temporary.Name()
	if _, err := temporary.WriteString(rendered.String()); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
		return fmt.Errorf("write configuration %q: %w", path, err)
	}
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
		return fmt.Errorf("write configuration %q: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryName)
		return fmt.Errorf("write configuration %q: %w", path, err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		_ = os.Remove(temporaryName)
		return fmt.Errorf("write configuration %q: %w", path, err)
	}
	return nil
}

func documentContent(document *yaml.Node) *yaml.Node {
	if document == nil {
		return nil
	}
	if document.Kind == yaml.DocumentNode {
		if len(document.Content) == 0 {
			return nil
		}
		return document.Content[0]
	}
	return document
}

func filtersSequence(root *yaml.Node) (*yaml.Node, error) {
	for index := 0; index+1 < len(root.Content); index += 2 {
		key := root.Content[index]
		value := root.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Value != "filters" {
			continue
		}
		if value == nil || (value.Kind == yaml.ScalarNode && value.Tag == "!!null") {
			sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			root.Content[index+1] = sequence
			return sequence, nil
		}
		if value.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("filters section must be a list")
		}
		return value, nil
	}
	return nil, nil
}

func presetNameMatches(item *yaml.Node, name string) bool {
	if item == nil || item.Kind != yaml.MappingNode {
		return false
	}
	for index := 0; index+1 < len(item.Content); index += 2 {
		key := item.Content[index]
		value := item.Content[index+1]
		if key.Kind == yaml.ScalarNode && key.Value == "name" && value.Kind == yaml.ScalarNode {
			return strings.EqualFold(strings.TrimSpace(value.Value), strings.TrimSpace(name))
		}
	}
	return false
}

func presetNode(preset FilterPreset) *yaml.Node {
	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar := func(key, value string) {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
		)
	}
	addList := func(key string, values []string) {
		if len(values) == 0 {
			return
		}
		sorted := append([]string(nil), values...)
		sort.Slice(sorted, func(left, right int) bool {
			if strings.ToLower(sorted[left]) == strings.ToLower(sorted[right]) {
				return sorted[left] < sorted[right]
			}
			return strings.ToLower(sorted[left]) < strings.ToLower(sorted[right])
		})
		sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, value := range sorted {
			sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
		}
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			sequence,
		)
	}
	addScalar("name", preset.Name)
	if preset.Server != "" {
		addScalar("server", preset.Server)
	}
	addList("servers", preset.Servers)
	if strings.ToLower(preset.Match) == "any" {
		addScalar("match", "any")
	}
	addList("tenants", preset.Tenants)
	addList("sites", preset.Sites)
	addList("roles", preset.Roles)
	return mapping
}
