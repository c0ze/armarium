package tailscale

import "testing"

func TestParseName(t *testing.T) {
	cases := map[string]string{
		`{"BackendState":"Running","Self":{"DNSName":"Studio.tail1234.ts.net."}}`: "studio.tail1234.ts.net",
		`{"BackendState":"Stopped","Self":{"DNSName":"studio.tail1234.ts.net."}}`: "",
		`{"BackendState":"NeedsLogin","Self":{"DNSName":""}}`:                     "",
		`not json`: "",
	}
	for in, want := range cases {
		if got := parseName([]byte(in)); got != want {
			t.Errorf("parseName(%s) = %q, want %q", in, got, want)
		}
	}
}
