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
		start, end := model.visibleRange(len(visible))
		endpointWidth := model.endpointWidth(widths)
		output.WriteString(renderStyled(headingStyle, fmt.Sprintf("      %-*s %-*s %s", widths[0], "TARGET", widths[1], "SERVICE", "ENDPOINT")))
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
			row := fmt.Sprintf("%s%s%s %-*s %-*s %s", prefix, shortcut, favorite, widths[0], truncate(selection.Service.TargetName(), widths[0]), widths[1], truncate(selection.Service.Name, widths[1]), truncate(selection.Endpoint, endpointWidth))
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

func (model model) columnWidths() [2]int {
	widths := [2]int{len("TARGET"), len("SERVICE")}
	for _, selection := range model.choices {
		fields := [2]string{selection.Service.TargetName(), selection.Service.Name}
		for index, field := range fields {
			widths[index] = max(widths[index], len(field))
		}
	}
	if model.width > 0 {
		maximumFields := max(2, model.width-16)
		if widths[0]+widths[1] > maximumFields {
			widths[0] = max(1, maximumFields*2/3)
			widths[1] = max(1, maximumFields-widths[0])
		}
	}
	return widths
}

func (model model) endpointWidth(widths [2]int) int {
	if model.width <= 0 {
		return 1 << 30
	}
	return max(1, model.width-8-widths[0]-widths[1])
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
