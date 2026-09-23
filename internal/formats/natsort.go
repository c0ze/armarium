package formats

import "strings"

// NaturalLess orders strings with embedded numbers numerically, case-insensitively:
// "page2" < "page10", "Vol 9" < "vol 10".
func NaturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			na, ra := splitDigits(a)
			nb, rb := splitDigits(b)
			ta, tb := strings.TrimLeft(na, "0"), strings.TrimLeft(nb, "0")
			if len(ta) != len(tb) {
				return len(ta) < len(tb)
			}
			if ta != tb {
				return ta < tb
			}
			if len(na) != len(nb) { // "01" before "1" keeps the order total
				return len(na) > len(nb)
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func splitDigits(s string) (string, string) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

// SortKey is a lowercase key that sorts like NaturalLess when compared bytewise in
// SQL: every digit run is left-padded to 10 digits.
func SortKey(s string) string {
	s = strings.ToLower(s)
	var sb strings.Builder
	for s != "" {
		if isDigit(s[0]) {
			d, rest := splitDigits(s)
			d = strings.TrimLeft(d, "0")
			if len(d) < 10 {
				sb.WriteString(strings.Repeat("0", 10-len(d)))
			}
			sb.WriteString(d)
			s = rest
			continue
		}
		sb.WriteByte(s[0])
		s = s[1:]
	}
	return sb.String()
}
