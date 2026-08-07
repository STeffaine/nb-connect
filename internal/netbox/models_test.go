package netbox

import (
	"reflect"
	"testing"
)

func TestServiceTargetName(t *testing.T) {
	if got := (Service{Device: "router", VM: "router-vm"}).TargetName(); got != "router" {
		t.Fatalf("TargetName() = %q", got)
	}
	if got := (Service{VM: "router-vm"}).TargetName(); got != "router-vm" {
		t.Fatalf("TargetName() = %q", got)
	}
}

func TestServiceEndpoints(t *testing.T) {
	service := Service{
		IPs:   []string{" 192.0.2.10/32 ", "2001:db8::10/128", "console.example.test"},
		Ports: []int{22, 2222},
	}
	want := []string{
		"192.0.2.10:22", "192.0.2.10:2222",
		"[2001:db8::10]:22", "[2001:db8::10]:2222",
		"console.example.test:22", "console.example.test:2222",
	}
	if got := service.Endpoints(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Endpoints() = %v, want %v", got, want)
	}
}

func TestServiceEndpointsWithoutAddressesOrPorts(t *testing.T) {
	if got := (Service{IPs: []string{"192.0.2.10"}}).Endpoints(); len(got) != 0 {
		t.Fatalf("Endpoints() = %v, want no endpoints", got)
	}
}
