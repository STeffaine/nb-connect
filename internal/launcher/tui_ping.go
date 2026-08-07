package launcher

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type pingMessage struct {
	line string
	err  error
	done bool
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
