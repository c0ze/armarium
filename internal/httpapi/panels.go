package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/c0ze/armarium/internal/store"
)

func (s *Server) panelItem(w http.ResponseWriter, r *http.Request) (store.Item, int, bool) {
	it, ok := s.item(w, r)
	if !ok {
		return it, 0, false
	}
	page, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || page < 1 || page > it.Pages || (it.Format != "cbz" && it.Format != "cbr") {
		s.fail(w, r, errNotFound)
		return it, 0, false
	}
	return it, page, true
}
func (s *Server) getPanels(w http.ResponseWriter, r *http.Request) {
	it, page, ok := s.panelItem(w, r)
	if !ok {
		return
	}
	correction, err := s.Store.PanelCorrection(r.Context(), it.ID, page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"correction": correction})
}
func (s *Server) putPanels(w http.ResponseWriter, r *http.Request) {
	it, page, ok := s.panelItem(w, r)
	if !ok {
		return
	}
	var correction store.PanelCorrection
	if decodeJSON(r, &correction) != nil || !correction.Valid() || correction.UpdatedAt != 0 {
		writeError(w, http.StatusBadRequest, "invalid panel correction")
		return
	}
	correction.ImageHash = strings.ToLower(correction.ImageHash)
	release, err := s.acquire(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer release()
	comic, done, err := s.openComic(r.Context(), it)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer done()
	image, _, err := comic.Page(page - 1)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer image.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, image); err != nil {
		s.fail(w, r, err)
		return
	}
	if hex.EncodeToString(hash.Sum(nil)) != correction.ImageHash {
		writeError(w, http.StatusConflict, "page image changed; reopen the editor")
		return
	}
	saved, err := s.Store.SavePanelCorrection(r.Context(), it.ID, page, correction)
	if errors.Is(err, store.ErrPanelConflict) {
		writeError(w, http.StatusConflict, "panels changed on another client; reopen the editor")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"correction": saved})
}
