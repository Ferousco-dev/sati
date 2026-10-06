package auth

import "testing"

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct-horse", hash) {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword("wrong-horse", hash) {
		t.Fatal("wrong password accepted")
	}
	if VerifyPassword("correct-horse", "garbage") {
		t.Fatal("malformed hash accepted")
	}

	again, _ := HashPassword("correct-horse")
	if hash == again {
		t.Fatal("hashes must be salted")
	}
}
