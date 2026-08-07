package launcher

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func choicesForServices(services []netbox.Service) ([]Selection, error) {
	choices := make([]Selection, 0, len(services))
	for _, service := range services {
		for _, endpoint := range service.Endpoints() {
			choices = append(choices, Selection{Service: service, Endpoint: endpoint})
		}
	}
	if len(choices) == 0 {
		return nil, fmt.Errorf("no cached services with usable endpoints are available; run nbcon sync")
	}
	return choices, nil
}

func choiceSearchIndex(choices []Selection) []string {
	index := make([]string, len(choices))
	for position, selection := range choices {
		service := selection.Service
		index[position] = strings.ToLower(strings.Join([]string{
			service.TargetName(), service.Name, selection.Endpoint, service.Role, service.Tenant, service.Status,
		}, " "))
	}
	return index
}

func (model model) visibleChoices() []Selection {
	query := strings.ToLower(strings.TrimSpace(model.filter.Value()))
	if len(model.choiceSearch) != len(model.choices) {
		return model.matchingChoices(query, choiceSearchIndex(model.choices))
	}
	return model.matchingChoices(query, model.choiceSearch)
}

func (model model) matchingChoices(query string, searchIndex []string) []Selection {
	choices := make([]Selection, 0, len(model.choices))
	favorites := make([]Selection, 0, len(model.choices))
	recentsByKey := make(map[string]Selection, len(model.recents))
	for position, selection := range model.choices {
		candidate := searchIndex[position]
		if query != "" && !strings.Contains(candidate, query) && !fuzzyMatch(query, candidate) {
			continue
		}
		switch model.priorityFor(selection) {
		case 0:
			favorites = append(favorites, selection)
		case 1:
			recentsByKey[selectionKey(selection)] = selection
		default:
			choices = append(choices, selection)
		}
	}
	recents := make([]Selection, 0, len(recentsByKey))
	for _, key := range model.recents {
		if selection, ok := recentsByKey[key]; ok {
			recents = append(recents, selection)
			delete(recentsByKey, key)
		}
	}
	if len(favorites)+len(recents)+len(choices) == 0 {
		return nil
	}
	return append(append(favorites, recents...), choices...)
}

func fuzzyMatch(query, candidate string) bool {
	for _, queryRune := range query {
		matchPosition := strings.IndexRune(candidate, queryRune)
		if matchPosition < 0 {
			return false
		}
		candidate = candidate[matchPosition+utf8.RuneLen(queryRune):]
	}
	return true
}

func (model model) priorityFor(selection Selection) int {
	key := selectionKey(selection)
	switch {
	case model.favorites[key]:
		return 0
	case model.isRecent(key):
		return 1
	default:
		return 2
	}
}

func sectionHeading(priority int) string {
	switch priority {
	case 0:
		return "Favorites"
	case 1:
		return "Recents"
	default:
		return ""
	}
}

func (model model) selectionForKey(key string) (Selection, bool) {
	for _, selection := range model.choices {
		if selectionKey(selection) == key {
			return selection, true
		}
	}
	return Selection{}, false
}

func (model model) isRecent(key string) bool {
	for _, recent := range model.recents {
		if recent == key {
			return true
		}
	}
	return false
}

func selectionKey(selection Selection) string {
	parts := []string{strings.ToLower(strings.TrimSpace(selection.Service.TargetName())), strings.ToLower(strings.TrimSpace(selection.Service.Name)), strings.ToLower(strings.TrimSpace(selection.Endpoint))}
	return strings.Join(parts, "::")
}
