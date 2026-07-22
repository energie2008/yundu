package auth

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestSignAndVerify(t *testing.T) {
	secret := "test-secret-key-12345"
	method := "POST"
	path := "/api/v1/agent/heartbeat"
	body := `{"node_id":"abc123"}`
	ts := time.Now().Unix()
	nonce := "random-nonce-xyz"

	sig := Sign(method, path, body, ts, nonce, secret)
	if sig == "" {
		t.Fatal("signature is empty")
	}

	err := VerifySignature(method, path, body, strconv.FormatInt(ts, 10), nonce, sig, secret)
	if err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	err = VerifySignature(method, path, body, strconv.FormatInt(ts, 10), nonce, sig+"tampered", secret)
	if err != ErrInvalidSignature {
		t.Error("tampered signature should fail")
	}
}

func TestVerifyExpiredTimestamp(t *testing.T) {
	secret := "test-secret"
	err := VerifySignature("GET", "/test", "", strconv.FormatInt(time.Now().Unix()-600, 10), "n1", "sig", secret)
	if err != ErrClockSkew {
		t.Errorf("expected ErrClockSkew, got %v", err)
	}

	err = VerifySignature("GET", "/test", "", strconv.FormatInt(time.Now().Unix()+600, 10), "n1", "sig", secret)
	if err != ErrClockSkew {
		t.Errorf("expected ErrClockSkew for future, got %v", err)
	}
}

func TestVerifyInvalidTimestamp(t *testing.T) {
	secret := "test-secret"
	err := VerifySignature("GET", "/test", "", "not-a-number", "n1", "sig", secret)
	if err != ErrInvalidTimestamp {
		t.Errorf("expected ErrInvalidTimestamp, got %v", err)
	}
}

func TestVerifyInvalidSignature(t *testing.T) {
	secret := "test-secret"
	ts := time.Now().Unix()
	err := VerifySignature("GET", "/test", "", strconv.FormatInt(ts, 10), "n1", "wrongsig", secret)
	if err != ErrInvalidSignature {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerifyMissingHeaders(t *testing.T) {
	secret := "test-secret"
	err := VerifySignature("GET", "/test", "", "", "n1", "sig", secret)
	if err != ErrMissingHeader {
		t.Errorf("expected ErrMissingHeader for empty timestamp, got %v", err)
	}

	err = VerifySignature("GET", "/test", "", strconv.FormatInt(time.Now().Unix(), 10), "", "sig", secret)
	if err != ErrMissingHeader {
		t.Errorf("expected ErrMissingHeader for empty nonce, got %v", err)
	}

	err = VerifySignature("GET", "/test", "", strconv.FormatInt(time.Now().Unix(), 10), "n1", "", secret)
	if err != ErrMissingHeader {
		t.Errorf("expected ErrMissingHeader for empty signature, got %v", err)
	}
}

func TestVerifyWrongMethod(t *testing.T) {
	secret := "test-secret"
	ts := time.Now().Unix()
	sig := Sign("POST", "/test", "body", ts, "n1", secret)
	err := VerifySignature("GET", "/test", "body", strconv.FormatInt(ts, 10), "n1", sig, secret)
	if err != ErrInvalidSignature {
		t.Errorf("expected ErrInvalidSignature for wrong method, got %v", err)
	}
}

func TestNonceStoreMemory(t *testing.T) {
	store := NewMemoryNonceStore()
	ctx := context.Background()

	ok, err := store.CheckAndStore(ctx, "nonce-1", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("first check should pass: ok=%v err=%v", ok, err)
	}

	ok, err = store.CheckAndStore(ctx, "nonce-1", 10*time.Second)
	if err != nil || ok {
		t.Fatalf("duplicate nonce should fail: ok=%v err=%v", ok, err)
	}

	ok, err = store.CheckAndStore(ctx, "nonce-2", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("new nonce should pass: ok=%v err=%v", ok, err)
	}
}

func TestHMACDeterministic(t *testing.T) {
	sig1 := HMACSHA256("test", "secret")
	sig2 := HMACSHA256("test", "secret")
	if sig1 != sig2 {
		t.Error("HMAC should be deterministic")
	}
	sig3 := HMACSHA256("test", "secret2")
	if sig1 == sig3 {
		t.Error("different secrets should produce different signatures")
	}
}

func TestSignatureFormat(t *testing.T) {
	sig := Sign("GET", "/api/test", "", time.Now().Unix(), "abc", "mykey")
	if len(sig) != 64 {
		t.Errorf("expected 64 char hex signature, got %d chars: %s", len(sig), sig)
	}
	for _, c := range sig {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("signature contains non-hex character: %c", c)
		}
	}
}

func i64s(i int64) string {
	return fmt.Sprintf("%d", i)
}

var _ = i64s
