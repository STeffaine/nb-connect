package connector

import (
	"fmt"
	"net"
	"strings"

	"github.com/steffaine/nb-connect/internal/netbox"
)

type Command struct {
	Name string
	Args []string
}

func SSH(service netbox.Service, endpoint, user, identityFile string) (Command, error) {
	if !isSSHService(service.Name) {
		return Command{}, fmt.Errorf("service %q is not supported by the SSH connector", service.Name)
	}
	if !strings.EqualFold(service.Protocol, "tcp") {
		return Command{}, fmt.Errorf("SSH service %q must use TCP, got %q", service.Name, service.Protocol)
	}
	if strings.TrimSpace(user) == "" {
		return Command{}, fmt.Errorf("no SSH user configured for %q", service.TargetName())
	}
	if endpoint == "" {
		endpoints := service.Endpoints()
		if len(endpoints) != 1 {
			return Command{}, fmt.Errorf("service %q has %d endpoints; choose one with --endpoint", service.TargetName(), len(endpoints))
		}
		endpoint = endpoints[0]
	}

	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || host == "" || port == "" {
		return Command{}, fmt.Errorf("invalid endpoint %q", endpoint)
	}
	args := make([]string, 0, 5)
	if identityFile = strings.TrimSpace(identityFile); identityFile != "" {
		args = append(args, "-i", identityFile)
	}
	args = append(args, user+"@"+host, "-p", port)
	return Command{Name: "ssh", Args: args}, nil
}

func isSSHService(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ssh", "sshd":
		return true
	default:
		return false
	}
}
