package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
)

var ErrPanelConflict = errors.New("panel correction changed")

type PanelBox struct {
	ID   int       `json:"id"`
	Rect []float64 `json:"rect"`
}
type PanelSize struct {
	W int `json:"w"`
	H int `json:"h"`
}
type PanelDetector struct {
	Model    string `json:"model"`
	Revision string `json:"revision"`
}
type PanelCorrection struct {
	ImageHash           string        `json:"imageHash"`
	Revision            int64         `json:"revision"`
	Enabled             bool          `json:"enabled"`
	PageSize            PanelSize     `json:"pageSize"`
	Direction           string        `json:"direction"`
	Panels              []PanelBox    `json:"panels"`
	AutomaticCandidates [][]float64   `json:"automaticCandidates"`
	AutomaticPanels     []PanelBox    `json:"automaticPanels"`
	Detector            PanelDetector `json:"detector"`
	UpdatedAt           int64         `json:"updatedAt"`
}

func validPanelRect(rect []float64, minimum float64) bool {
	if len(rect) != 4 {
		return false
	}
	for _, n := range rect {
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1 {
			return false
		}
	}
	return rect[2] >= minimum && rect[3] >= minimum && rect[0]+rect[2] <= 1.000000001 && rect[1]+rect[3] <= 1.000000001
}
func (p PanelCorrection) Valid() bool {
	hash, err := hex.DecodeString(p.ImageHash)
	if err != nil || len(hash) != 32 || p.Revision < 0 || p.PageSize.W < 1 || p.PageSize.H < 1 || p.PageSize.W > 100000 || p.PageSize.H > 100000 || (p.Direction != "ltr" && p.Direction != "rtl") || p.Panels == nil || len(p.Panels) > 100 || len(p.AutomaticCandidates) > 500 || len(p.Detector.Model) > 200 || len(p.Detector.Revision) > 200 {
		return false
	}
	ids := map[int]bool{}
	for _, box := range p.Panels {
		if box.ID < 0 || ids[box.ID] || !validPanelRect(box.Rect, .005) {
			return false
		}
		ids[box.ID] = true
	}
	for _, rect := range p.AutomaticCandidates {
		if !validPanelRect(rect, 0) {
			return false
		}
	}
	if len(p.AutomaticPanels) > 500 {
		return false
	}
	ids = map[int]bool{}
	for _, panel := range p.AutomaticPanels {
		if panel.ID < 0 || ids[panel.ID] || !validPanelRect(panel.Rect, 0) {
			return false
		}
		ids[panel.ID] = true
	}
	return true
}
func (s *Store) PanelCorrection(ctx context.Context, itemID int64, page int) (*PanelCorrection, error) {
	var document string
	err := s.DB.QueryRowContext(ctx, "SELECT document FROM panel_correction WHERE item_id = ? AND page = ?", itemID, page).Scan(&document)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var correction PanelCorrection
	if err := json.Unmarshal([]byte(document), &correction); err != nil {
		return nil, err
	}
	return &correction, nil
}

// SavePanelCorrection is an atomic compare-and-swap. Revision zero creates;
// stale drafts cannot overwrite a newer shared correction or its tombstone.
func (s *Store) SavePanelCorrection(ctx context.Context, itemID int64, page int, correction PanelCorrection) (*PanelCorrection, error) {
	if page < 1 || !correction.Valid() {
		return nil, errors.New("invalid panel correction")
	}
	expected := correction.Revision
	correction.Revision++
	correction.UpdatedAt = s.now()
	document, err := json.Marshal(correction)
	if err != nil {
		return nil, err
	}
	var result sql.Result
	if expected == 0 {
		result, err = s.DB.ExecContext(ctx, "INSERT INTO panel_correction (item_id,page,revision,document) VALUES (?,?,?,?) ON CONFLICT (item_id,page) DO NOTHING", itemID, page, correction.Revision, string(document))
	} else {
		result, err = s.DB.ExecContext(ctx, "UPDATE panel_correction SET revision = ?, document = ? WHERE item_id = ? AND page = ? AND revision = ?", correction.Revision, string(document), itemID, page, expected)
	}
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrPanelConflict
	}
	return &correction, nil
}
