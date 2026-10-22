package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/c0ze/armarium/internal/kosync"
	"github.com/c0ze/armarium/internal/store"
)

// KOReader sync (KOSync) for CrossPoint Reader: the server URL is <base>/kosync,
// any user name, an API token as the password (CrossPoint sends it as Basic).
// See docs/superpowers/specs/2026-10-22-kosync-design.md.
func (s *Server) kosyncRoutes(route func(string, Access, http.HandlerFunc)) {
	route("GET /kosync/healthcheck", Public, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"state": "OK"})
	})
	route("POST /kosync/users/create", Public, func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusForbidden, "no sign-up: log in with an Armarium API token as the password")
	})
	route("GET /kosync/users/auth", Reader, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"authorized": "OK"})
	})
	route("GET /kosync/syncs/progress/{document}", Reader, s.kosyncGet)
	route("PUT /kosync/syncs/progress", Reader, s.kosyncPut)
}

type koProgress struct {
	Document   string  `json:"document"`
	Progress   string  `json:"progress"`
	Percentage float64 `json:"percentage"`
	Device     string  `json:"device"`
	DeviceID   string  `json:"device_id"`
	Timestamp  int64   `json:"timestamp"`
}

func (s *Server) kosyncGet(w http.ResponseWriter, r *http.Request) {
	doc := r.PathValue("document")
	if !kosync.ValidDocument(doc) {
		writeError(w, http.StatusBadRequest, "bad document id")
		return
	}
	rec, err := s.Store.KOSyncDocument(r.Context(), doc)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if rec.ItemID != 0 {
		it, err := s.Store.Item(r.Context(), rec.ItemID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.fail(w, r, err)
			return
		}
		// Read on elsewhere in Armarium since the last upload: hand that out.
		if err == nil && it.Progress.Page > 0 && it.Progress.UpdatedAt > rec.UpdatedAt {
			chapter, scroll := it.Progress.Page-1, 0.0
			var loc struct {
				Chapter *int    `json:"chapter"`
				Scroll  float64 `json:"scroll"`
			}
			if json.Unmarshal([]byte(it.Progress.Locator), &loc) == nil && loc.Chapter != nil && *loc.Chapter >= 0 {
				chapter, scroll = *loc.Chapter, min(max(loc.Scroll, 0), 1)
			}
			pct := 0.0
			if it.Pages > 0 {
				pct = min((float64(chapter)+scroll)/float64(it.Pages), 1)
			}
			writeJSON(w, http.StatusOK, koProgress{Document: doc, Progress: kosync.XPointer(chapter), Percentage: pct,
				Device: "Armarium", DeviceID: "armarium", Timestamp: it.Progress.UpdatedAt})
			return
		}
	}
	if rec.UpdatedAt == 0 {
		writeError(w, http.StatusNotFound, "no progress for this document")
		return
	}
	writeJSON(w, http.StatusOK, koProgress{Document: doc, Progress: rec.Progress, Percentage: rec.Percentage,
		Device: rec.Device, DeviceID: rec.DeviceID, Timestamp: rec.UpdatedAt})
}

func (s *Server) kosyncPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		koProgress
		Metadata *struct {
			Filename string `json:"filename"`
			Title    string `json:"title"`
			Authors  string `json:"authors"`
		} `json:"metadata"`
	}
	// Not decodeJSON: KOSync clients add fields of their own.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !kosync.ValidDocument(body.Document) ||
		body.Percentage < 0 || body.Percentage > 1 || len(body.Progress) > 1024 ||
		len(body.Device) > 100 || len(body.DeviceID) > 100 {
		writeError(w, http.StatusBadRequest, "body must be {document, progress, percentage, device, device_id}")
		return
	}
	ctx := r.Context()
	rec, err := s.Store.KOSyncDocument(ctx, body.Document)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if rec.ItemID == 0 && body.Metadata != nil {
		rec.ItemID, err = s.matchDocument(ctx, body.Document, body.Metadata.Title, body.Metadata.Authors)
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	rec = store.KOSyncRecord{Document: body.Document, ItemID: rec.ItemID, Progress: body.Progress,
		Percentage: body.Percentage, Device: body.Device, DeviceID: body.DeviceID, UpdatedAt: s.Store.Now().Unix()}
	if rec.ItemID != 0 {
		it, err := s.Store.Item(ctx, rec.ItemID)
		switch {
		case errors.Is(err, store.ErrNotFound): // gone missing since: keep the record only
		case err != nil:
			s.fail(w, r, err)
			return
		default:
			chapter, scroll := kosync.ChapterOf(body.Progress, body.Percentage, it.Pages)
			loc, _ := json.Marshal(map[string]any{"chapter": chapter, "scroll": scroll})
			p, _, err := s.Store.SaveProgress(ctx, it.ID, it.Pages, store.ProgressUpdate{Page: chapter + 1, Locator: string(loc)})
			if err != nil {
				s.fail(w, r, err)
				return
			}
			// Not newer than the progress this upload caused, so it is not handed back as Armarium's own.
			rec.UpdatedAt = max(rec.UpdatedAt, p.UpdatedAt)
		}
	}
	if err := s.Store.SaveKOSync(ctx, rec); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": rec.Document, "timestamp": rec.UpdatedAt})
}

// matchDocument finds the item an unknown document names from the metadata a
// device sent: a same-title EPUB whose file has that KOReader id, else the
// only same-title (and, among several, same-author) EPUB. 0 when none.
func (s *Server) matchDocument(ctx context.Context, doc, title, authors string) (int64, error) {
	cands, err := s.Store.EPUBCandidates(ctx, strings.TrimSpace(title), 10)
	if err != nil {
		return 0, err
	}
	for _, c := range cands {
		if d, err := s.epubDigest(ctx, c); err == nil && d == doc {
			return c.ItemID, nil
		}
	}
	if len(cands) > 1 && authors != "" {
		var same []store.EPUBFile
		for _, c := range cands {
			if strings.EqualFold(strings.TrimSpace(c.Author), strings.TrimSpace(authors)) {
				same = append(same, c)
			}
		}
		cands = same
	}
	if len(cands) == 1 {
		return cands[0].ItemID, nil
	}
	return 0, nil
}

func (s *Server) epubDigest(ctx context.Context, c store.EPUBFile) (string, error) {
	lib, err := s.Store.Library(ctx, c.LibraryID)
	if err != nil {
		return "", err
	}
	p, err := s.inLibrary(ctx, c.LibraryID, filepath.Join(lib.Root, filepath.FromSlash(c.Path)))
	if err != nil {
		return "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return fileDigest(f)
}

func fileDigest(f *os.File) (string, error) {
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	return kosync.Digest(f, info.Size())
}

// linkDownload records the KOReader id of an EPUB being served, so a device
// that syncs it later (CrossPoint "Binary" matching) is matched to the item.
func (s *Server) linkDownload(ctx context.Context, f *os.File, itemID int64) {
	d, err := fileDigest(f)
	if err == nil {
		err = s.Store.LinkKOSyncDocument(ctx, d, itemID)
	}
	if err != nil {
		s.Log.Warn("kosync: linking a download failed", "item", itemID, "err", err)
	}
}
