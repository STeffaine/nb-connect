package launcher

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/steffaine/nb-connect/internal/config"
)

// PresetSaver persists a filter preset to config.yaml.
type PresetSaver func(preset config.FilterPreset) error

// PresetDeleter removes a filter preset from config.yaml by name.
type PresetDeleter func(name string) error

func filtersFromPreset(preset config.FilterPreset, choices []Selection) serviceFilters {
	filters := newServiceFilters()
	filters.matchAll = preset.MatchAll()
	for _, server := range preset.EffectiveServers() {
		filters.toggle(filterServer, canonicalPresetServer(choices, server))
	}
	for _, tenant := range preset.EffectiveTenants() {
		filters.toggle(filterTenant, tenant)
	}
	for _, site := range preset.EffectiveSites() {
		filters.toggle(filterSite, site)
	}
	for _, role := range preset.EffectiveRoles() {
		filters.toggle(filterRole, role)
	}
	return filters
}

func presetFromFilters(name string, filters serviceFilters) config.FilterPreset {
	preset := config.FilterPreset{Name: strings.TrimSpace(name)}
	if filters.matchAll {
		preset.Match = "all"
	} else {
		preset.Match = "any"
	}
	preset.Tenants = sortedFilterValues(filters.values[filterTenant])
	preset.Sites = sortedFilterValues(filters.values[filterSite])
	preset.Roles = sortedFilterValues(filters.values[filterRole])
	servers := sortedFilterValues(filters.values[filterServer])
	if len(servers) == 1 {
		preset.Server = servers[0]
	} else {
		preset.Servers = servers
	}
	return preset
}

func sortedFilterValues(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	sorted := make([]string, 0, len(values))
	for value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			sorted = append(sorted, value)
		}
	}
	sort.Slice(sorted, func(left, right int) bool {
		if strings.ToLower(sorted[left]) == strings.ToLower(sorted[right]) {
			return sorted[left] < sorted[right]
		}
		return strings.ToLower(sorted[left]) < strings.ToLower(sorted[right])
	})
	return sorted
}

func canonicalPresetServer(choices []Selection, name string) string {
	trimmed := strings.TrimSpace(name)
	for _, choice := range choices {
		if strings.EqualFold(choice.Service.Server, trimmed) {
			return choice.Service.Server
		}
	}
	return trimmed
}

func (model model) applyPreset(preset config.FilterPreset) model {
	model.filters = filtersFromPreset(preset, model.choices)
	model.activePreset = strings.TrimSpace(preset.Name)
	model.cursor = 0
	model.filterCursor = 0
	return model
}

func (model model) storePreset(preset config.FilterPreset) model {
	name := strings.TrimSpace(preset.Name)
	replaced := false
	for index := range model.presets {
		if strings.EqualFold(strings.TrimSpace(model.presets[index].Name), name) {
			model.presets[index] = preset
			replaced = true
			break
		}
	}
	if !replaced {
		model.presets = append(model.presets, preset)
	}
	model.activePreset = name
	return model
}

func (model model) openPresetPicker() (model, tea.Cmd) {
	if len(model.presets) == 0 {
		model.presetNote = ""
		model.presetError = "No filter presets saved yet - press s to save the current filters"
		return model, nil
	}
	model.presetPicker = true
	model.presetCursor = 0
	for index := range model.presets {
		if strings.EqualFold(strings.TrimSpace(model.presets[index].Name), model.activePreset) {
			model.presetCursor = index
			break
		}
	}
	model.deletingPreset = false
	model.deletePresetName = ""
	model.presetNote = ""
	model.presetError = ""
	return model, nil
}

func (model model) updatePresetPickerKey(message tea.KeyMsg) (model, tea.Cmd) {
	if model.deletingPreset {
		switch message.String() {
		case "y", "Y":
			return model.deletePresetFromPicker(), nil
		case "n", "N":
			model.deletingPreset = false
			model.deletePresetName = ""
			model.presetNote = ""
			model.presetError = ""
			return model, nil
		}
		return model, nil
	}
	switch message.String() {
	case "up", "k":
		if model.presetCursor > 0 {
			model.presetCursor--
		}
		return model, nil
	case "down", "j":
		if model.presetCursor+1 < len(model.presets) {
			model.presetCursor++
		}
		return model, nil
	case "enter":
		return model.applyPresetFromPicker(model.presetCursor), nil
	case "r", "R":
		return model.confirmDeletePreset(), nil
	case "p", "P":
		model.presetPicker = false
		model.presetCursor = 0
		return model, nil
	}
	if shortcutIndex, ok := numberShortcut(message.String()); ok {
		return model.applyPresetFromPicker(shortcutIndex), nil
	}
	return model, nil
}

func (model model) confirmDeletePreset() model {
	if model.presetCursor < 0 || model.presetCursor >= len(model.presets) {
		return model
	}
	if model.deletePreset == nil {
		model.presetNote = ""
		model.presetError = "Deleting presets is unavailable"
		return model
	}
	model.deletingPreset = true
	model.deletePresetName = strings.TrimSpace(model.presets[model.presetCursor].Name)
	model.presetNote = fmt.Sprintf("Delete preset %q? (y/n)", model.deletePresetName)
	model.presetError = ""
	return model
}

func (model model) deletePresetFromPicker() model {
	name := model.deletePresetName
	model.deletingPreset = false
	model.deletePresetName = ""
	if model.deletePreset == nil {
		model.presetNote = ""
		model.presetError = "Deleting presets is unavailable"
		return model
	}
	if err := model.deletePreset(name); err != nil {
		model.presetNote = ""
		model.presetError = err.Error()
		return model
	}
	kept := make([]config.FilterPreset, 0, len(model.presets))
	for _, preset := range model.presets {
		if strings.EqualFold(strings.TrimSpace(preset.Name), name) {
			continue
		}
		kept = append(kept, preset)
	}
	model.presets = kept
	if strings.EqualFold(model.activePreset, name) {
		model.activePreset = ""
	}
	if model.presetCursor >= len(model.presets) {
		model.presetCursor = max(0, len(model.presets)-1)
	}
	model.presetNote = fmt.Sprintf("Deleted preset %q", name)
	model.presetError = ""
	if len(model.presets) == 0 {
		model.presetPicker = false
		model.presetCursor = 0
	}
	return model
}

func (model model) applyPresetFromPicker(index int) model {
	if index < 0 || index >= len(model.presets) {
		return model
	}
	model = model.applyPreset(model.presets[index])
	model = model.returnHome()
	model.presetNote = fmt.Sprintf("Applied preset %q", model.activePreset)
	model.presetError = ""
	return model
}

func (model model) savePresetFromInput() (model, tea.Cmd) {
	name := strings.TrimSpace(model.presetNameInput.Value())
	if name == "" {
		model.presetNote = ""
		model.presetError = "Preset name is required"
		return model, nil
	}
	if model.savePreset == nil {
		model.presetNote = ""
		model.presetError = "Saving presets is unavailable"
		model.savingPreset = false
		model.presetNameInput.SetValue("")
		model.presetNameInput.Blur()
		return model, nil
	}
	_, exists := config.PresetByName(model.presets, name)
	preset := presetFromFilters(name, model.filters)
	if err := model.savePreset(preset.Normalized()); err != nil {
		model.presetNote = ""
		model.presetError = err.Error()
		model.savingPreset = false
		model.presetNameInput.SetValue("")
		model.presetNameInput.Blur()
		return model, nil
	}
	model = model.storePreset(preset.Normalized())
	model.savingPreset = false
	model.presetNameInput.SetValue("")
	model.presetNameInput.Blur()
	if exists {
		model.presetNote = fmt.Sprintf("Updated preset %q", name)
	} else {
		model.presetNote = fmt.Sprintf("Saved preset %q", name)
	}
	model.presetError = ""
	return model, nil
}
