package connector

import (
	"fmt"
	"net"
	"strings"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func Telnet(service netbox.Service, endpoint string) (Command, error) {
	if !strings.EqualFold(strings.TrimSpace(service.Name), "telnet") {
		return Command{}, fmt.Errorf("service %q is not supported by the Telnet connector", service.Name)
	}
	if !strings.EqualFold(service.Protocol, "tcp") {
		return Command{}, fmt.Errorf("Telnet service %q must use TCP, got %q", service.Name, service.Protocol)
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
	return Command{Name: "telnet", Args: []string{host, port}}, nil
}
