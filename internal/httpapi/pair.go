package httpapi

import (
	"net/http"
	"slices"
)

// pairReaders are the Skrivist apps Admin can pair. All of them open
// skrivist-<app>://pair#opds=<catalogue>: the iPhone, iPad and Android apps
// scan it as a QR code (Add library → Scan QR code), and the Mac and Windows
// apps open it as a link on this computer.
var pairReaders = []struct {
	ID, Name, URL string
	Desktop       bool
}{
	{"comics", "Skrivist Comics", "skrivist-comics://pair", false},
	{"books", "Skrivist Books", "skrivist-books://pair", false},
	{"comics-desktop", "Skrivist Comics for Mac and Windows", "skrivist-comics://pair", true},
	{"books-desktop", "Skrivist Books for Mac and Windows", "skrivist-books://pair", true},
}

type pairReader struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Origin      string `json:"origin"`
	CORSAllowed bool   `json:"corsAllowed"`
	Desktop     bool   `json:"desktop"`
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
		reader := pairReader{ID: p.ID, Name: p.Name, URL: p.URL, Desktop: p.Desktop, Missing: []string{}}
		// The phone apps are native HTTP clients: they send no Origin, so CORS
		// doesn't apply. The desktop apps load Armarium from a webview.
		if p.Desktop {
			// macOS sends tauri://localhost; Windows http://tauri.localhost, or
			// https://tauri.localhost in builds that use useHttpsScheme.
			if !allowed("tauri://localhost") {
				reader.Missing = append(reader.Missing, "tauri://localhost")
			}
			if !allowed("http://tauri.localhost") && !allowed("https://tauri.localhost") {
				reader.Missing = append(reader.Missing, "http://tauri.localhost")
			}
		}
		reader.CORSAllowed = len(reader.Missing) == 0
		info.Readers = append(info.Readers, reader)
	}
	writeJSON(w, http.StatusOK, info)
}
