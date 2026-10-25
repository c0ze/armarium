package formats

import "testing"

type closer struct{ closed *int }

func (c closer) Close() error { *c.closed++; return nil }

func TestCacheClearClosesEverythingOnceReleased(t *testing.T) {
	c := NewCache[closer](4)
	var a, b int
	_, relA, _ := c.Get(1, func() (closer, error) { return closer{&a}, nil })
	relA()
	_, relB, _ := c.Get(2, func() (closer, error) { return closer{&b}, nil })
	c.Clear()
	if a != 1 {
		t.Fatalf("an idle archive was not closed: %d", a)
	}
	if b != 0 {
		t.Fatal("an archive in use was closed underneath its reader")
	}
	relB()
	if b != 1 {
		t.Fatalf("an archive in use was not closed when released: %d", b)
	}
}
