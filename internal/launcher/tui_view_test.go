package launcher

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSearchInputEditsUnicodeRunes(t *testing.T) {
	input := searchInput{value: "rout\u00e9"}
	updated, _ := input.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got, want := updated.Value(), "rout"; got != want {
		t.Fatalf("backspace value = %q, want %q", got, want)
	}
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\u00e9")})
	if got, want := updated.Value(), "rout\u00e9"; got != want {
		t.Fatalf("typed value = %q, want %q", got, want)
	}
}

func TestPingPopupRespectsTerminalWidth(t *testing.T) {
	model := model{width: 16, pinging: true, pingNote: "a very long ping result"}
	view := model.View()
	for _, line := range strings.Split(view, "\n") {
		if (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "|")) && len(line) > 16 {
			t.Fatalf("line exceeds terminal width: %q", line)
		}
	}
}
