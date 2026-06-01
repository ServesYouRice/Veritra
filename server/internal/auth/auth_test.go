package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordRejectsBcryptTruncationRange(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrWeakPassword {
		t.Fatalf("short password err=%v want %v", err, ErrWeakPassword)
	}
	if _, err := HashPassword(strings.Repeat("a", 72)); err != nil {
		t.Fatalf("72-byte password should be accepted: %v", err)
	}
	if _, err := HashPassword(strings.Repeat("a", 73)); err != ErrPasswordTooLong {
		t.Fatalf("73-byte password err=%v want %v", err, ErrPasswordTooLong)
	}
}

func TestVerifyPasswordOrDummyAlwaysRejectsMissingHash(t *testing.T) {
	if VerifyPasswordOrDummy("", "owner-password-123") {
		t.Fatal("missing hash should not authenticate")
	}
}
