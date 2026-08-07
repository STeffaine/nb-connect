package launcher

import "testing"

func TestPingHost(t *testing.T) {
	tests := []struct {
		endpoint string
		host     string
		valid    bool
	}{
		{endpoint: "192.0.2.10:22", host: "192.0.2.10", valid: true},
		{endpoint: "[2001:db8::1]:22", host: "2001:db8::1", valid: true},
		{endpoint: "router-01", valid: false},
		{endpoint: ":22", valid: false},
		{endpoint: "router-01:", valid: false},
	}
	for _, test := range tests {
		host, err := pingHost(test.endpoint)
		if test.valid && (err != nil || host != test.host) {
			t.Errorf("pingHost(%q) = %q, %v; want %q, nil", test.endpoint, host, err, test.host)
		}
		if !test.valid && err == nil {
			t.Errorf("pingHost(%q) succeeded with host %q", test.endpoint, host)
		}
	}
}
