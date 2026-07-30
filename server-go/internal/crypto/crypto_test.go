package crypto

import "testing"

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := HashPassword("123123")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("123123", h) {
		t.Fatal("round-trip verify failed")
	}
	if VerifyPassword("wrong", h) {
		t.Fatal("wrong password verified")
	}
}
