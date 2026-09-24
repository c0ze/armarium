package store

import (
	"context"
	"testing"
)

func TestSearchFoldsCaseAndAccents(t *testing.T) {
	s := openTest(t)
	seed(t, s, "Osmanlı İmparatorluğu.cbz", "Les misérables.cbz", "Çalıkuşu.cbz")
	ctx := context.Background()
	for q, want := range map[string]int{
		"osmanli": 1, "OSMANLI": 1, "imparatorlugu": 1, "İMPARATORLUĞU": 1,
		"miserables": 1, "MISÉRABLES": 1, "calikusu": 1, "Çalıkuşu": 1, "zzz": 0,
	} {
		if _, total, err := s.Items(ctx, ItemFilter{Query: q, Limit: 10}); err != nil || total != want {
			t.Errorf("%q: got %d (%v), want %d", q, total, err, want)
		}
	}
}
