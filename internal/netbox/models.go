package netbox

import (
	"net"
	"strings"
)

type Service struct {
	Server   string   `json:"server,omitempty"`
	Device   string   `json:"device"`
	VM       string   `json:"virtual_machine,omitempty"`
	Name     string   `json:"service"`
	Protocol string   `json:"protocol"`
	Ports    []int    `json:"ports"`
	IPs      []string `json:"ips"`
	Site     string   `json:"site,omitempty"`
	Role     string   `json:"role,omitempty"`
	Tenant   string   `json:"tenant,omitempty"`
	Platform string   `json:"platform,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Status   string   `json:"status,omitempty"`
}

func (service Service) TargetName() string {
	if service.Device != "" {
		return service.Device
	}
	return service.VM
}

func (service Service) Endpoints() []string {
	endpoints := make([]string, 0, len(service.IPs)*len(service.Ports))
	for _, address := range service.IPs {
		address = strings.TrimSpace(address)
		if host, _, err := net.ParseCIDR(address); err == nil {
			address = host.String()
		}
		for _, port := range service.Ports {
			endpoints = append(endpoints, net.JoinHostPort(address, stringPort(port)))
		}
	}
	return endpoints
}

func stringPort(port int) string {
	if port == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for port > 0 {
		index--
		digits[index] = byte('0' + port%10)
		port /= 10
	}
	return string(digits[index:])
}
