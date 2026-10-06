package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/c0ze/armarium/internal/store"
	tu "github.com/c0ze/armarium/internal/testutil"
)

func panelInput() store.PanelCorrection {
	hash := sha256.Sum256(tu.PNG(8, 12, 20))
	return store.PanelCorrection{ImageHash: hex.EncodeToString(hash[:]), Enabled: true, PageSize: store.PanelSize{W: 8, H: 12}, Direction: "rtl", Panels: []store.PanelBox{{ID: 9, Rect: []float64{.5, 0, .5, 1}}, {ID: 2, Rect: []float64{0, 0, .5, 1}}}, AutomaticCandidates: [][]float64{{0, 0, 1, 1}}, Detector: store.PanelDetector{Model: "test", Revision: "v1"}}
}
func TestPanelsSharedAcrossTokensAndRevisionConflict(t *testing.T) {
	e := newEnv(t)
	path := fmt.Sprintf("/api/items/%d/pages/2/panels", e.comicID)
	input := panelInput()
	input.AutomaticPanels = []store.PanelBox{{ID: 0, Rect: []float64{0, 0, 1, 1}}}
	put := func(input store.PanelCorrection) *store.PanelCorrection {
		input.UpdatedAt = 0
		body, _ := json.Marshal(input)
		w := e.do(req{method: "PUT", path: path, body: string(body), token: e.token, origin: "https://comics.skriv.ist"})
		if w.Code != 200 {
			t.Fatalf("save: %d %s", w.Code, w.Body)
		}
		return decode[struct {
			Correction *store.PanelCorrection `json:"correction"`
		}](t, w).Correction
	}
	saved := put(input)
	if saved.Revision != 1 || saved.Panels[0].ID != 9 {
		t.Fatalf("order/revision lost: %+v", saved)
	}
	e.login() // A separate client credential reads exactly the same correction.
	other := decode[struct {
		Correction *store.PanelCorrection `json:"correction"`
	}](t, e.do(req{path: path, cookie: true})).Correction
	if other.Revision != 1 || other.Panels[0].ID != 9 || len(other.AutomaticPanels) != 1 {
		t.Fatal("correction must belong to the comic")
	}
	body, _ := json.Marshal(input)
	if w := e.do(req{method: "PUT", path: path, body: string(body), token: e.token}); w.Code != 409 {
		t.Fatalf("stale create: %d", w.Code)
	}
	input = *saved
	input.Panels = []store.PanelBox{}
	input.UpdatedAt = 0
	whole := put(input)
	if whole.Revision != 2 || len(whole.Panels) != 0 || !whole.Enabled {
		t.Fatal("whole-page override lost")
	}
	input = *whole
	input.Enabled = false
	input.UpdatedAt = 0
	reset := put(input)
	if reset.Revision != 3 || reset.Enabled {
		t.Fatal("reset tombstone lost")
	}
	old, _ := json.Marshal(*whole)
	// With a clean request timestamp, an old revision still cannot overwrite reset.
	input = *whole
	input.UpdatedAt = 0
	old, _ = json.Marshal(input)
	if w := e.do(req{method: "PUT", path: path, body: string(old), token: e.token}); w.Code != 409 {
		t.Fatalf("stale update: %d", w.Code)
	}
}
func TestPanelsValidateImageBoundsAndAuthentication(t *testing.T) {
	e := newEnv(t)
	path := fmt.Sprintf("/api/items/%d/pages/2/panels", e.comicID)
	input := panelInput()
	body, _ := json.Marshal(input)
	if w := e.do(req{method: "PUT", path: path, body: string(body)}); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w := e.do(req{method: "PUT", path: path, body: string(body), token: e.token, origin: "https://evil.test"}); w.Code != 403 {
		t.Fatalf("origin: %d", w.Code)
	}
	input.ImageHash = fmt.Sprintf("%064x", 1)
	body, _ = json.Marshal(input)
	if w := e.do(req{method: "PUT", path: path, body: string(body), token: e.token}); w.Code != 409 {
		t.Fatalf("wrong image: %d", w.Code)
	}
	for _, mutate := range []func(*store.PanelCorrection){
		func(p *store.PanelCorrection) { p.Panels[0].Rect = []float64{.9, 0, .5, 1} },
		func(p *store.PanelCorrection) { p.Panels[1].ID = p.Panels[0].ID },
		func(p *store.PanelCorrection) { p.Panels[0].Rect = []float64{0, 0, 0, 1} },
		func(p *store.PanelCorrection) { p.PageSize.W = 0 },
	} {
		bad := panelInput()
		mutate(&bad)
		body, _ = json.Marshal(bad)
		if w := e.do(req{method: "PUT", path: path, body: string(body), token: e.token}); w.Code != 400 {
			t.Fatalf("invalid input: %d", w.Code)
		}
	}
	for _, target := range []string{fmt.Sprintf("/api/items/%d/pages/4/panels", e.comicID), fmt.Sprintf("/api/items/%d/pages/1/panels", e.bookID)} {
		if w := e.do(req{path: target, token: e.token}); w.Code != 404 {
			t.Fatalf("invalid page: %d", w.Code)
		}
	}
}
