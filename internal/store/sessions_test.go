package store

import (
	"context"
	"testing"
	"time"
)

func TestSessionsEndWhenThePasswordChanges(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	s.AddSession(ctx, "h1", time.Hour)
	// The first run after an upgrade only records the password: nobody is logged out.
	if changed, err := s.ResetSessionsIfPasswordChanged(ctx, "hashA"); err != nil || changed {
		t.Fatalf("first run: %v %v", changed, err)
	}
	if ok, _ := s.SessionValid(ctx, "h1"); !ok {
		t.Fatal("the first run must keep sessions")
	}
	if changed, _ := s.ResetSessionsIfPasswordChanged(ctx, "hashB"); !changed {
		t.Fatal("a new password must report a change")
	}
	if ok, _ := s.SessionValid(ctx, "h1"); ok {
		t.Fatal("a session survived the password change")
	}
	s.AddSession(ctx, "h2", time.Hour)
	if changed, _ := s.ResetSessionsIfPasswordChanged(ctx, "hashB"); changed {
		t.Fatal("the same password is not a change")
	}
	if ok, _ := s.SessionValid(ctx, "h2"); !ok {
		t.Fatal("a session was lost without a change")
	}
}
