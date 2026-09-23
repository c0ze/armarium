package scan

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	bracketed = regexp.MustCompile(`\([^)]*\)|\[[^\]]*\]|\{[^}]*\}`)
	hashNum   = regexp.MustCompile(`#\s*(\d+(?:\.\d+)?)`)
	markedNum = regexp.MustCompile(`(?i)\b(?:vol(?:ume)?|v|ch(?:apter)?|issue|no|book|part)\.?\s*(\d+(?:\.\d+)?)\b`)
	anyNum    = regexp.MustCompile(`\b(\d+(?:\.\d+)?)\b`)
	spaces    = regexp.MustCompile(`\s+`)
)

// titleFromName turns "Saga_v01_(2012)_(Digital).cbz" into "Saga v01 (2012) (Digital)".
func titleFromName(name string) string {
	base := strings.TrimSuffix(name, path.Ext(name))
	base = strings.ReplaceAll(base, "_", " ")
	return strings.TrimSpace(spaces.ReplaceAllString(base, " "))
}

// numberFromName finds the issue/volume number in a comic file name. Bracketed
// parts are ignored so a year like "(2012)" is never taken for the number.
func numberFromName(name string) *float64 {
	s := bracketed.ReplaceAllString(titleFromName(name), " ")
	for _, re := range []*regexp.Regexp{hashNum, markedNum} {
		if m := re.FindStringSubmatch(s); m != nil {
			return parseNum(m[1])
		}
	}
	all := anyNum.FindAllStringSubmatch(s, -1)
	if len(all) == 0 {
		return nil
	}
	return parseNum(all[len(all)-1][1])
}

func parseNum(s string) *float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// sourceKey derives the source of a book from its top-level folder, the way the
// Skrivist Books reader did: "oreilly_downloads" → "oreilly".
func sourceKey(top string) string { return strings.TrimSuffix(top, "_downloads") }

// Labels maps a source key or folder name to a display label.
type Labels map[string]string

// LoadLabels reads {"key": "Label"} or the reader's humble_meta.json shape
// {"key": {"label": "..", "year": 2015}}.
func LoadLabels(p string) (Labels, error) {
	out := Labels{}
	if p == "" {
		return out, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("labels %s: %w", p, err)
	}
	for k, v := range raw {
		var s string
		if json.Unmarshal(v, &s) == nil {
			out[k] = s
			continue
		}
		var o struct {
			Label string `json:"label"`
			Year  int    `json:"year"`
		}
		if err := json.Unmarshal(v, &o); err != nil || o.Label == "" {
			continue
		}
		out[k] = o.Label
		if y := strconv.Itoa(o.Year); o.Year > 0 && !strings.Contains(o.Label, y) {
			out[k] = o.Label + " (" + y + ")"
		}
	}
	return out, nil
}

// Label returns the display name for a source key, falling back to a tidied key:
// "horror-bookbundle" → "Horror".
func (l Labels) Label(key string) string {
	if v, ok := l[key]; ok {
		return v
	}
	for _, suffix := range []string{"-bookbundle", "-bundle"} {
		if v, ok := l[key+suffix]; ok {
			return v
		}
		key = strings.TrimSuffix(key, suffix)
	}
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(key))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
