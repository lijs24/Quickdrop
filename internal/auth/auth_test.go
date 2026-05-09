package auth

import "testing"

func TestHashAndVerifyToken(t *testing.T) {
	hash := HashToken("secret")
	if hash == "" {
		t.Fatal("expected token hash")
	}
	if !VerifyToken("secret", hash) {
		t.Fatal("expected token to verify")
	}
	if VerifyToken("wrong", hash) {
		t.Fatal("wrong token verified")
	}
}
