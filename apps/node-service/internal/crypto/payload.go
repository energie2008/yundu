package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// PayloadManifest 是控制面编译产出的不可变配置包。
// Content 字段存储加密后的 PayloadContent：当 PayloadEncrypted=true 时，
// Content 是一个 JSON 字符串字面量（base64 编码的 nonce||ciphertext），
// 便于直接序列化为合法 JSON 并写入 PostgreSQL JSONB 列。
type PayloadManifest struct {
	VersionNo        int64           `json:"version_no"`
	DeploymentID     string          `json:"deployment_id,omitempty"`
	SHA256           string          `json:"sha256"`
	Timestamp        int64           `json:"timestamp"`
	Kernel           string          `json:"kernel"`            // xray / sing-box
	RollbackStrategy string          `json:"rollback_strategy"` // lkg / none
	PayloadEncrypted bool            `json:"payload_encrypted"`
	Content          json.RawMessage `json:"content"` // 加密后的 content（base64 字符串）或明文 content
}

// PayloadContent 是解密后的实际配置内容
type PayloadContent struct {
	ConfigJSON   map[string]interface{}  `json:"config_json"`
	TLSMaterials map[string]*TLSMaterial `json:"tls_materials,omitempty"` // domain -> PEM bundle
}

// TLSMaterial 是内联的 TLS 证书材料
type TLSMaterial struct {
	CertPEM string `json:"cert_pem"`
	KeyPEM  string `json:"key_pem"`
}

// EncryptPayload 使用 AES-GCM 加密 PayloadContent。
// 输出格式：nonce || ciphertext（原始字节，非 base64）。
func EncryptPayload(content *PayloadContent, key []byte) ([]byte, error) {
	plaintext, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("marshal payload content: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// 输出格式: nonce || ciphertext
	ciphertext := aesgcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// DecryptPayload 使用 AES-GCM 解密 PayloadContent。
// 入参 encrypted 为 EncryptPayload 的原始输出（nonce || ciphertext）。
func DecryptPayload(encrypted []byte, key []byte) (*PayloadContent, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := aesgcm.NonceSize()
	if len(encrypted) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := encrypted[:nonceSize], encrypted[nonceSize:]
	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	var content PayloadContent
	if err := json.Unmarshal(plaintext, &content); err != nil {
		return nil, fmt.Errorf("unmarshal payload content: %w", err)
	}

	return &content, nil
}

// BuildManifest 构建一个完整的 Payload Manifest。
// Content 字段会被设置为 base64 编码的加密数据（JSON 字符串字面量），
// 以确保 PayloadManifest 可被 json.Marshal 序列化为合法 JSON，
// 并可写入 PostgreSQL JSONB 列。
func BuildManifest(versionNo int64, deploymentID string, kernel string, content *PayloadContent, key []byte) (*PayloadManifest, error) {
	// SHA256 仅基于 ConfigJSON 计算，与 Agent 端验签口径一致：
	// Agent 解密后只对 config_json 做 sha256 比对（main.go 的 signature_verify），
	// 而 TLSMaterials（证书）由 AES-GCM 解密保证完整性（篡改会导致解密失败），
	// 因此不纳入签名哈希，避免"含证书哈希 vs 仅配置哈希"导致的 signature_verify 永久 nack。
	configJSONBytes, err := json.Marshal(content.ConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("marshal config json for hash: %w", err)
	}

	hash := sha256.Sum256(configJSONBytes)
	hashStr := hex.EncodeToString(hash[:])

	// 加密 content，得到 nonce || ciphertext
	encrypted, err := EncryptPayload(content, key)
	if err != nil {
		return nil, fmt.Errorf("encrypt payload: %w", err)
	}

	// 将加密字节 base64 编码后包装为 JSON 字符串字面量，
	// 使 Content 成为合法的 json.RawMessage（可直接 Marshal / 存 JSONB）。
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	contentJSON := json.RawMessage(`"` + encoded + `"`)

	return &PayloadManifest{
		VersionNo:        versionNo,
		DeploymentID:     deploymentID,
		SHA256:           hashStr,
		Timestamp:        time.Now().Unix(),
		Kernel:           kernel,
		RollbackStrategy: "lkg",
		PayloadEncrypted: true,
		Content:          contentJSON,
	}, nil
}

// DecryptManifest 从 PayloadManifest.Content 中解密出 PayloadContent。
// 内部完成 base64 解码（Content 是 JSON 字符串字面量）后调用 DecryptPayload。
func DecryptManifest(m *PayloadManifest, key []byte) (*PayloadContent, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest is nil")
	}
	if !m.PayloadEncrypted {
		// 明文模式：Content 直接是 PayloadContent 的 JSON
		var content PayloadContent
		if err := json.Unmarshal(m.Content, &content); err != nil {
			return nil, fmt.Errorf("unmarshal plaintext content: %w", err)
		}
		return &content, nil
	}

	// 加密模式：Content 是 base64 字符串字面量
	var encoded string
	if err := json.Unmarshal(m.Content, &encoded); err != nil {
		return nil, fmt.Errorf("decode content base64 string: %w", err)
	}
	encrypted, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	return DecryptPayload(encrypted, key)
}

// DeriveKey 从环境变量密钥字符串派生 32 字节 AES 密钥。
// SHA-256 输出正好 32 字节，适用于 AES-256。
func DeriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}
