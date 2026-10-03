package httpapi

import (
	"net/http"
	"net/url"
	"slices"
)

// pairReaders are the hosted readers Admin can pair a phone with. The QR opens
// <url>/#opds=<catalogue>, which both read on startup.
var pairReaders = []struct{ ID, Name, URL string }{
	{"comics", "Skrivist Comics", "https://comics.skriv.ist"},
	{"books", "Skrivist Books", "https://books.skriv.ist"},
}

type pairReader struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Origin      string `json:"origin"`
	CORSAllowed bool   `json:"corsAllowed"`
}

type pairInfo struct {
	PublicURL string       `json:"publicUrl"`
	Readers   []pairReader `json:"readers"`
}

func (s *Server) getPair(w http.ResponseWriter, r *http.Request) {
	info := pairInfo{PublicURL: s.Cfg.PublicURL, Readers: []pairReader{}}
	for _, p := range pairReaders {
		u, _ := url.Parse(p.URL)
		origin := u.Scheme + "://" + u.Host
		info.Readers = append(info.Readers, pairReader{
			ID: p.ID, Name: p.Name, URL: p.URL, Origin: origin,
			CORSAllowed: slices.Contains(s.Cfg.CORSOrigins, origin),
		})
	}
	writeJSON(w, http.StatusOK, info)
}
