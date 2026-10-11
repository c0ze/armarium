package store

import (
	"context"
	"testing"
)

func TestMeta(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if v, err := s.Meta(ctx, "page_order"); err != nil || v != "" {
		t.Fatalf("missing key: %q %v", v, err)
	}
	if err := s.SetMeta(ctx, "page_order", "2"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta(ctx, "page_order", "3"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Meta(ctx, "page_order"); v != "3" {
		t.Fatalf("round trip: %q", v)
	}
}
