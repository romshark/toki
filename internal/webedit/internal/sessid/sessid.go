package sessid

import (
	"crypto/rand"
	"encoding/base64"
)

// MustNewID is similar to NewID but panics in the unlikely event of
// a crypto/rand.Read failure.
func MustNewID() string {
	id, err := NewID()
	if err != nil {
		panic(err)
	}
	return id
}

// NewID returns a cryptographically secure, URL-safe session ID.
// 32 bytes = 256 bits of entropy.
func NewID() (string, error) {
	const size = 32

	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// RawURLEncoding avoids padding and is cookie / URL safe.
	return base64.RawURLEncoding.EncodeToString(b), nil
}
