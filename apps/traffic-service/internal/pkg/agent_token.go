package pkg

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
