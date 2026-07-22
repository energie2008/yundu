package service

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// SS2022CipherConfig 定义 SS2022 各加密算法的密钥长度
// 对齐 XBoard 的 CIPHER_CONFIGURATIONS 常量
var ss2022CipherConfig = map[string]struct {
	ServerKeySize int
	UserKeySize   int
}{
	"2022-blake3-aes-128-gcm":       {ServerKeySize: 16, UserKeySize: 16},
	"2022-blake3-aes-256-gcm":       {ServerKeySize: 32, UserKeySize: 32},
	"2022-blake3-chacha20-poly1305": {ServerKeySize: 32, UserKeySize: 32},
}

// generateSS2022Password 派生 SS2022 密码（对齐 XBoard 算法）
// 格式: {serverKey}:{userKey}
//   serverKey = base64(substr(sha256(nodeCreatedAt), 0, serverKeySize))
//   userKey   = base64(substr(sha256(userUUID), 0, userKeySize))
//
// 使用 sha256（32字节）确保足够长度，md5 只有 16 字节无法满足 256 位算法要求
func generateSS2022Password(userUUID string, nodeCreatedAt string, cipher string) string {
	config, ok := ss2022CipherConfig[cipher]
	if !ok {
		return userUUID
	}

	// serverKey = base64(substr(sha256(nodeCreatedAt), 0, serverKeySize))
	shaSum := sha256.Sum256([]byte(nodeCreatedAt))
	serverKey := base64.StdEncoding.EncodeToString(shaSum[:config.ServerKeySize])

	// userKey = base64(substr(sha256(userUUID), 0, userKeySize))
	userSum := sha256.Sum256([]byte(userUUID))
	userKey := base64.StdEncoding.EncodeToString(userSum[:config.UserKeySize])

	return serverKey + ":" + userKey
}

// isSS2022Cipher 判断是否是 SS2022 加密算法
func isSS2022Cipher(method string) bool {
	return strings.HasPrefix(method, "2022-")
}
