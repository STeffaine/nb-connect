package connector

import (
	"reflect"
	"testing"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestTelnetBuildsCommand(t *testing.T) {
	service := netbox.Service{Name: "telnet", Protocol: "tcp", IPs: []string{"2001:db8::10/128"}, Ports: []int{23}}
	command, err := Telnet(service, "")
	if err != nil {
		t.Fatal(err)
	}
	if command.Name != "telnet" || !reflect.DeepEqual(command.Args, []string{"2001:db8::10", "23"}) {
		t.Fatalf("command = %#v", command)
	}
}

func TestTelnetRejectsUnsupportedOrAmbiguousServices(t *testing.T) {
	if _, err := Telnet(netbox.Service{Name: "sshd", Protocol: "tcp"}, "192.0.2.10:22"); err == nil {
		t.Fatal("Telnet() error = nil for unsupported service")
	}
	service := netbox.Service{Name: "telnet", Protocol: "tcp", IPs: []string{"192.0.2.10"}, Ports: []int{23, 2323}}
	if _, err := Telnet(service, ""); err == nil {
		t.Fatal("Telnet() error = nil for ambiguous endpoint")
	}
}
