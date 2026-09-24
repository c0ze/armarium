package store

import (
	"database/sql/driver"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"modernc.org/sqlite"
)

// fold makes search forgiving: SQLite's LIKE folds ASCII case only, so
// "osmanli" missed "Osmanlı" and "miserables" missed "misérables". Both sides
// of a match go through fold(): lower case, accents stripped, dotless ı as i.
func fold(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii { // most titles: skip normalisation, it runs once per row per search
		return strings.ToLower(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == 'ı':
			b.WriteRune('i')
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("fold", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			switch v := args[0].(type) {
			case string:
				return fold(v), nil
			case []byte:
				return fold(string(v)), nil
			}
			return args[0], nil
		})
}
