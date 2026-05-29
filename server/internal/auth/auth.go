package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

var ErrWeakPassword = errors.New("password must be at least 12 characters")

func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// VerifyPasswordOrDummy verifies password against hash. When hash is empty
// (caller hit a non-existent account) it still performs a bcrypt comparison
// against a fixed dummy hash so the response time matches the populated path,
// closing a username-enumeration timing side-channel.
func VerifyPasswordOrDummy(hash, password string) bool {
	dummyHashOnce.Do(func() {
		h, err := bcrypt.GenerateFromPassword([]byte("veritra-timing-dummy"), bcrypt.DefaultCost)
		if err == nil {
			dummyHash = h
		}
	})
	if hash == "" {
		if len(dummyHash) > 0 {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		}
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func NewToken() (plain string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	return plain, HashToken(plain), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
