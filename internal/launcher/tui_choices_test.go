package launcher

import (
	"context"
	"strings"
	"testing"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestChoicesForServicesRejectsMissingEndpoints(t *testing.T) {
	_, err := choicesForServices([]netbox.Service{{Device: "router-01", Name: "sshd"}})
	if err == nil || !strings.Contains(err.Error(), "no cached services") {
		t.Fatalf("choicesForServices error = %v", err)
	}
}

func TestVisibleChoicesRebuildsStaleSearchIndex(t *testing.T) {
	selector, err := newModel(context.Background(), []netbox.Service{{Device: "router-01", Name: "sshd", IPs: []string{"192.0.2.10"}, Ports: []int{22}, Role: "Edge"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selector.choiceSearch = nil
	selector.filter.SetValue("edge")
	if got := selector.visibleChoices(); len(got) != 1 || got[0].Service.TargetName() != "router-01" {
		t.Fatalf("visible choices with stale index = %#v", got)
	}
}
