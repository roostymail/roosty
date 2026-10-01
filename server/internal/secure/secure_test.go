package secure

import "testing"

func TestSealOpen(t *testing.T) {
	key := RandomBytes(32)
	ct, _ := Seal(key, []byte("senha"))
	if pt, err := Open(key, ct); err != nil || string(pt) != "senha" {
		t.Fatal("round trip failed")
	}
	if _, err := Open(RandomBytes(32), ct); err == nil {
		t.Fatal("wrong key must fail")
	}
}

func TestPassword(t *testing.T) {
	h := HashPassword("correct horse")
	if !CheckPassword(h, "correct horse") || CheckPassword(h, "wrong") {
		t.Fatal("password check failed")
	}
}

func TestSign(t *testing.T) {
	k := []byte("k")
	if !Verify(k, "msg", Sign(k, "msg")) || Verify(k, "msg2", Sign(k, "msg")) {
		t.Fatal("signature check failed")
	}
}
