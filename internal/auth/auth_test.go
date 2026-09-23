package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := CheckPassword(h, "correct horse"); !ok || err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	if ok, _ := CheckPassword(h, "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	for _, bad := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA", "$argon2id$v=19$m=99999999,t=1,p=1$c2FsdA$aGFzaA"} {
		if _, err := CheckPassword(bad, "x"); err == nil {
			t.Errorf("%q: expected ErrBadHash", bad)
		}
	}
}

func TestSecretsAreUniqueAndDigestStable(t *testing.T) {
	a, b := NewSecret(), NewSecret()
	if a == b || len(a) < 40 {
		t.Fatal("weak secret")
	}
	if Digest(a) != Digest(a) || Digest(a) == Digest(b) {
		t.Fatal("digest")
	}
}
