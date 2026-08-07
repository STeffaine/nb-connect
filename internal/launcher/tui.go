package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/steffaine/nb-connect/internal/netbox"
)

type Selection struct {
	Service  netbox.Service
	Endpoint string
}

var ErrSelectionCancelled = errors.New("service selection cancelled")

type SyncServices func(context.Context) ([]netbox.Service, error)

func Select(ctx context.Context, services []netbox.Service, pingCount int, syncServices SyncServices) (Selection, error) {
	selector, err := newModelWithPingCount(ctx, services, pingCount, syncServices)
	if err != nil {
		return Selection{}, err
	}
	program := tea.NewProgram(selector, tea.WithContext(ctx), tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout), tea.WithAltScreen())
	result, err := program.Run()
	if err != nil {
		return Selection{}, fmt.Errorf("run service selector: %w", err)
	}
	selected := result.(model)
	if selected.cancelled {
		return Selection{}, ErrSelectionCancelled
	}
	if selected.selection == nil {
		return Selection{}, fmt.Errorf("service selection ended without a result")
	}
	return *selected.selection, nil
}

type model struct {
	choices      []Selection
	choiceSearch []string
	filter       searchInput
	cursor       int
	searching    bool
	syncing      bool
	pinging      bool
	syncError    string
	syncNote     string
	pingNote     string
	context      context.Context
	sync         SyncServices
	pingLines    chan pingMessage
	pingCount    int
	width        int
	height       int
	selection    *Selection
	cancelled    bool
	favorites    map[string]bool
	recents      []string
	statePath    string
}

type syncResult struct {
	services []netbox.Service
	err      error
}

func newModel(ctx context.Context, services []netbox.Service, syncServices SyncServices) (model, error) {
	return newModelWithPingCount(ctx, services, 4, syncServices)
}

func newModelWithPingCount(ctx context.Context, services []netbox.Service, pingCount int, syncServices SyncServices) (model, error) {
	choices, err := choicesForServices(services)
	if err != nil {
		return model{}, err
	}
	filter := searchInput{}
	statePath := defaultLauncherStatePath()
	favorites, recents, err := loadLauncherState(statePath)
	if err != nil {
		favorites = map[string]bool{}
		recents = nil
	}
	return model{choices: choices, choiceSearch: choiceSearchIndex(choices), context: ctx, filter: filter, sync: syncServices, pingCount: pingCount, favorites: favorites, recents: recents, statePath: statePath}, nil
}

func (model model) Init() tea.Cmd {
	return nil
}

func (model model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		return model, nil
	case pingMessage:
		return model.updatePing(message)
	case syncResult:
		return model.updateSync(message)
	case tea.KeyMsg:
		return model.updateKey(message)
	}
	return model, nil
}

func (model model) updatePing(message pingMessage) (model, tea.Cmd) {
	if message.line != "" {
		if model.pingNote != "" {
			model.pingNote += "\n"
		}
		model.pingNote += message.line
	}
	if message.err != nil {
		model.pinging = false
		if model.pingNote == "" {
			model.pingNote = fmt.Sprintf("Ping failed: %v", message.err)
		}
		return model, nil
	}
	if message.done {
		model.pinging = false
		return model, nil
	}
	return model, waitForPingMessage(model.pingLines)
}

func (model model) updateSync(message syncResult) (model, tea.Cmd) {
	model.syncing = false
	if message.err != nil {
		model.syncError = message.err.Error()
		return model, nil
	}
	choices, err := choicesForServices(message.services)
	if err != nil {
		model.syncError = err.Error()
		return model, nil
	}
	model.choices = choices
	model.choiceSearch = choiceSearchIndex(choices)
	model.cursor = 0
	model.filter.SetValue("")
	model.syncError = ""
	model.syncNote = fmt.Sprintf("Synced %d services", len(message.services))
	return model, nil
}

func (model model) updateKey(message tea.KeyMsg) (model, tea.Cmd) {
	if model.searching {
		return model.updateSearchKey(message)
	}
	return model.updateBrowseKey(message)
}

func (model model) updateSearchKey(message tea.KeyMsg) (model, tea.Cmd) {
	switch message.String() {
	case "ctrl+c":
		model.cancelled = true
		return model, tea.Quit
	case "esc":
		model.filter.SetValue("")
		model.filter.Blur()
		model.searching = false
		model.cursor = 0
		return model, nil
	case "enter":
		model.filter.Blur()
		model.searching = false
		return model, nil
	}
	var command tea.Cmd
	model.filter, command = model.filter.Update(message)
	if visible := model.visibleChoices(); model.cursor >= len(visible) {
		model.cursor = max(0, len(visible)-1)
	}
	return model, command
}

func (model model) updateBrowseKey(message tea.KeyMsg) (model, tea.Cmd) {
	if shortcutIndex, ok := numberShortcut(message.String()); ok {
		return model.selectChoice(shortcutIndex)
	}
	switch message.String() {
	case "ctrl+c", "esc", "q":
		model.cancelled = true
		return model, tea.Quit
	case "f":
		model.searching = true
		return model, model.filter.Focus()
	case "s":
		return model.startSync()
	case "enter":
		return model.selectChoice(model.cursor)
	case "m":
		if selection, ok := model.currentChoice(); ok {
			model = model.toggleFavorite(selection)
		}
	case "p":
		if selection, ok := model.currentChoice(); ok && !model.pinging {
			model.pinging = true
			model.pingNote = ""
			model.pingLines = make(chan pingMessage, 16)
			return model, startPing(model.context, selection.Endpoint, model.pingCount, model.pingLines)
		}
	case "l":
		if len(model.recents) > 0 {
			if selection, ok := model.selectionForKey(model.recents[0]); ok {
				model.selection = &selection
				model = model.recordSelection(selection)
				return model, tea.Quit
			}
		}
	case "up", "k":
		if model.cursor > 0 {
			model.cursor--
		}
	case "down", "j":
		if model.cursor+1 < len(model.visibleChoices()) {
			model.cursor++
		}
	}
	return model, nil
}

func (model model) currentChoice() (Selection, bool) {
	visible := model.visibleChoices()
	if model.cursor >= len(visible) {
		return Selection{}, false
	}
	return visible[model.cursor], true
}

func (model model) selectChoice(index int) (model, tea.Cmd) {
	visible := model.visibleChoices()
	if index >= len(visible) {
		return model, nil
	}
	selection := visible[index]
	model.selection = &selection
	model = model.recordSelection(selection)
	return model, tea.Quit
}

func (model model) startSync() (model, tea.Cmd) {
	if model.syncing {
		return model, nil
	}
	if model.sync == nil {
		model.syncError = "sync is unavailable"
		return model, nil
	}
	model.syncing = true
	model.syncError = ""
	model.syncNote = ""
	return model, func() tea.Msg {
		services, err := model.sync(model.context)
		return syncResult{services: services, err: err}
	}
}

func numberShortcut(key string) (int, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	return int(key[0] - '1'), true
}
