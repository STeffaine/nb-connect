package connector

import (
	"strings"
	"testing"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestSSHValidation(t *testing.T) {
	service := netbox.Service{Name: "ssh", Protocol: "tcp", IPs: []string{"192.0.2.10"}, Ports: []int{22}}
	tests := []struct {
		name     string
		service  netbox.Service
		endpoint string
		user     string
		want     string
	}{
		{name: "non TCP", service: netbox.Service{Name: "ssh", Protocol: "udp"}, endpoint: "192.0.2.10:22", user: "ops", want: "must use TCP"},
		{name: "missing user", service: service, endpoint: "192.0.2.10:22", want: "no SSH user"},
		{name: "invalid endpoint", service: service, endpoint: "router-01", user: "ops", want: "invalid endpoint"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := SSH(test.service, test.endpoint, test.user, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("SSH() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestTelnetValidation(t *testing.T) {
	service := netbox.Service{Name: "telnet", Protocol: "tcp", IPs: []string{"192.0.2.10"}, Ports: []int{23}}
	tests := []struct {
		name     string
		service  netbox.Service
		endpoint string
		want     string
	}{
		{name: "non TCP", service: netbox.Service{Name: "telnet", Protocol: "udp"}, endpoint: "192.0.2.10:23", want: "must use TCP"},
		{name: "invalid endpoint", service: service, endpoint: "router-01", want: "invalid endpoint"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Telnet(test.service, test.endpoint)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Telnet() error = %v, want %q", err, test.want)
			}
		})
	}
}
