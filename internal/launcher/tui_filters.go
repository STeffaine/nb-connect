package launcher

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type filterCategory string

const (
	filterServer filterCategory = "NetBox server"
	filterTenant filterCategory = "Tenant"
	filterSite   filterCategory = "Site"
	filterRole   filterCategory = "Role"
)

var filterCategories = []filterCategory{filterServer, filterTenant, filterSite, filterRole}

type serviceFilters struct {
	values   map[filterCategory]map[string]bool
	matchAll bool
}

func newServiceFilters() serviceFilters {
	return serviceFilters{values: make(map[filterCategory]map[string]bool), matchAll: true}
}

func (filters *serviceFilters) toggle(category filterCategory, value string) {
	if filters.values == nil {
		filters.values = make(map[filterCategory]map[string]bool)
	}
	if filters.values[category] == nil {
		filters.values[category] = make(map[string]bool)
	}
	if filters.values[category][value] {
		delete(filters.values[category], value)
		if len(filters.values[category]) == 0 {
			delete(filters.values, category)
		}
		return
	}
	filters.values[category][value] = true
}

func (filters serviceFilters) matches(selection Selection) bool {
	if len(filters.values) == 0 {
		return true
	}
	matched := 0
	for category, values := range filters.values {
		if values[filterValue(selection, category)] {
			matched++
		}
	}
	if filters.matchAll {
		return matched == len(filters.values)
	}
	return matched > 0
}

func (model model) filterOptions() []string {
	category := filterCategories[model.filterCategory]
	values := make(map[string]struct{})
	for _, selection := range model.choices {
		if value := filterValue(selection, category); value != "" {
			values[value] = struct{}{}
		}
	}
	query := strings.ToLower(strings.TrimSpace(model.filterMenuSearch.Value()))
	options := make([]string, 0, len(values))
	for value := range values {
		candidate := strings.ToLower(value)
		if query == "" || strings.Contains(candidate, query) || fuzzyMatch(query, candidate) {
			options = append(options, value)
		}
	}
	sort.Slice(options, func(left, right int) bool {
		return strings.ToLower(options[left]) < strings.ToLower(options[right])
	})
	return options
}

func filterValue(selection Selection, category filterCategory) string {
	switch category {
	case filterServer:
		return selection.Service.Server
	case filterTenant:
		return selection.Service.Tenant
	case filterSite:
		return selection.Service.Site
	case filterRole:
		return selection.Service.Role
	default:
		return ""
	}
}

func (model model) updateFilterKey(message tea.KeyMsg) (model, tea.Cmd) {
	if model.filterSearching {
		switch message.String() {
		case "ctrl+c":
			model.cancelled = true
			return model, tea.Quit
		case "esc":
			model.filterMenuSearch.SetValue("")
			model.filterMenuSearch.Blur()
			model.filterSearching = false
			model.filterCursor = 0
			return model, nil
		case "up":
			if model.filterCursor > 0 {
				model.filterCursor--
			}
			return model, nil
		case "down":
			if options := model.filterOptions(); model.filterCursor+1 < len(options) {
				model.filterCursor++
			}
			return model, nil
		case " ", "enter":
			options := model.filterOptions()
			if model.filterCursor < len(options) {
				model.filters.toggle(filterCategories[model.filterCategory], options[model.filterCursor])
			}
			if message.String() == " " {
				return model, nil
			}
			model.filterMenuSearch.Blur()
			model.filterSearching = false
			return model, nil
		}
		var command tea.Cmd
		model.filterMenuSearch, command = model.filterMenuSearch.Update(message)
		if options := model.filterOptions(); model.filterCursor >= len(options) {
			model.filterCursor = max(0, len(options)-1)
		}
		return model, command
	}

	options := model.filterOptions()
	switch message.String() {
	case "ctrl+c":
		model.cancelled = true
		return model, tea.Quit
	case "esc":
		model.filterMenuSearch.Blur()
		model.filtering = false
		model.cursor = 0
		return model, nil
	case "/":
		model.filterSearching = true
		model.filterOptionsFocused = true
		return model, model.filterMenuSearch.Focus()
	case "tab":
		model.filterOptionsFocused = !model.filterOptionsFocused
		return model, nil
	case "a":
		model.filters.matchAll = !model.filters.matchAll
		return model, nil
	case "left", "shift+tab":
		model.filterCategory = (model.filterCategory + len(filterCategories) - 1) % len(filterCategories)
		model.filterMenuSearch.SetValue("")
		model.filterCursor = 0
		return model, nil
	case "right":
		model.filterCategory = (model.filterCategory + 1) % len(filterCategories)
		model.filterMenuSearch.SetValue("")
		model.filterCursor = 0
		return model, nil
	}

	if model.filterOptionsFocused {
		switch message.String() {
		case "up", "k":
			if model.filterCursor > 0 {
				model.filterCursor--
			}
		case "down", "j":
			if model.filterCursor+1 < len(options) {
				model.filterCursor++
			}
		case " ", "enter":
			if model.filterCursor < len(options) {
				model.filters.toggle(filterCategories[model.filterCategory], options[model.filterCursor])
			}
		}
		return model, nil
	}

	switch message.String() {
	case "up", "k":
		model.filterCategory = (model.filterCategory + len(filterCategories) - 1) % len(filterCategories)
	case "down", "j":
		model.filterCategory = (model.filterCategory + 1) % len(filterCategories)
	default:
		return model, nil
	}
	model.filterMenuSearch.SetValue("")
	model.filterCursor = 0
	return model, nil
}

func (model model) filterSummary() string {
	if len(model.filters.values) == 0 {
		return ""
	}
	mode := "all"
	if !model.filters.matchAll {
		mode = "any"
	}
	return "Filters (" + mode + "): " + strings.Join(model.selectedFilterValues(), ", ")
}

func (model model) selectedFilterValues() []string {
	var selected []string
	for _, category := range filterCategories {
		for value := range model.filters.values[category] {
			selected = append(selected, string(category)+"="+value)
		}
	}
	sort.Slice(selected, func(left, right int) bool { return strings.ToLower(selected[left]) < strings.ToLower(selected[right]) })
	return selected
}