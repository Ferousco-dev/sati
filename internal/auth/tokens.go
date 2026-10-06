package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

type hasher struct{ key []byte }

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (h hasher) sum(token string) []byte {
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(token))
	return mac.Sum(nil)
}
