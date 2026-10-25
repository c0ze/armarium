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

func TestServeState(t *testing.T) {
	ours := "http://127.0.0.1:8580"
	cases := []struct {
		name, json    string
		owned, others bool
		conflict      bool
	}{
		{"empty", `{}`, false, false, false},
		{"ours", `{"TCP":{"443":{"HTTPS":true}},"Web":{"studio.ts.net:443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:8580"}}}}}`, true, false, false},
		{"ours with tincan", `{"TCP":{"443":{"HTTPS":true}},"Web":{"studio.ts.net:443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:8580"},"/tincan":{"Proxy":"http://127.0.0.1:7788"}}}}}`, true, true, false},
		{"only tincan", `{"TCP":{"443":{"HTTPS":true}},"Web":{"macmini.ts.net:443":{"Handlers":{"/tincan":{"Proxy":"http://127.0.0.1:7788"}}}}}`, false, true, false},
		{"someone else on /", `{"TCP":{"443":{"HTTPS":true}},"Web":{"studio.ts.net:443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:3000"}}}}}`, false, true, true},
		{"static files on /", `{"TCP":{"443":{"HTTPS":true}},"Web":{"studio.ts.net:443":{"Handlers":{"/":{"Path":"/srv/www"}}}}}`, false, true, true},
		{"old port", `{"TCP":{"443":{"HTTPS":true}},"Web":{"studio.ts.net:443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:8581"}}}}}`, false, true, true},
		{"tcp forward on 443", `{"TCP":{"443":{"TCPForward":"127.0.0.1:22"}}}`, false, true, true},
	}
	for _, c := range cases {
		st, err := parseServe([]byte(c.json), ours)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if st.Owned != c.owned || (st.Conflict != "") != c.conflict {
			t.Errorf("%s: %+v", c.name, st)
		}
	}
}
