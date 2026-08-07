package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
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

func Select(ctx context.Context, services []netbox.Service, syncServices SyncServices) (Selection, error) {
	selector, err := newModel(ctx, services, syncServices)
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
	syncError string
	syncNote  string
	context   context.Context
	sync      SyncServices
	selection *Selection
	cancelled bool
}

type syncResult struct {
	services []netbox.Service
	err      error
}

func newModel(ctx context.Context, services []netbox.Service, syncServices SyncServices) (model, error) {
	choices, err := choicesForServices(services)
	if err != nil {
		return model{}, err
	}
	filter := textinput.New()
	filter.Prompt = "Search: "
	return model{choices: choices, context: ctx, filter: filter, sync: syncServices}, nil
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
				return model, tea.Quit
			}
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
	} else if model.syncError != "" {
		fmt.Fprintf(&output, "Sync failed: %s\n\n", model.syncError)
	} else if model.syncNote != "" {
		fmt.Fprintf(&output, "%s\n\n", model.syncNote)
	}
	if len(visible) == 0 {
		output.WriteString("No matching services\n")
	} else {
		widths := model.columnWidths()
		output.WriteString(headingStyle.Render(fmt.Sprintf("    %-*s %-*s %s", widths[0], "TARGET", widths[1], "SERVICE", "ENDPOINT")))
		output.WriteString("\n")
		for index, selection := range visible {
			prefix := "  "
			if index == model.cursor {
				prefix = "> "
			}
			shortcut := "  "
			if index < 9 {
				shortcut = fmt.Sprintf("%d ", index+1)
			}
			row := fmt.Sprintf("%s%s%-*s %-*s %s", prefix, shortcut, widths[0], selection.Service.TargetName(), widths[1], selection.Service.Name, selection.Endpoint)
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
		output.WriteString("\n1-9 connect | Enter connect | f search | s sync | j/k or arrows move | Esc cancel\n")
	}
	return output.String()
}

func numberShortcut(key string) (int, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	return int(key[0] - '1'), true
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
	fmt.Fprintf(output, "\nDetails: role: %s | tenant: %s | status: %s\n", valueOrUnknown(service.Role), valueOrUnknown(service.Tenant), statusStyle(service.Status).Render(valueOrUnknown(service.Status)))
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
	if query == "" {
		return model.choices
	}
	candidates := make([]string, 0, len(model.choices))
	for _, selection := range model.choices {
		fields := []string{selection.Service.TargetName(), selection.Service.Name, selection.Endpoint, selection.Service.Role, selection.Service.Tenant, selection.Service.Status}
		candidates = append(candidates, strings.ToLower(strings.Join(fields, " ")))
	}
	matches := fuzzy.Find(query, candidates)
	visible := make([]Selection, 0, len(matches))
	for _, match := range matches {
		visible = append(visible, model.choices[match.Index])
	}
	return visible
}
