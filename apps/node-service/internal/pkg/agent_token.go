package pkg

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	ErrInvalidAgentToken = errors.New("invalid agent token")
)

func GenerateAgentToken(serverCode, salt string) string {
	mac := hmac.New(sha256.New, []byte(salt))
	mac.Write([]byte(serverCode))
	return hex.EncodeToString(mac.Sum(nil))
}

func ValidateAgentToken(serverCode, token, salt string) bool {
	expected := GenerateAgentToken(serverCode, salt)
	return hmac.Equal([]byte(token), []byte(expected))
}

func HmacSHA256(msg, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(msg))
	return hex.EncodeToString(h.Sum(nil))
}

func ComputeSignature(machineID, nonce string, timestamp int64, token string) string {
	payload := fmt.Sprintf("%s%s%d", machineID, nonce, timestamp)
	return HmacSHA256(payload, token)
}

func ValidateSignature(machineID, nonce string, timestamp int64, token, signature string) bool {
	expected := ComputeSignature(machineID, nonce, timestamp, token)
	return hmac.Equal([]byte(expected), []byte(signature))
}
