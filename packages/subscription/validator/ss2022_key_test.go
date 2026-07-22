package validator

import (
	"testing"

	"github.com/airport-panel/subscription/nodespec"
)

// TestValidateSS2022Key 验证 SS2022 key 长度校验
// 覆盖：正确长度、错误长度、非 base64、非 SS2022 跳过、map credentials
func TestValidateSS2022Key(t *testing.T) {
	// 生成正确的测试 key（base64 编码）
	// 32 字节 → 2022-blake3-aes-256-gcm
	validKey32 := "JU+LXjyDXXw9Hqx8gLf7ASOrNkjy+o+WaZubeofku/I=" // openssl rand -base64 32
	// 16 字节 → 2022-blake3-aes-128-gcm
	validKey16 := "TkUPRpSqGoDDt4LHrEAkaA==" // openssl rand -base64 16

	tests := []struct {
		name     string
		protocol nodespec.Protocol
		creds    interface{}
		wantErr  bool
		errField string
	}{
		// 正确用例
		{
			name:     "valid_256gcm_32bytes",
			protocol: nodespec.ProtocolShadowsocks,
			creds: nodespec.ShadowsocksCredentials{
				Method:   "2022-blake3-aes-256-gcm",
				Password: validKey32,
			},
			wantErr: false,
		},
		{
			name:     "valid_128gcm_16bytes",
			protocol: nodespec.ProtocolShadowsocks,
			creds: nodespec.ShadowsocksCredentials{
				Method:   "2022-blake3-aes-128-gcm",
				Password: validKey16,
			},
			wantErr: false,
		},
		{
			name:     "valid_chacha20_32bytes",
			protocol: nodespec.ProtocolShadowsocks,
			creds: nodespec.ShadowsocksCredentials{
				Method:   "2022-blake3-chacha20-poly1305",
				Password: validKey32,
			},
			wantErr: false,
		},
		// 错误用例：长度不匹配（P15 实际故障：256-gcm 配 16 字节 key）
		{
			name:     "wrong_256gcm_16bytes",
			protocol: nodespec.ProtocolShadowsocks,
			creds: nodespec.ShadowsocksCredentials{
				Method:   "2022-blake3-aes-256-gcm",
				Password: validKey16, // 16 字节，应该 32
			},
			wantErr:  true,
			errField: "credentials.password",
		},
		// 错误用例：非 base64 编码（hk01 实际故障：普通字符串密码）
		{
			name:     "not_base64_string",
			protocol: nodespec.ProtocolShadowsocks,
			creds: nodespec.ShadowsocksCredentials{
				Method:   "2022-blake3-aes-128-gcm",
				Password: "ss2022pass-hk01-000000007", // 普通 string，非 base64
			},
			wantErr:  true,
			errField: "credentials.password",
		},
		// 错误用例：未知 cipher
		{
			name:     "unknown_cipher",
			protocol: nodespec.ProtocolShadowsocks,
			creds: nodespec.ShadowsocksCredentials{
				Method:   "2022-blake3-unknown-cipher",
				Password: validKey32,
			},
			wantErr:  true,
			errField: "credentials.method",
		},
		// 跳过用例：非 SS2022（传统 SS，不校验 key 格式）
		{
			name:     "skip_legacy_ss",
			protocol: nodespec.ProtocolShadowsocks,
			creds: nodespec.ShadowsocksCredentials{
				Method:   "aes-256-gcm",
				Password: "any-password-string",
			},
			wantErr: false,
		},
		// 跳过用例：非 shadowsocks 协议
		{
			name:     "skip_vless",
			protocol: nodespec.ProtocolVLESS,
			creds: nodespec.VLESSCredentials{
				UUID: "00000000-0000-0000-0000-000000000001",
			},
			wantErr: false,
		},
		// 边界用例：map credentials（数据库 JSON 反序列化场景）
		{
			name:     "map_creds_valid",
			protocol: nodespec.ProtocolShadowsocks,
			creds: map[string]interface{}{
				"method":   "2022-blake3-aes-256-gcm",
				"password": validKey32,
			},
			wantErr: false,
		},
		{
			name:     "map_creds_wrong_length",
			protocol: nodespec.ProtocolShadowsocks,
			creds: map[string]interface{}{
				"method":   "2022-blake3-aes-256-gcm",
				"password": validKey16, // 16 字节，应该 32
			},
			wantErr:  true,
			errField: "credentials.password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &nodespec.NodeSpec{
				Protocol:    tt.protocol,
				Credentials: tt.creds,
			}
			errs := validateSS2022Key(spec, nil)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Errorf("validateSS2022Key() hasErr = %v, wantErr = %v, errs = %+v", hasErr, tt.wantErr, errs)
			}
			if tt.wantErr && tt.errField != "" {
				if errs[0].Field != tt.errField {
					t.Errorf("expected error field %q, got %q", tt.errField, errs[0].Field)
				}
			}
		})
	}
}
