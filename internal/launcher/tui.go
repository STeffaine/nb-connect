package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/steffaine/nb-connect/internal/netbox"
)

type Selection struct {
	Service  netbox.Service
	Endpoint string
}

var ErrSelectionCancelled = errors.New("service selection cancelled")

type SyncServices func(context.Context) ([]netbox.Service, error)

func Select(ctx context.Context, services []netbox.Service, serverFilter string, pingCount int, infoPanelOpenByDefault bool, syncServices SyncServices) (Selection, error) {
	selector, err := newModelWithPingCountAndSettings(ctx, services, pingCount, infoPanelOpenByDefault, syncServices, serverFilter)
	if err != nil {
		return Selection{}, err
	}
	program := tea.NewProgram(selector, tea.WithContext(ctx), tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout), tea.WithAltScreen(), tea.WithFPS(15))
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
	choices              []Selection
	choiceSearch         []string
	filter               searchInput
	filterMenuSearch     searchInput
	filters              serviceFilters
	cursor               int
	searching            bool
	filtering            bool
	filterSearching      bool
	filterOptionsFocused bool
	filterCategory       int
	filterCursor         int
	syncing              bool
	pinging              bool
	tracing              bool
	networkOverlay       bool
	syncError            string
	syncNote             string
	pingNote             string
	traceNote            string
	context              context.Context
	networkCancel        context.CancelFunc
	sync                 SyncServices
	pingLines            chan pingMessage
	traceLines           chan traceMessage
	pingCount            int
	width                int
	height               int
	selection            *Selection
	infoOpen             bool
	infoOverlay          bool
	infoManual           bool
	infoPanelOpenByDefault bool
	cancelled            bool
	favorites            map[string]bool
	recents              []string
	statePath            string
}

type syncResult struct {
	services []netbox.Service
	err      error
}

func newModel(ctx context.Context, services []netbox.Service, syncServices SyncServices) (model, error) {
	return newModelWithPingCountAndSettings(ctx, services, 4, true, syncServices, "")
}

func newModelWithServerFilter(ctx context.Context, services []netbox.Service, serverFilter string, syncServices SyncServices) (model, error) {
	return newModelWithPingCountAndSettings(ctx, services, 4, true, syncServices, serverFilter)
}

func newModelWithPingCount(ctx context.Context, services []netbox.Service, pingCount int, syncServices SyncServices, serverFilter string) (model, error) {
	return newModelWithPingCountAndSettings(ctx, services, pingCount, true, syncServices, serverFilter)
}

func newModelWithPingCountAndSettings(ctx context.Context, services []netbox.Service, pingCount int, infoPanelOpenByDefault bool, syncServices SyncServices, serverFilter string) (model, error) {
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
	filters := newServiceFilters()
	if trimmed := strings.TrimSpace(serverFilter); trimmed != "" {
		canonicalServer := trimmed
		for _, service := range services {
			if strings.EqualFold(service.Server, trimmed) {
				canonicalServer = service.Server
				break
			}
		}
		filters.toggle(filterServer, canonicalServer)
	}
	return model{choices: choices, choiceSearch: choiceSearchIndex(choices), context: ctx, filter: filter, filters: filters, sync: syncServices, pingCount: pingCount, favorites: favorites, recents: recents, statePath: statePath, infoPanelOpenByDefault: infoPanelOpenByDefault}, nil
}

func (model model) Init() tea.Cmd {
	return nil
}

func (model model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		if !model.infoManual {
			model.infoOpen = model.infoPanelOpenByDefault && model.canDockInfoPanel()
			model.infoOverlay = false
		} else if model.infoOpen && !model.canDockInfoPanel() {
			model.infoOpen = false
			model.infoOverlay = true
		}
		return model, nil
	case pingMessage:
		return model.updatePing(message)
	case traceMessage:
		return model.updateTrace(message)
	case syncResult:
		return model.updateSync(message)
	case tea.KeyMsg:
		return model.updateKey(message)
	}
	return model, nil
}

func (model model) updatePing(message pingMessage) (model, tea.Cmd) {
	if !model.pinging {
		return model, nil
	}
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

func (model model) updateTrace(message traceMessage) (model, tea.Cmd) {
	if !model.tracing {
		return model, nil
	}
	if message.line != "" {
		if model.traceNote != "" {
			model.traceNote += "\n"
		}
		model.traceNote += message.line
	}
	if message.err != nil {
		model.tracing = false
		if model.traceNote == "" {
			model.traceNote = fmt.Sprintf("Traceroute failed: %v", message.err)
		}
		return model, nil
	}
	if message.done {
		model.tracing = false
		return model, nil
	}
	return model, waitForTraceMessage(model.traceLines)
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
	if model.isQuitKey(message.String()) {
		model = model.closeNetworkOverlay()
		model.cancelled = true
		return model, tea.Quit
	}
	if message.String() == "q" {
		return model.returnHome(), nil
	}
	if model.filtering {
		return model.updateFilterKey(message)
	}
	if model.searching {
		return model.updateSearchKey(message)
	}
	return model.updateBrowseKey(message)
}

func (model model) updateSearchKey(message tea.KeyMsg) (model, tea.Cmd) {
	switch message.String() {
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
	if model.networkOverlay {
		switch message.String() {
		case "q", "p":
			model = model.closeNetworkOverlay()
			return model, nil
		}
		return model, nil
	}
	if model.infoOverlay {
		switch message.String() {
		case "q", "i":
			model.infoOverlay = false
			return model, nil
		case "enter":
			return model.selectChoice(model.cursor)
		}
		return model, nil
	}
	if shortcutIndex, ok := numberShortcut(message.String()); ok {
		return model.selectChoice(shortcutIndex)
	}
	switch message.String() {
	case "/":
		model.searching = true
		return model, model.filter.Focus()
	case "i":
		if model.canDockInfoPanel() {
			model.infoOpen = !model.infoOpen
			model.infoOverlay = false
		} else {
			model.infoOverlay = !model.infoOverlay
			model.infoOpen = false
		}
		model.infoManual = true
		return model, nil
	case "f":
		model.filtering = true
		model.filterSearching = false
		model.filterCursor = 0
		return model, nil
	case "s":
		return model.startSync()
	case "enter":
		return model.selectChoice(model.cursor)
	case "m":
		if selection, ok := model.currentChoice(); ok {
			model = model.toggleFavorite(selection)
		}
	case "p":
		if selection, ok := model.currentChoice(); ok && !model.pinging && !model.tracing {
			networkContext, cancel := context.WithCancel(model.context)
			model.networkCancel = cancel
			model.networkOverlay = true
			model.pinging = true
			model.tracing = true
			model.pingNote = ""
			model.traceNote = ""
			model.pingLines = make(chan pingMessage, 16)
			model.traceLines = make(chan traceMessage, 16)
			return model, tea.Batch(
				startPing(networkContext, selection.Endpoint, model.pingCount, model.pingLines),
				startTraceroute(networkContext, selection.Endpoint, model.traceLines),
			)
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

func (model model) closeNetworkOverlay() model {
	if model.networkCancel != nil {
		model.networkCancel()
		model.networkCancel = nil
	}
	model.networkOverlay = false
	model.pinging = false
	model.tracing = false
	return model
}

func (model model) returnHome() model {
	model = model.closeNetworkOverlay()
	model.searching = false
	model.filter.SetValue("")
	model.filter.Blur()
	model.filtering = false
	model.filterSearching = false
	model.filterMenuSearch.SetValue("")
	model.filterMenuSearch.Blur()
	model.filterOptionsFocused = false
	model.filterCursor = 0
	model.infoOverlay = false
	model.cursor = 0
	return model
}

func (model model) isQuitKey(key string) bool {
	return key == "ctrl+c" || key == "esc"
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
