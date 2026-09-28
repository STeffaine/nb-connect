package app

import (
	"context"
	"strings"
	"testing"

	"github.com/steffaine/nb-connect/internal/config"
	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestSelectServiceWithPresetRejectsUnknown(t *testing.T) {
	services := []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}
	_, err := selectServiceWithPreset(context.Background(), services, "", "missing", "router-01", "sshd", "", 4, true, []config.FilterPreset{{Name: "vpn"}}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), `no filter preset named "missing"`) {
		t.Fatalf("selectServiceWithPreset() error = %v", err)
	}
}

func TestSelectServiceWithPresetUsesPresetServerInExplicitMode(t *testing.T) {
	services := []netbox.Service{
		{Server: "production", Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}},
		{Server: "lab", Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.11"}, Ports: []int{22}},
	}
	presets := []config.FilterPreset{{Name: "lab-only", Server: "lab"}}
	selection, err := selectServiceWithPreset(context.Background(), services, "", "lab-only", "router-01", "sshd", "192.0.2.11:22", 4, true, presets, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Service.Server != "lab" {
		t.Fatalf("selection server = %q, want lab", selection.Service.Server)
	}
}

func TestSelectServiceWithPresetServerFlagWins(t *testing.T) {
	services := []netbox.Service{
		{Server: "production", Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}},
		{Server: "lab", Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.11"}, Ports: []int{22}},
	}
	presets := []config.FilterPreset{{Name: "lab-only", Server: "lab"}}
	selection, err := selectServiceWithPreset(context.Background(), services, "production", "lab-only", "router-01", "sshd", "192.0.2.10:22", 4, true, presets, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Service.Server != "production" {
		t.Fatalf("selection server = %q, want production", selection.Service.Server)
	}
}
