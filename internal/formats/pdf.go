package formats

import (
	"os"
	"regexp"
	"strconv"
)

// pdfWindow is how much of each end of a PDF we read. The page tree root sits near
// one end in nearly every file (start when linearized, end after an update), and
// reading 2 MiB instead of whole files keeps scans light on the NAS.
const pdfWindow = 1 << 20

var pagesCount = regexp.MustCompile(`/Type\s*/Pages\b[^>]*?/Count\s+(\d+)|/Count\s+(\d+)[^>]*?/Type\s*/Pages\b`)

// PDFPageCount is a light heuristic: the largest /Count of a /Pages dictionary in
// the first and last MiB. It returns 0 when the tree lives in a compressed object
// stream; the browser's PDF viewer does not need the count.
func PDFPageCount(p string) int {
	f, err := os.Open(p)
	if err != nil {
		return 0
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0
	}
	best := 0
	scan := func(off int64) {
		buf := make([]byte, pdfWindow)
		n, _ := f.ReadAt(buf, off)
		for _, m := range pagesCount.FindAllSubmatch(buf[:n], -1) {
			v := m[1]
			if v == nil {
				v = m[2]
			}
			if c, err := strconv.Atoi(string(v)); err == nil && c > best && c < 1_000_000 {
				best = c
			}
		}
	}
	scan(0)
	if st.Size() > pdfWindow {
		scan(max(st.Size()-pdfWindow, pdfWindow))
	}
	return best
}
