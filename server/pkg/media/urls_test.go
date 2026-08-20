package media

import (
	"testing"
	"time"
)

func TestSignedURLIssuer_IssueAndVerify(t *testing.T) {
	issuer := NewSignedURLIssuer([]byte("test-key"), 5*time.Minute)
	roundID := "round-123"
	assetRef := "abc123.webp"

	token, expires, err := issuer.Issue(roundID, assetRef, time.Now())
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if expires.IsZero() {
		t.Fatal("expected expiry")
	}

	gotRef, err := issuer.Verify(roundID, token, time.Now())
	if err != nil {
		t.Fatalf("verify valid token: %v", err)
	}
	if gotRef != assetRef {
		t.Fatalf("expected assetRef %q, got %q", assetRef, gotRef)
	}
}

func TestSignedURLIssuer_ExpiredTokenRejected(t *testing.T) {
	issuer := NewSignedURLIssuer([]byte("test-key"), 1*time.Second)
	now := time.Now()
	token, _, _ := issuer.Issue("round-1", "asset.webp", now)

	_, err := issuer.Verify("round-1", token, now.Add(2*time.Second))
	if err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestSignedURLIssuer_WrongRoundRejected(t *testing.T) {
	issuer := NewSignedURLIssuer([]byte("test-key"), 5*time.Minute)
	token, _, _ := issuer.Issue("round-1", "asset.webp", time.Now())

	_, err := issuer.Verify("round-2", token, time.Now())
	if err == nil {
		t.Fatal("expected cross-round token to be rejected")
	}
}

func TestSignedURLIssuer_TamperedTokenRejected(t *testing.T) {
	issuer := NewSignedURLIssuer([]byte("test-key"), 5*time.Minute)
	token, _, _ := issuer.Issue("round-1", "asset.webp", time.Now())

	_, err := issuer.Verify("round-1", token+"x", time.Now())
	if err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}

func TestSignedURLIssuer_MissingInputs(t *testing.T) {
	issuer := NewSignedURLIssuer([]byte("test-key"), 5*time.Minute)
	if _, _, err := issuer.Issue("", "asset.webp", time.Now()); err == nil {
		t.Fatal("expected roundID required")
	}
	if _, _, err := issuer.Issue("round-1", "", time.Now()); err == nil {
		t.Fatal("expected assetRef required")
	}
}
