package launcher

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	headingStyle       = "\x1b[1;38;5;39m"
	selectedRowStyle   = "\x1b[1;38;5;39m"
	statusActiveStyle  = "\x1b[1;38;5;42m"
	statusPendingStyle = "\x1b[1;38;5;214m"
	statusProblemStyle = "\x1b[1;38;5;196m"
)

const ansiReset = "\x1b[0m"

func (model model) View() string {
	var output strings.Builder
	if model.searching {
		output.WriteString(model.filter.View())
		output.WriteString("\n\n")
	} else if query := strings.TrimSpace(model.filter.Value()); query != "" {
		fmt.Fprintf(&output, "Filter: %s\n\n", query)
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
		showServer := model.hasMultipleServers()
		start, end := model.visibleRange(len(visible))
		endpointWidth := model.endpointWidth(widths, showServer)
		if showServer {
			output.WriteString(renderStyled(headingStyle, fmt.Sprintf("      %-*s %-*s %-*s %s", widths[1], "TARGET", widths[2], "SERVICE", endpointWidth, "ENDPOINT", "SERVER")))
		} else {
			output.WriteString(renderStyled(headingStyle, fmt.Sprintf("      %-*s %-*s %s", widths[1], "TARGET", widths[2], "SERVICE", "ENDPOINT")))
		}
		output.WriteString("\n")
		lastPriority := -1
		for index := start; index < end; index++ {
			selection := visible[index]
			priority := model.priorityFor(selection)
			if priority != lastPriority {
				if lastPriority != -1 {
					output.WriteString("\n")
				}
				if heading := sectionHeading(priority); heading != "" {
					output.WriteString(heading)
					output.WriteString("\n")
				}
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
			var row string
			if showServer {
				row = fmt.Sprintf("%s%s%s %-*s %-*s %-*s %s", prefix, shortcut, favorite, widths[1], truncate(selection.Service.TargetName(), widths[1]), widths[2], truncate(selection.Service.Name, widths[2]), endpointWidth, truncate(selection.Endpoint, endpointWidth), truncate(selection.Service.Server, widths[0]))
			} else {
				row = fmt.Sprintf("%s%s%s %-*s %-*s %s", prefix, shortcut, favorite, widths[1], truncate(selection.Service.TargetName(), widths[1]), widths[2], truncate(selection.Service.Name, widths[2]), truncate(selection.Endpoint, endpointWidth))
			}
			if index == model.cursor {
				row = renderStyled(selectedRowStyle, row)
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
	if model.pinging || model.pingNote != "" {
		model.writePingPopup(&output)
	}
	return output.String()
}

func (model model) writePingPopup(output *strings.Builder) {
	title := "Ping results"
	if model.pinging {
		title = "Ping in progress"
	}
	lines := []string{"Waiting for ping output..."}
	if model.pingNote != "" {
		lines = strings.Split(model.pingNote, "\n")
	}
	width := len(title)
	for _, line := range lines {
		width = max(width, len(line))
	}
	if model.width > 0 {
		width = min(width, max(1, model.width-4))
	}

	output.WriteString("\n+")
	output.WriteString(strings.Repeat("-", width+2))
	output.WriteString("+\n")
	fmt.Fprintf(output, "| %-*s |\n", width, truncate(title, width))
	output.WriteString("+")
	output.WriteString(strings.Repeat("-", width+2))
	output.WriteString("+\n")
	for _, line := range lines {
		fmt.Fprintf(output, "| %-*s |\n", width, truncate(line, width))
	}
	output.WriteString("+")
	output.WriteString(strings.Repeat("-", width+2))
	output.WriteString("+\n")
}

type searchInput struct {
	value   string
	focused bool
}

func (input *searchInput) SetValue(value string) {
	input.value = value
}

func (input searchInput) Value() string {
	return input.value
}

func (input *searchInput) Focus() tea.Cmd {
	input.focused = true
	return nil
}

func (input *searchInput) Blur() {
	input.focused = false
}

func (input searchInput) Focused() bool {
	return input.focused
}

func (input searchInput) View() string {
	return "Search: " + input.value
}

func (input searchInput) Update(message tea.KeyMsg) (searchInput, tea.Cmd) {
	switch message.Type {
	case tea.KeyRunes:
		input.value += string(message.Runes)
	case tea.KeyBackspace, tea.KeyDelete:
		if len(input.value) > 0 {
			_, size := utf8.DecodeLastRuneInString(input.value)
			input.value = input.value[:len(input.value)-size]
		}
	}
	return input, nil
}

func renderStyled(style, value string) string {
	if style == "" || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" {
		return value
	}
	return style + value + ansiReset
}

func (model model) columnWidths() [3]int {
	widths := [3]int{len("SERVER"), len("TARGET"), len("SERVICE")}
	for _, selection := range model.choices {
		fields := [3]string{selection.Service.Server, selection.Service.TargetName(), selection.Service.Name}
		for index, field := range fields {
			widths[index] = max(widths[index], len(field))
		}
	}
	if model.width > 0 && model.hasMultipleServers() {
		maximumFields := max(3, model.width-20)
		if widths[0]+widths[1]+widths[2] > maximumFields {
			widths[0] = max(1, maximumFields/4)
			widths[1] = max(1, maximumFields/2)
			widths[2] = max(1, maximumFields-widths[0]-widths[1])
		}
	} else if model.width > 0 {
		maximumFields := max(2, model.width-16)
		if widths[1]+widths[2] > maximumFields {
			widths[1] = max(1, maximumFields*2/3)
			widths[2] = max(1, maximumFields-widths[1])
		}
	}
	return widths
}

func (model model) endpointWidth(widths [3]int, showServer bool) int {
	if model.width <= 0 {
		return 1 << 30
	}
	if !showServer {
		return max(1, model.width-8-widths[1]-widths[2])
	}
	return max(1, model.width-12-widths[0]-widths[1]-widths[2])
}

func (model model) hasMultipleServers() bool {
	servers := make(map[string]struct{})
	for _, selection := range model.choices {
		server := strings.ToLower(strings.TrimSpace(selection.Service.Server))
		if server == "" {
			continue
		}
		servers[server] = struct{}{}
		if len(servers) > 1 {
			return true
		}
	}
	return false
}

func (model model) visibleRange(choiceCount int) (int, int) {
	if model.height <= 0 || choiceCount == 0 {
		return 0, choiceCount
	}
	rowCount := max(1, model.height-10)
	if choiceCount <= rowCount {
		return 0, choiceCount
	}
	start := max(0, model.cursor-rowCount/2)
	end := start + rowCount
	if end > choiceCount {
		end = choiceCount
		start = end - rowCount
	}
	return start, end
}

func truncate(value string, width int) string {
	if width < 1 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return string(runes[:1])
	}
	return string(runes[:width-1]) + "~"
}

func (model model) writeSelectionDetails(output *strings.Builder, selection Selection) {
	service := selection.Service
	favorite := "no"
	if model.favorites[selectionKey(selection)] {
		favorite = "yes"
	}
	if service.Server != "" {
		fmt.Fprintf(output, "\nDetails: server: %s | favorite: %s | role: %s | tenant: %s | status: %s\n", service.Server, favorite, valueOrUnknown(service.Role), valueOrUnknown(service.Tenant), renderStyled(statusStyle(service.Status), valueOrUnknown(service.Status)))
		return
	}
	fmt.Fprintf(output, "\nDetails: favorite: %s | role: %s | tenant: %s | status: %s\n", favorite, valueOrUnknown(service.Role), valueOrUnknown(service.Tenant), renderStyled(statusStyle(service.Status), valueOrUnknown(service.Status)))
}

func statusStyle(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return statusActiveStyle
	case "planned", "pending", "staged":
		return statusPendingStyle
	case "failed", "offline", "decommissioning":
		return statusProblemStyle
	default:
		return ""
	}
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
