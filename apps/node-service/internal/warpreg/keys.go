package warpreg

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// GenerateKeyPair 生成 WireGuard 密钥对（curve25519）
// 返回 base64 编码的 private_key 和 public_key
func GenerateKeyPair() (privateKey, publicKey string, err error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", fmt.Errorf("generate private key: %w", err)
	}
	// Clamp private key per WireGuard spec
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)

	privateKey = base64.StdEncoding.EncodeToString(priv[:])
	publicKey = base64.StdEncoding.EncodeToString(pub[:])
	return
}

// PublicKeyFromPrivate 从私钥推导公钥
func PublicKeyFromPrivate(privateKey string) (string, error) {
	priv, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return "", fmt.Errorf("decode private key: %w", err)
	}
	if len(priv) != 32 {
		return "", fmt.Errorf("invalid private key length: %d", len(priv))
	}
	var privArr, pubArr [32]byte
	copy(privArr[:], priv)
	curve25519.ScalarBaseMult(&pubArr, &privArr)
	return base64.StdEncoding.EncodeToString(pubArr[:]), nil
}
