package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/steffaine/nb-connect/internal/connector"
	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestSelectServiceMatchesCaseInsensitively(t *testing.T) {
	services := []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}}
	selection, err := selectService(context.Background(), services, "ROUTER-01", "SSHD", "192.0.2.10:22", 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Endpoint != "192.0.2.10:22" || selection.Service.TargetName() != "router-01" {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestSelectServiceRejectsMissingAndAmbiguousMatches(t *testing.T) {
	services := []netbox.Service{
		{Device: "router-01", Name: "sshd"},
		{Device: "router-01", Name: "sshd"},
	}
	tests := []struct {
		name        string
		serviceName string
		want        string
	}{
		{name: "missing", serviceName: "https", want: "no cached"},
		{name: "ambiguous", serviceName: "sshd", want: "multiple cached"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := selectService(context.Background(), services, "router-01", test.serviceName, "", 4, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("selectService() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCommandFormattingAndServiceOutput(t *testing.T) {
	formatted := formatCommand(connector.Command{Name: "ssh", Args: []string{"ops@router-01", "-p", "22"}})
	if got, want := formatted, `ssh "ops@router-01" "-p" "22"`; got != want {
		t.Fatalf("formatCommand() = %q, want %q", got, want)
	}

	var output bytes.Buffer
	writeServices(&output, []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}}})
	for _, want := range []string{"TARGET", "router-01", "192.0.2.10:22"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("service output %q does not contain %q", output.String(), want)
		}
	}
}
