package cert

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// ACMECredentials 承载 ACME DNS provider 的凭证
// 序列化为 JSON 后整体用 AES-GCM 加密落库
type ACMECredentials struct {
	Provider string            `json:"provider"` // "cloudflare" / "alidns" / ...
	Vars     map[string]string `json:"vars"`     // {"api_token": "...", "access_key_id": "...", ...}
}

var (
	cryptoKeyOnce sync.Once
	cryptoKey     []byte

	// ErrCryptoKeyNotSet 表示未配置 CERT_CREDENTIALS_KEY 环境变量
	ErrCryptoKeyNotSet = errors.New("CERT_CREDENTIALS_KEY not set; ACME credentials encryption unavailable")
)

// loadCryptoKey 懒加载加密密钥（32 字节，从环境变量 CERT_CREDENTIALS_KEY 读取 hex 编码）
// 若未配置则退化为从 JWT_SECRET 派生（取前 32 字节的 SHA-256）
func loadCryptoKey() ([]byte, error) {
	var err error
	cryptoKeyOnce.Do(func() {
		hexKey := os.Getenv("CERT_CREDENTIALS_KEY")
		if hexKey != "" {
			k, e := hex.DecodeString(hexKey)
			if e != nil || len(k) != 32 {
				err = fmt.Errorf("CERT_CREDENTIALS_KEY must be 32 bytes hex-encoded (64 chars): %w", e)
				return
			}
			cryptoKey = k
			return
		}
		// 退化：从 JWT_SECRET 派生（兼容未单独配置 CERT_CREDENTIALS_KEY 的部署）
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			err = ErrCryptoKeyNotSet
			return
		}
		derived := deriveKeyFromSecret(jwtSecret)
		cryptoKey = derived
	})
	return cryptoKey, err
}

// EncryptCredentials 将 ACMECredentials 序列化为 JSON 并用 AES-GCM 加密
// 返回 hex 编码的密文，可直接存入 DB 的 acme_credentials_encrypted 列
func EncryptCredentials(c ACMECredentials) (string, error) {
	plaintext, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal credentials: %w", err)
	}
	key, err := loadCryptoKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	// nonce 拼在密文前面（解密时切片分离）
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return hex.EncodeToString(ciphertext), nil
}

// DecryptCredentials 解密 hex 编码的密文并反序列化为 ACMECredentials
func DecryptCredentials(encrypted string) (*ACMECredentials, error) {
	if encrypted == "" {
		return nil, errors.New("empty encrypted credentials")
	}
	ciphertext, err := hex.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}
	key, err := loadCryptoKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	ct := ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w (key may have changed)", err)
	}
	var creds ACMECredentials
	if err := json.Unmarshal(plaintext, &creds); err != nil {
		return nil, fmt.Errorf("unmarshal credentials: %w", err)
	}
	return &creds, nil
}

// deriveKeyFromSecret 从任意长度 secret 派生 32 字节 AES 密钥
// 使用 SHA-256(secret) 作为密钥（非密钥派生函数，但满足 ACME 凭证加密场景）
func deriveKeyFromSecret(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}
