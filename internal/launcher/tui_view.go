package launcher

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	headingStyle       = "\x1b[1;38;5;39m"
	infoLabelStyle     = "\x1b[1;38;5;81m"
	selectedRowStyle   = "\x1b[1;38;5;39m"
	statusActiveStyle  = "\x1b[1;38;5;42m"
	statusPendingStyle = "\x1b[1;38;5;214m"
	statusProblemStyle = "\x1b[1;38;5;196m"
)

const minDockInfoPanelWidth = 28

const ansiReset = "\x1b[0m"

var ansiSequence = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const (
	infoBoxTopLeft     = "┌"
	infoBoxTopRight    = "┐"
	infoBoxDividerLeft = "├"
	infoBoxDividerRight = "┤"
	infoBoxBottomLeft  = "└"
	infoBoxBottomRight = "┘"
	infoBoxHorizontal  = "─"
	infoBoxVertical    = "│"
	infoBoxTopPadding  = 1
)

func (model model) View() string {
	var output strings.Builder
	if model.filtering {
		model.writeFilterMenu(&output)
		return output.String()
	}
	if model.infoOverlay {
		return model.renderInfoOverlay()
	}
	if model.infoOpen && model.canDockInfoPanel() {
		return model.renderSplitView()
	}
	if model.searching {
		output.WriteString(model.filter.View())
		output.WriteString("\n\n")
	} else if query := strings.TrimSpace(model.filter.Value()); query != "" {
		fmt.Fprintf(&output, "Filter: %s\n\n", query)
	}
	if summary := model.filterSummary(); summary != "" {
		fmt.Fprintf(&output, "%s\n\n", summary)
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
		if model.infoOpen {
			model.writeInfoPopup(&output, visible[model.cursor])
		}
	}
	if model.searching {
		output.WriteString("\nEnter apply | Esc clear and return\n")
	} else {
		footer := "\n1-9 connect | Enter connect | m favorite | l last used | p ping | i info | / search | f filters | s sync | j/k or arrows move | Esc cancel\n"
		if model.infoOpen {
			footer = "\n1-9 connect | Enter connect | m favorite | l last used | p ping | i close | / search | f filters | s sync | j/k or arrows move | Esc close\n"
		}
		output.WriteString(footer)
	}
	if model.pinging || model.pingNote != "" {
		model.writePingPopup(&output)
	}
	return output.String()
}

func (model model) writeFilterMenu(output *strings.Builder) {
	mode := "all conditions"
	if !model.filters.matchAll {
		mode = "any condition"
	}
	fmt.Fprintf(output, "Filters: %s\n", mode)
	for index, category := range filterCategories {
		prefix := "  "
		if !model.filterOptionsFocused && index == model.filterCategory {
			prefix = "> "
		}
		fmt.Fprintf(output, "%s%s\n", prefix, category)
	}
	searchPrompt := "Press / to search"
	if model.filterSearching {
		searchPrompt = "Search"
	}
	fmt.Fprintf(output, "\n%s %s: %s\n\n", searchPrompt, filterCategories[model.filterCategory], model.filterMenuSearch.Value())
	options := model.filterOptions()
	if len(options) == 0 {
		output.WriteString("No matching values\n")
	}
	for index, option := range options {
		prefix := "  "
		if model.filterOptionsFocused && index == model.filterCursor {
			prefix = "> "
		}
		marker := " "
		if model.filters.values[filterCategories[model.filterCategory]][option] {
			marker = "x"
		}
		fmt.Fprintf(output, "%s[%s] %s\n", prefix, marker, option)
	}
	if model.filterSearching {
		output.WriteString("\nType to search | arrows move | Space select more | Enter select | Esc clear\n")
		return
	}
	if model.filterOptionsFocused {
		output.WriteString("\nTab filters | / search values | j/k or arrows move | Space/Enter select | a all/any | Esc close\n")
		return
	}
	output.WriteString("\nTab options | j/k or arrows category | / search values | a all/any | Esc close\n")
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
		fmt.Fprintf(output, "\nDetails: server: %s | favorite: %s | role: %s | tenant: %s | status: %s | description: %s\n", service.Server, favorite, valueOrUnknown(service.Role), valueOrUnknown(service.Tenant), renderStyled(statusStyle(service.Status), valueOrUnknown(service.Status)), valueOrUnknown(service.Description))
		return
	}
	fmt.Fprintf(output, "\nDetails: favorite: %s | role: %s | tenant: %s | status: %s | description: %s\n", favorite, valueOrUnknown(service.Role), valueOrUnknown(service.Tenant), renderStyled(statusStyle(service.Status), valueOrUnknown(service.Status)), valueOrUnknown(service.Description))
}

func (model model) writeInfoPopup(output *strings.Builder, selection Selection) {
	for _, line := range model.infoPanelBoxLines(selection, 0, 0) {
		output.WriteString(line)
		output.WriteString("\n")
	}
}

func (model model) renderInfoOverlay() string {
	var output strings.Builder
	selection, ok := model.currentChoice()
	if !ok {
		return ""
	}
	for _, line := range model.infoPanelBoxLines(selection, 0, 0) {
		output.WriteString(line)
		output.WriteString("\n")
	}
	output.WriteString("\ni close | Enter connect | Esc close\n")
	return output.String()
}

func (model model) renderSplitView() string {
	selection, ok := model.currentChoice()
	if !ok {
		return model.renderListView()
	}
	panelWidth := model.dockedInfoPanelWidth()
	panelLines := model.infoPanelBoxLines(selection, panelWidth, model.dockedInfoPanelHeight())
	leftWidth := model.requiredListWidth(model.hasMultipleServers())
	leftBody := strings.Split(strings.TrimSuffix(model.renderListBody(leftWidth, false, true), "\n"), "\n")
	anchorIndex := 0
	for index, line := range leftBody {
		if strings.Contains(line, "TARGET") {
			anchorIndex = index
			break
		}
	}
	if anchorIndex > 0 {
		anchorIndex--
	}
	rightBody := append(make([]string, anchorIndex), panelLines...)
	combined := combineColumns(leftBody, rightBody, leftWidth, 3)
	combined = append(combined, "")
	combined = append(combined, strings.Split(strings.TrimSuffix(model.renderFooter(), "\n"), "\n")...)
	return strings.Join(combined, "\n")
}

func (model model) renderListView() string {
	var output strings.Builder
	output.WriteString(model.renderListBody(model.width, true, false))
	output.WriteString(model.renderFooter())
	if model.pinging || model.pingNote != "" {
		model.writePingPopup(&output)
	}
	return output.String()
}

func (model model) renderListBody(width int, includeDetails bool, fixedColumns bool) string {
	subject := model
	subject.width = width
	var output strings.Builder
	if subject.searching {
		output.WriteString(subject.filter.View())
		output.WriteString("\n\n")
	} else if query := strings.TrimSpace(subject.filter.Value()); query != "" {
		fmt.Fprintf(&output, "Filter: %s\n\n", query)
	}
	if summary := subject.filterSummary(); summary != "" {
		fmt.Fprintf(&output, "%s\n\n", summary)
	}
	visible := subject.visibleChoices()
	if subject.syncing {
		output.WriteString("Syncing services from NetBox...\n\n")
	} else if subject.syncError != "" {
		fmt.Fprintf(&output, "Sync failed: %s\n\n", subject.syncError)
	} else if subject.syncNote != "" {
		fmt.Fprintf(&output, "%s\n\n", subject.syncNote)
	}
	if len(visible) == 0 {
		output.WriteString("No matching services\n")
		return output.String()
	}
	var widths [3]int
	var endpointWidth int
	if fixedColumns {
		widths, endpointWidth = subject.dockedListMetrics()
	} else {
		widths = subject.columnWidths()
		endpointWidth = subject.endpointWidth(widths, subject.hasMultipleServers())
	}
	showServer := subject.hasMultipleServers()
	start, end := subject.visibleRange(len(visible))
	if showServer {
		output.WriteString(renderStyled(headingStyle, fmt.Sprintf("      %-*s %-*s %-*s %s", widths[1], "TARGET", widths[2], "SERVICE", endpointWidth, "ENDPOINT", "SERVER")))
	} else {
		output.WriteString(renderStyled(headingStyle, fmt.Sprintf("      %-*s %-*s %s", widths[1], "TARGET", widths[2], "SERVICE", "ENDPOINT")))
	}
	output.WriteString("\n")
	lastPriority := -1
	for index := start; index < end; index++ {
		selection := visible[index]
		priority := subject.priorityFor(selection)
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
		if index == subject.cursor {
			prefix = "> "
		}
		shortcut := "  "
		if index < 9 {
			shortcut = fmt.Sprintf("%d ", index+1)
		}
		favorite := " "
		if subject.favorites[selectionKey(selection)] {
			favorite = "*"
		}
		var row string
		if showServer {
			row = fmt.Sprintf("%s%s%s %-*s %-*s %-*s %s", prefix, shortcut, favorite, widths[1], truncate(selection.Service.TargetName(), widths[1]), widths[2], truncate(selection.Service.Name, widths[2]), endpointWidth, truncate(selection.Endpoint, endpointWidth), truncate(selection.Service.Server, widths[0]))
		} else {
			row = fmt.Sprintf("%s%s%s %-*s %-*s %s", prefix, shortcut, favorite, widths[1], truncate(selection.Service.TargetName(), widths[1]), widths[2], truncate(selection.Service.Name, widths[2]), truncate(selection.Endpoint, endpointWidth))
		}
		if index == subject.cursor {
			row = renderStyled(selectedRowStyle, row)
		}
		output.WriteString(row)
		output.WriteString("\n")
	}
	if includeDetails {
		subject.writeSelectionDetails(&output, visible[subject.cursor])
	}
	return output.String()
}

func (model model) renderFooter() string {
	if model.searching {
		return "\nEnter apply | Esc clear and return\n"
	}
	if model.infoOverlay {
		return "\ni close | Enter connect | Esc close\n"
	}
	if model.infoOpen {
		return "\n1-9 connect | Enter connect | m favorite | l last used | p ping | i close | / search | f filters | s sync | j/k or arrows move | Esc close\n"
	}
	return "\n1-9 connect | Enter connect | m favorite | l last used | p ping | i info | / search | f filters | s sync | j/k or arrows move | Esc cancel\n"
}

func (model model) canDockInfoPanel() bool {
	if _, ok := model.currentChoice(); !ok || model.width <= 0 {
		return false
	}
	panelWidth := model.dockedInfoPanelWidth()
	return panelWidth >= minDockInfoPanelWidth
}

func (model model) dockedInfoPanelHeight() int {
	if model.height <= 0 {
		return 0
	}
	return max(4, model.height-2)
}

func (model model) dockedInfoPanelWidth() int {
	if model.width <= 0 {
		return 0
	}
	width := model.width - model.requiredListWidth(model.hasMultipleServers()) - 3
	if width < minDockInfoPanelWidth {
		return 0
	}
	return width
}

func (model model) requiredListWidth(showServer bool) int {
	if len(model.choices) == 0 {
		return 0
	}
	widths, endpointWidth := model.dockedListMetrics()
	if showServer {
		return 9 + widths[1] + widths[2] + endpointWidth + widths[0]
	}
		return 8 + widths[1] + widths[2] + endpointWidth
}

func (model model) dockedListMetrics() ([3]int, int) {
	widths := [3]int{len("SERVER"), len("TARGET"), len("SERVICE")}
	endpointWidth := len("ENDPOINT")
	for _, selection := range model.choices {
		widths[0] = max(widths[0], len(selection.Service.Server))
		widths[1] = max(widths[1], len(selection.Service.TargetName()))
		widths[2] = max(widths[2], len(selection.Service.Name))
		endpointWidth = max(endpointWidth, len(selection.Endpoint))
	}
	return widths, endpointWidth
}

func (model model) infoPanelEntries(selection Selection) []struct {
	label string
	value string
	style string
	indent int
} {
	service := selection.Service
	return []struct {
		label string
		value string
		style string
		indent int
	}{
		{label: "server", value: valueOrUnknown(service.Server), style: infoLabelStyle},
		{label: "target", value: valueOrUnknown(service.TargetName()), style: infoLabelStyle},
		{label: "device", value: valueOrUnknown(service.Device), style: infoLabelStyle},
		{label: "vm", value: valueOrUnknown(service.VM), style: infoLabelStyle},
		{label: "service", value: valueOrUnknown(service.Name), style: infoLabelStyle},
		{label: "protocol", value: valueOrUnknown(service.Protocol), style: infoLabelStyle},
		{label: "ports", value: joinOrUnknown(service.Ports), style: infoLabelStyle},
		{label: "ips", value: joinOrUnknown(service.IPs), style: infoLabelStyle},
		{label: "site", value: valueOrUnknown(service.Site), style: infoLabelStyle},
		{label: "role", value: valueOrUnknown(service.Role), style: infoLabelStyle},
		{label: "tenant", value: valueOrUnknown(service.Tenant), style: infoLabelStyle},
		{label: "platform", value: valueOrUnknown(service.Platform), style: infoLabelStyle},
		{label: "tags", value: joinOrUnknown(service.Tags), style: infoLabelStyle},
		{label: "status", value: valueOrUnknown(service.Status), style: infoLabelStyle},
		{label: "description", value: valueOrUnknown(service.Description), style: infoLabelStyle},
		{label: "endpoint", value: valueOrUnknown(selection.Endpoint), style: infoLabelStyle},
		{label: "favorite", value: yesNo(model.favorites[selectionKey(selection)]), style: infoLabelStyle},
	}
}

func (model model) infoPanelBoxLines(selection Selection, width, height int) []string {
	entries := model.infoPanelEntries(selection)
	title := "Service information"
	if width <= 0 {
		width = max(1, model.width-4)
	}
	contentWidth := width - 4
	if contentWidth < 1 {
		contentWidth = 1
	}
	if width <= 0 {
		width = len(title)
		contentWidth = width
	}
	contentHeight := 0
	if height > 0 {
		contentHeight = max(0, height-4-infoBoxTopPadding)
	}
	box := make([]string, 0, len(entries)+4)
	for index := 0; index < infoBoxTopPadding; index++ {
		box = append(box, "")
	}
	box = append(box, infoBoxTopLeft+strings.Repeat(infoBoxHorizontal, contentWidth+2)+infoBoxTopRight)
	box = append(box, formatInfoBoxLine(renderStyled(headingStyle, title), contentWidth))
	box = append(box, infoBoxDividerLeft+strings.Repeat(infoBoxHorizontal, contentWidth+2)+infoBoxDividerRight)
	contentLines := make([]string, 0, len(entries))
	for _, entry := range entries {
		contentLines = append(contentLines, formatInfoFieldLines(entry.label, entry.value, entry.style, contentWidth)...)
	}
	if contentHeight > 0 {
		if len(contentLines) > contentHeight {
			contentLines = contentLines[:contentHeight]
		}
		for len(contentLines) < contentHeight {
			contentLines = append(contentLines, formatInfoBoxLine("", contentWidth))
		}
	}
	box = append(box, contentLines...)
	if contentHeight == 0 {
		box = append(box, contentLines...)
	}
	box = append(box, infoBoxBottomLeft+strings.Repeat(infoBoxHorizontal, contentWidth+2)+infoBoxBottomRight)
	return box
}

func formatInfoFieldLines(label, value, labelStyle string, width int) []string {
	if value == "" {
		value = "unknown"
	}
	prefix := renderStyled(labelStyle, label) + ": "
	contentWidth := max(1, width-visibleWidth(prefix))
	chunks := wrapText(value, contentWidth)
	lines := make([]string, 0, len(chunks))
	indent := strings.Repeat(" ", len(label)+2)
	for index, chunk := range chunks {
		styledChunk := chunk
		if label == "status" {
			styledChunk = renderStyled(statusStyle(value), chunk)
		}
		if index == 0 {
			lines = append(lines, formatInfoBoxLine(prefix+styledChunk, width))
			continue
		}
		lines = append(lines, formatInfoBoxLine(indent+styledChunk, width))
	}
	return lines
}

func formatInfoBoxLine(content string, width int) string {
	content = fitInfoBoxContent(content, width)
	padding := width - visibleWidth(content)
	if padding < 0 {
		padding = 0
	}
	return infoBoxVertical + " " + content + strings.Repeat(" ", padding) + " " + infoBoxVertical
}

func fitInfoBoxContent(content string, width int) string {
	if visibleWidth(content) <= width {
		return content
	}
	plain := ansiSequence.ReplaceAllString(content, "")
	runes := []rune(plain)
	if len(runes) <= width {
		return plain
	}
	if width == 1 {
		return string(runes[:1])
	}
	return string(runes[:width-1]) + "~"
}

func wrapText(value string, width int) []string {
	if width < 1 {
		return []string{value}
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return []string{value}
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if visibleWidth(current)+1+visibleWidth(word) <= width {
			current += " " + word
			continue
		}
		if visibleWidth(word) > width {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			runes := []rune(word)
			for len(runes) > width {
				lines = append(lines, string(runes[:width]))
				runes = runes[width:]
			}
			current = string(runes)
			continue
		}
		lines = append(lines, current)
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func combineColumns(left, right []string, leftWidth, gap int) []string {
	combined := make([]string, 0, max(len(left), len(right)))
	padding := strings.Repeat(" ", gap)
	lineCount := max(len(left), len(right))
	for index := 0; index < lineCount; index++ {
		leftLine := ""
		if index < len(left) {
			leftLine = left[index]
		}
		rightLine := ""
		if index < len(right) {
			rightLine = right[index]
		}
		combined = append(combined, padRight(leftLine, leftWidth)+padding+rightLine)
	}
	return combined
}

func padRight(value string, width int) string {
	if width <= 0 {
		return value
	}
	padding := width - visibleWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func visibleWidth(value string) int {
	return len(ansiSequence.ReplaceAllString(value, ""))
}

func statusStyle(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return statusActiveStyle
	case "offline":
		return statusProblemStyle
	default:
		return statusPendingStyle
	}
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func joinOrUnknown[T ~string | ~int](values []T) string {
	if len(values) == 0 {
		return "unknown"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprint(value))
	}
	return strings.Join(parts, ", ")
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
