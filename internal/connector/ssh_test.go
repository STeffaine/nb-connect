package connector

import (
	"reflect"
	"testing"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestSSHBuildsCommand(t *testing.T) {
	service := netbox.Service{Name: "sshd", Protocol: "tcp", IPs: []string{"2001:db8::10/128"}, Ports: []int{22}}
	command, err := SSH(service, "", "ops", "/home/ops/.ssh/id_ops")
	if err != nil {
		t.Fatal(err)
	}
	if command.Name != "ssh" || !reflect.DeepEqual(command.Args, []string{"-i", "/home/ops/.ssh/id_ops", "ops@2001:db8::10", "-p", "22"}) {
		t.Fatalf("command = %#v", command)
	}
}

func TestSSHRejectsUnsupportedOrAmbiguousServices(t *testing.T) {
	if _, err := SSH(netbox.Service{Name: "https", Protocol: "tcp"}, "192.0.2.10:443", "ops", ""); err == nil {
		t.Fatal("SSH() error = nil for unsupported service")
	}
	service := netbox.Service{Name: "ssh", Protocol: "tcp", IPs: []string{"192.0.2.10"}, Ports: []int{22, 2222}}
	if _, err := SSH(service, "", "ops", ""); err == nil {
		t.Fatal("SSH() error = nil for ambiguous endpoint")
	}
}
