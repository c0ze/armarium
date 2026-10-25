package store

import (
	"context"
	"testing"
	"time"
)

func TestOwnerPasswordRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if h, err := s.OwnerPassword(ctx); err != nil || h != "" {
		t.Fatalf("unset: %q %v", h, err)
	}
	if err := s.SetOwnerPassword(ctx, "$argon2id$stored"); err != nil {
		t.Fatal(err)
	}
	if h, _ := s.OwnerPassword(ctx); h != "$argon2id$stored" {
		t.Fatalf("read back %q", h)
	}
}

func TestSetOwnerPasswordEndsSessions(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	s.AddSession(ctx, "h1", time.Hour)
	s.AddSession(ctx, "h2", time.Hour)
	if err := s.SetOwnerPassword(ctx, "$argon2id$new"); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"h1", "h2"} {
		if ok, _ := s.SessionValid(ctx, h); ok {
			t.Fatalf("session %s survived", h)
		}
	}
	// The start-up check sees the stored hash as already recorded: no second reset.
	s.AddSession(ctx, "h3", time.Hour)
	if changed, _ := s.ResetSessionsIfPasswordChanged(ctx, "$argon2id$new"); changed {
		t.Fatal("start-up reset after SetOwnerPassword")
	}
}
