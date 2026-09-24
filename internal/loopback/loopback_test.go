package loopback

import "testing"

// TestClassify pins the shared accept/reject predicate both consumers build on:
// literal loopback IPs are OK, unspecified addresses are their own reason (so a
// caller can map them to a wildcard-specific message), non-IP strings and
// non-loopback IPs are rejected with distinct reasons.
func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		bind string
		want Reason
	}{
		{name: "ipv4 loopback", bind: "127.0.0.1", want: OK},
		{name: "ipv4 loopback /8 subnet", bind: "127.1.2.3", want: OK},
		{name: "ipv6 loopback", bind: "::1", want: OK},
		{name: "unspecified ipv4", bind: "0.0.0.0", want: Unspecified},
		{name: "unspecified ipv6", bind: "::", want: Unspecified},
		{name: "hostname", bind: "localhost", want: NotAnIP},
		{name: "empty", bind: "", want: NotAnIP},
		{name: "public ipv4", bind: "192.168.1.5", want: NotLoopback},
		{name: "ipv4-mapped public ip", bind: "::ffff:8.8.8.8", want: NotLoopback},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, reason := Classify(tt.bind)
			if reason != tt.want {
				t.Fatalf("Classify(%q) reason = %v, want %v", tt.bind, reason, tt.want)
			}
			if tt.want == NotAnIP && ip != nil {
				t.Errorf("Classify(%q) ip = %v, want nil", tt.bind, ip)
			}
			if tt.want != NotAnIP && ip == nil {
				t.Errorf("Classify(%q) ip = nil, want parsed address", tt.bind)
			}
		})
	}
}
