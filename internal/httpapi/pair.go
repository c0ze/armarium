package httpapi

import (
	"net/http"
	"net/url"
	"slices"
)

// pairReaders are the readers Admin can pair. The hosted ones open
// <url>/#opds=<catalogue> on a phone; the desktop apps open their own
// skrivist-<app>://pair#opds=<catalogue> link on this computer. The Books
// phone app scans skrivist-books://pair#opds=<catalogue> from Add library →
// Scan QR code; it reaches plain HTTP on the home network and needs no CORS.
var pairReaders = []struct {
	ID, Name, URL   string
	Desktop, Native bool
}{
	{"books-app", "Skrivist Books app (iPhone, iPad, Android)", "skrivist-books://pair", false, true},
	{"comics", "Skrivist Comics", "https://comics.skriv.ist", false, false},
	{"books", "Skrivist Books", "https://books.skriv.ist", false, false},
	{"comics-desktop", "Skrivist Comics for Mac and Windows", "skrivist-comics://pair", true, false},
	{"books-desktop", "Skrivist Books for Mac and Windows", "skrivist-books://pair", true, false},
}

type pairReader struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Origin      string `json:"origin"`
	CORSAllowed bool   `json:"corsAllowed"`
	Desktop     bool   `json:"desktop"`
	Native      bool   `json:"native"`
	// Missing lists the exact cors_origins entries still to add.
	Missing []string `json:"missing"`
}

type pairInfo struct {
	PublicURL string       `json:"publicUrl"`
	Readers   []pairReader `json:"readers"`
}

func (s *Server) getPair(w http.ResponseWriter, r *http.Request) {
	allowed := func(o string) bool { return slices.Contains(s.Cfg.CORSOrigins, o) }
	info := pairInfo{PublicURL: s.Cfg.PublicURL, Readers: []pairReader{}}
	for _, p := range pairReaders {
		reader := pairReader{ID: p.ID, Name: p.Name, URL: p.URL, Desktop: p.Desktop, Native: p.Native, Missing: []string{}}
		// A native app's requests are not cross-origin browser requests, so it
		// needs no cors_origins entry.
		switch {
		case p.Native:
		case p.Desktop:
			// macOS sends tauri://localhost; Windows http://tauri.localhost, or
			// https://tauri.localhost in builds that use useHttpsScheme.
			if !allowed("tauri://localhost") {
				reader.Missing = append(reader.Missing, "tauri://localhost")
			}
			if !allowed("http://tauri.localhost") && !allowed("https://tauri.localhost") {
				reader.Missing = append(reader.Missing, "http://tauri.localhost")
			}
		default:
			u, _ := url.Parse(p.URL)
			reader.Origin = u.Scheme + "://" + u.Host
			if !allowed(reader.Origin) {
				reader.Missing = append(reader.Missing, reader.Origin)
			}
		}
		reader.CORSAllowed = len(reader.Missing) == 0
		info.Readers = append(info.Readers, reader)
	}
	writeJSON(w, http.StatusOK, info)
}
