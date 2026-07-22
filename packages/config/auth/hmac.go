package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

const (
	HeaderTimestamp = "X-Timestamp"
	HeaderNonce     = "X-Nonce"
	HeaderSignature = "X-Signature"
	MaxClockSkew    = 300 * time.Second
)

func Sign(method, path, body string, timestamp int64, nonce, secret string) string {
	payload := fmt.Sprintf("%s\n%s\n%s\n%d\n%s", method, path, body, timestamp, nonce)
	return HMACSHA256(payload, secret)
}

func HMACSHA256(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

type SignatureError struct {
	Code    string
	Message string
}

func (e *SignatureError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

var (
	ErrMissingHeader   = &SignatureError{Code: "MISSING_HEADER", Message: "missing required signature headers"}
	ErrInvalidTimestamp = &SignatureError{Code: "INVALID_TIMESTAMP", Message: "timestamp must be a unix second integer"}
	ErrClockSkew       = &SignatureError{Code: "CLOCK_SKEW", Message: "timestamp deviation exceeds 5 minutes"}
	ErrInvalidSignature = &SignatureError{Code: "INVALID_SIGNATURE", Message: "signature verification failed"}
)

func VerifySignature(method, path, body, timestampStr, nonce, signature, secret string) error {
	if timestampStr == "" || nonce == "" || signature == "" {
		return ErrMissingHeader
	}

	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return ErrInvalidTimestamp
	}

	now := time.Now().Unix()
	if ts < now-int64(MaxClockSkew.Seconds()) || ts > now+int64(MaxClockSkew.Seconds()) {
		return ErrClockSkew
	}

	expected := Sign(method, path, body, ts, nonce, secret)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return ErrInvalidSignature
	}

	return nil
}
