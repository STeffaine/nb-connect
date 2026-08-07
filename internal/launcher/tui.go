package launcher

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/steffaine/nb-connect/internal/netbox"
)

type Selection struct {
	Service  netbox.Service
	Endpoint string
}

var ErrSelectionCancelled = errors.New("service selection cancelled")

type SyncServices func(context.Context) ([]netbox.Service, error)

var (
	headingStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	selectedRowStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusActiveStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	statusPendingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	statusProblemStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
)

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
	choices   []Selection
	filter    textinput.Model
	cursor    int
	searching bool
	syncing   bool
	pinging   bool
	syncError string
	syncNote  string
	pingNote  string
	context   context.Context
	sync      SyncServices
	pingLines chan pingMessage
	pingCount int
	selection *Selection
	cancelled bool
	favorites map[string]bool
	recents   []string
	statePath string
}

type syncResult struct {
	services []netbox.Service
	err      error
}

type pingMessage struct {
	line string
	err  error
	done bool
}

func newModel(ctx context.Context, services []netbox.Service, syncServices SyncServices) (model, error) {
	return newModelWithPingCount(ctx, services, 4, syncServices)
}

func newModelWithPingCount(ctx context.Context, services []netbox.Service, pingCount int, syncServices SyncServices) (model, error) {
	choices, err := choicesForServices(services)
	if err != nil {
		return model{}, err
	}
	filter := textinput.New()
	filter.Prompt = "Search: "
	statePath := defaultLauncherStatePath()
	favorites, recents, err := loadLauncherState(statePath)
	if err != nil {
		favorites = map[string]bool{}
		recents = nil
	}
	return model{choices: choices, context: ctx, filter: filter, sync: syncServices, pingCount: pingCount, favorites: favorites, recents: recents, statePath: statePath}, nil
}

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

func (model model) Init() tea.Cmd {
	return textinput.Blink
}

func (model model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case pingMessage:
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
	case syncResult:
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
		model.cursor = 0
		model.filter.SetValue("")
		model.syncError = ""
		model.syncNote = fmt.Sprintf("Synced %d services", len(message.services))
		return model, nil
	case tea.KeyMsg:
		if model.searching {
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

		if shortcutIndex, ok := numberShortcut(message.String()); ok {
			visible := model.visibleChoices()
			if shortcutIndex < len(visible) {
				selection := visible[shortcutIndex]
				model.selection = &selection
				model = model.recordSelection(selection)
				return model, tea.Quit
			}
			return model, nil
		}

		switch message.String() {
		case "ctrl+c", "esc", "q":
			model.cancelled = true
			return model, tea.Quit
		case "f":
			model.searching = true
			return model, model.filter.Focus()
		case "s":
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
		case "enter":
			visible := model.visibleChoices()
			if len(visible) > 0 {
				selection := visible[model.cursor]
				model.selection = &selection
				model = model.recordSelection(selection)
				return model, tea.Quit
			}
		case "m":
			visible := model.visibleChoices()
			if len(visible) > 0 {
				selection := visible[model.cursor]
				model = model.toggleFavorite(selection)
			}
			return model, nil
		case "p":
			visible := model.visibleChoices()
			if len(visible) > 0 && !model.pinging {
				selection := visible[model.cursor]
				model.pinging = true
				model.pingNote = ""
				model.pingLines = make(chan pingMessage, 16)
				return model, startPing(model.context, selection.Endpoint, model.pingCount, model.pingLines)
			}
			return model, nil
		case "l":
			if len(model.recents) > 0 {
				selection, ok := model.selectionForKey(model.recents[0])
				if ok {
					model.selection = &selection
					model = model.recordSelection(selection)
					return model, tea.Quit
				}
			}
			return model, nil
		case "up", "k":
			if model.cursor > 0 {
				model.cursor--
			}
			return model, nil
		case "down", "j":
			if model.cursor+1 < len(model.visibleChoices()) {
				model.cursor++
			}
			return model, nil
		}
	}
	return model, nil
}

func (model model) View() string {
	var output strings.Builder
	output.WriteString("nbcon services\n")
	if model.searching {
		output.WriteString(model.filter.View())
		output.WriteString("\n\n")
	} else if query := strings.TrimSpace(model.filter.Value()); query != "" {
		fmt.Fprintf(&output, "Filter: %s\n\n", query)
	} else {
		output.WriteString("\n")
	}
	visible := model.visibleChoices()
	if model.syncing {
		output.WriteString("Syncing services from NetBox...\n\n")
	} else if model.pinging {
		output.WriteString("Pinging selected endpoint...\n")
		if model.pingNote != "" {
			fmt.Fprintf(&output, "%s\n", model.pingNote)
		}
		output.WriteString("\n")
	} else if model.syncError != "" {
		fmt.Fprintf(&output, "Sync failed: %s\n\n", model.syncError)
	} else if model.syncNote != "" {
		fmt.Fprintf(&output, "%s\n\n", model.syncNote)
	} else if model.pingNote != "" {
		fmt.Fprintf(&output, "Ping:\n%s\n\n", model.pingNote)
	}
	if len(visible) == 0 {
		output.WriteString("No matching services\n")
	} else {
		widths := model.columnWidths()
		output.WriteString(headingStyle.Render(fmt.Sprintf("      %-*s %-*s %s", widths[0], "TARGET", widths[1], "SERVICE", "ENDPOINT")))
		output.WriteString("\n")
		lastPriority := -1
		for index, selection := range visible {
			priority := model.priorityFor(selection)
			if priority != lastPriority {
				if lastPriority != -1 {
					output.WriteString("\n")
				}
				output.WriteString(sectionHeading(priority))
				output.WriteString("\n")
				lastPriority = priority
			}
			prefix := "  "
			if index == model.cursor {
				prefix = "> "
			}
			shortcut := "  "
			if index < 9 {
				shortcut = fmt.Sprintf("%d ", index+1)
			}
			favorite := " "
			if model.favorites[selectionKey(selection)] {
				favorite = "*"
			}
			row := fmt.Sprintf("%s%s%s %-*s %-*s %s", prefix, shortcut, favorite, widths[0], selection.Service.TargetName(), widths[1], selection.Service.Name, selection.Endpoint)
			if index == model.cursor {
				row = selectedRowStyle.Render(row)
			}
			output.WriteString(row)
			output.WriteString("\n")
		}
		model.writeSelectionDetails(&output, visible[model.cursor])
	}
	if model.searching {
		output.WriteString("\nEnter apply | Esc clear and return\n")
	} else {
		output.WriteString("\n1-9 connect | Enter connect | m favorite | l last used | p ping | f search | s sync | j/k or arrows move | Esc cancel\n")
	}
	return output.String()
}

func numberShortcut(key string) (int, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	return int(key[0] - '1'), true
}

func startPing(ctx context.Context, endpoint string, count int, messages chan pingMessage) tea.Cmd {
	return func() tea.Msg {
		host, err := pingHost(endpoint)
		if err != nil {
			return pingMessage{err: err}
		}
		go streamPing(ctx, host, count, messages)
		return <-messages
	}
}

func waitForPingMessage(messages <-chan pingMessage) tea.Cmd {
	return func() tea.Msg {
		return <-messages
	}
}

func streamPing(ctx context.Context, host string, count int, messages chan<- pingMessage) {
	defer close(messages)
	command := exec.CommandContext(ctx, "ping", "-c", fmt.Sprintf("%d", count), host)
	output, err := command.StdoutPipe()
	if err != nil {
		messages <- pingMessage{err: err}
		return
	}
	command.Stderr = command.Stdout
	if err := command.Start(); err != nil {
		messages <- pingMessage{err: err}
		return
	}
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		messages <- pingMessage{line: scanner.Text()}
	}
	if err := scanner.Err(); err != nil {
		messages <- pingMessage{err: err}
		return
	}
	messages <- pingMessage{err: command.Wait(), done: true}
}

func pingHost(endpoint string) (string, error) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return "", fmt.Errorf("invalid endpoint %q", endpoint)
	}
	return host, nil
}

func (model model) columnWidths() [2]int {
	widths := [2]int{len("TARGET"), len("SERVICE")}
	for _, selection := range model.choices {
		fields := [2]string{selection.Service.TargetName(), selection.Service.Name}
		for index, field := range fields {
			widths[index] = max(widths[index], len(field))
		}
	}
	return widths
}

func (model model) writeSelectionDetails(output *strings.Builder, selection Selection) {
	service := selection.Service
	favorite := "no"
	if model.favorites[selectionKey(selection)] {
		favorite = "yes"
	}
	fmt.Fprintf(output, "\nDetails: favorite: %s | role: %s | tenant: %s | status: %s\n", favorite, valueOrUnknown(service.Role), valueOrUnknown(service.Tenant), statusStyle(service.Status).Render(valueOrUnknown(service.Status)))
}

func statusStyle(status string) lipgloss.Style {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return statusActiveStyle
	case "planned", "pending", "staged":
		return statusPendingStyle
	case "failed", "offline", "decommissioning":
		return statusProblemStyle
	default:
		return lipgloss.NewStyle()
	}
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func (model model) visibleChoices() []Selection {
	query := strings.ToLower(strings.TrimSpace(model.filter.Value()))
	choices := make([]Selection, 0, len(model.choices))
	for _, selection := range model.choices {
		fields := []string{selection.Service.TargetName(), selection.Service.Name, selection.Endpoint, selection.Service.Role, selection.Service.Tenant, selection.Service.Status}
		candidate := strings.ToLower(strings.Join(fields, " "))
		matches := fuzzy.Find(query, []string{candidate})
		if query == "" || strings.Contains(candidate, query) || len(matches) > 0 {
			choices = append(choices, selection)
		}
	}
	if len(choices) == 0 {
		return nil
	}
	sort.SliceStable(choices, func(i, j int) bool {
		left := choices[i]
		right := choices[j]
		leftPriority := model.priorityFor(left)
		rightPriority := model.priorityFor(right)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return model.choiceIndex(left) < model.choiceIndex(right)
	})
	return choices
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
		return "Services"
	}
}

func (model model) choiceIndex(selection Selection) int {
	for index, candidate := range model.choices {
		if selectionKey(candidate) == selectionKey(selection) {
			return index
		}
	}
	return len(model.choices)
}

func (model model) favoriteSelections() []Selection {
	var favorites []Selection
	for _, selection := range model.choices {
		if model.favorites[selectionKey(selection)] {
			favorites = append(favorites, selection)
		}
	}
	return favorites
}

func (model model) selectionForKey(key string) (Selection, bool) {
	for _, selection := range model.choices {
		if selectionKey(selection) == key {
			return selection, true
		}
	}
	return Selection{}, false
}

func (model model) recentSelections() []Selection {
	var selections []Selection
	for _, key := range model.recents {
		for _, selection := range model.choices {
			if selectionKey(selection) == key {
				selections = append(selections, selection)
				break
			}
		}
	}
	return selections
}

func (model model) isRecent(key string) bool {
	for _, recent := range model.recents {
		if recent == key {
			return true
		}
	}
	return false
}

func (model model) recordSelection(selection Selection) model {
	key := selectionKey(selection)
	if key == "" {
		return model
	}
	if model.favorites == nil {
		model.favorites = map[string]bool{}
	}
	model.recents = append([]string{key}, model.recents...)
	seen := make(map[string]struct{}, len(model.recents))
	filtered := make([]string, 0, len(model.recents))
	for _, recent := range model.recents {
		if recent == "" {
			continue
		}
		if _, exists := seen[recent]; exists {
			continue
		}
		seen[recent] = struct{}{}
		filtered = append(filtered, recent)
		if len(filtered) == 8 {
			break
		}
	}
	model.recents = filtered
	_ = saveLauncherState(model.statePath, model.favorites, model.recents)
	return model
}

func (model model) toggleFavorite(selection Selection) model {
	key := selectionKey(selection)
	if key == "" {
		return model
	}
	if model.favorites == nil {
		model.favorites = map[string]bool{}
	}
	if model.favorites[key] {
		delete(model.favorites, key)
	} else {
		model.favorites[key] = true
	}
	_ = saveLauncherState(model.statePath, model.favorites, model.recents)
	return model
}

func selectionKey(selection Selection) string {
	parts := []string{strings.ToLower(strings.TrimSpace(selection.Service.TargetName())), strings.ToLower(strings.TrimSpace(selection.Service.Name)), strings.ToLower(strings.TrimSpace(selection.Endpoint))}
	return strings.Join(parts, "::")
}

func defaultLauncherStatePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cacheDir, "nb-connect", "launcher-state.json")
}

func loadLauncherState(path string) (map[string]bool, []string, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]bool{}, nil, nil
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]bool{}, nil, nil
		}
		return nil, nil, err
	}
	var state struct {
		Favorites map[string]bool `json:"favorites"`
		Recents   []string        `json:"recents"`
	}
	if err := json.Unmarshal(contents, &state); err != nil {
		return nil, nil, err
	}
	if state.Favorites == nil {
		state.Favorites = map[string]bool{}
	}
	return state.Favorites, state.Recents, nil
}

func saveLauncherState(path string, favorites map[string]bool, recents []string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(struct {
		Favorites map[string]bool `json:"favorites"`
		Recents   []string        `json:"recents"`
	}{Favorites: favorites, Recents: recents}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0o600)
}
