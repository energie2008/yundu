// Package crypto 提供 node-service 的密码学工具。
//
// P1-9: REALITY 密钥自动生成，替代固定密钥对。
// 使用 X25519 (curve25519) 椭圆曲线，与 Xray REALITY 协议兼容。
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// GenerateREALITYKeypair 生成 X25519 密钥对，用于 Xray REALITY 协议。
// 返回 base64url 编码（无 padding）的私钥和公钥，格式与 Xray 配置一致。
//
// REALITY 密钥用途：
//   - 私钥 (private_key): 服务端持有，用于完成 X25519 密钥交换
//   - 公钥 (public_key): 客户端持有，用于验证服务端身份
//
// 安全性：
//   - 私钥使用 crypto/rand 生成 32 字节随机数
//   - 按 RFC 7748 §5 进行 clamping
//   - 每个节点应使用独立密钥对，避免共享
func GenerateREALITYKeypair() (privateKey, publicKey string, err error) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return "", "", fmt.Errorf("generate private key: %w", err)
	}

	// X25519 私钥 clamping（RFC 7748 §5）
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("derive public key: %w", err)
	}

	// base64url 编码（无 padding），与 Xray REALITY 配置格式一致
	encoding := base64.RawURLEncoding
	return encoding.EncodeToString(priv), encoding.EncodeToString(pub), nil
}
