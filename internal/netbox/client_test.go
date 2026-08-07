package netbox

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServicesFollowsPaginationAndNormalizesAddresses(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Token example" {
			t.Fatalf("unexpected authorization header")
		}
		requests++
		response.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/virtualization/virtual-machines/7/" {
			_, _ = response.Write([]byte(`{"site":{"name":"HQ"},"role":{"name":"Management"},"tenant":{"name":"Operations"},"platform":{"name":"Linux"},"status":{"value":"active","label":"Active"}}`))
			return
		}
		if request.URL.Query().Get("page") == "2" {
			response.Write([]byte(`{"next":null,"results":[{"name":"https","protocol":{"value":{"id":1},"display":"tcp"},"ports":[443],"ipaddresses":[{"address":"2001:db8::1/128"}],"parent":{"name":"netbox","object_type":"virtualization.virtualmachine","url":"/api/virtualization/virtual-machines/7/"}}]}`))
			return
		}
		response.Write([]byte(`{"next":"/api/ipam/services/?limit=100&page=2","results":[{"name":"sshd","protocol":{"value":"tcp","label":"TCP"},"ports":[22],"ipaddresses":[{"address":"172.30.71.101/32"}],"device":{"name":"xcp-ng-01"},"tags":[{"name":"production"}]}]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "example", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	services, err := client.Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || len(services) != 2 {
		t.Fatalf("got %d requests and %d services", requests, len(services))
	}
	if got := services[0].Protocol; got != "tcp" {
		t.Fatalf("protocol = %q", got)
	}
	if got := services[1].Protocol; got != "tcp" {
		t.Fatalf("protocol = %q", got)
	}
	if got := services[1].VM; got != "netbox" {
		t.Fatalf("virtual machine = %q", got)
	}
	if got := services[1].Role; got != "Management" {
		t.Fatalf("role = %q", got)
	}
	if got := services[1].Tenant; got != "Operations" || services[1].Status != "Active" {
		t.Fatalf("metadata = %#v", services[1])
	}
	if got := services[0].Endpoints()[0]; got != "172.30.71.101:22" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := services[1].Endpoints()[0]; got != "[2001:db8::1]:443" {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestValidateRejectsUnauthorizedToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "example", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Validate(context.Background()); err == nil {
		t.Fatal("Validate() error = nil")
	}
}

func TestValidateWritesAPIDebugTraceWithoutAuthorizationHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer nbt_example" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		_, _ = response.Write([]byte(`{"netbox-version":"4.6"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "nbt_example", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var trace bytes.Buffer
	client.SetDebugOutput(&trace)
	if err := client.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := trace.String(); !strings.Contains(got, "GET "+server.URL+"/api/status/") || !strings.Contains(got, `{"netbox-version":"4.6"}`) || strings.Contains(got, "nbt_example") {
		t.Fatalf("unexpected trace: %q", got)
	}
}

func TestNewClientRejectsRelativeURL(t *testing.T) {
	if _, err := NewClient("/api", "example", nil); err == nil {
		t.Fatal("NewClient() error = nil")
	}
}

func TestServicesReportsHTTPAndDecodeFailures(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
		want string
	}{
		{name: "HTTP status", code: http.StatusInternalServerError, body: "unavailable", want: "unexpected status"},
		{name: "invalid JSON", code: http.StatusOK, body: "{", want: "decode NetBox services"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				response.WriteHeader(test.code)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "example", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Services(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Services() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestServicesCachesParentMetadata(t *testing.T) {
	parentRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/dcim/devices/1/" {
			parentRequests++
			_, _ = response.Write([]byte(`{"role":{"name":"Router"}}`))
			return
		}
		_, _ = response.Write([]byte(`{"next":null,"results":[{"name":"sshd","protocol":"tcp","ports":[22],"ipaddresses":[{"address":"192.0.2.10/32"}],"device":{"name":"router-01","url":"/api/dcim/devices/1/"}},{"name":"https","protocol":"tcp","ports":[443],"ipaddresses":[{"address":"192.0.2.10/32"}],"device":{"name":"router-01","url":"/api/dcim/devices/1/"}}]}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "example", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	services, err := client.Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if parentRequests != 1 || len(services) != 2 || services[0].Role != "Router" || services[1].Role != "Router" {
		t.Fatalf("parent requests = %d, services = %#v", parentRequests, services)
	}
}

func TestValidateWrapsTransportError(t *testing.T) {
	client, err := NewClient("https://netbox.example.test", "example", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network unavailable")
	})})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "contact NetBox") {
		t.Fatalf("Validate() error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
