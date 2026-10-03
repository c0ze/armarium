package config

import "testing"

func TestHostAllowed(t *testing.T) {
	cases := []struct {
		allowed []string
		host    string
		want    bool
	}{
		{[]string{"nas.local"}, "nas.local:8580", true},
		{[]string{"nas.local"}, "NAS.local", true},
		{[]string{"nas.local:8580"}, "nas.local:8580", true},
		{[]string{"nas.local:8580"}, "nas.local:9000", false},
		{[]string{"nas.local:8580"}, "nas.local", false},
		{[]string{"[::1]"}, "[::1]:8580", true},
		{[]string{"::1"}, "[::1]:8580", true},
		{[]string{"[::1]:8580"}, "[::1]:8580", true},
		{[]string{"[::1]:8580"}, "[::1]:9000", false},
		{[]string{"localhost"}, "evil.example", false},
	}
	for _, c := range cases {
		if got := HostAllowed(c.allowed, c.host); got != c.want {
			t.Errorf("HostAllowed(%q, %q) = %v, want %v", c.allowed, c.host, got, c.want)
		}
	}
}
