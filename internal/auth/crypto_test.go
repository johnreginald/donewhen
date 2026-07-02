package auth

import "testing"

func TestPasswordHashVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("valid password did not verify")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("wrong password verified")
	}
	if VerifyPassword("not-a-hash", "x") {
		t.Fatal("malformed hash verified")
	}
}

func TestHashPasswordUniqueSalt(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("identical passwords produced identical hashes (salt not random)")
	}
}

func TestHashTokenStable(t *testing.T) {
	first := HashToken("kanri_abc")
	second := HashToken("kanri_abc")
	if first != second {
		t.Fatal("token hash not stable")
	}
	if HashToken("a") == HashToken("b") {
		t.Fatal("different tokens hashed equal")
	}
}

func TestRandomTokenLength(t *testing.T) {
	tok, err := RandomToken(32)
	if err != nil {
		t.Fatalf("random: %v", err)
	}
	if len(tok) < 40 { // 32 bytes base64url ~ 43 chars
		t.Fatalf("token too short: %d", len(tok))
	}
	other, _ := RandomToken(32)
	if tok == other {
		t.Fatal("RandomToken not random")
	}
}
