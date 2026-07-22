// genkey 生成生产级安全密钥。
//
// 用法:
//
//	go run ./cmd/genkey            # 生成所有密钥
//	go run ./cmd/genkey jwt        # 仅生成 JWT_SECRET
//	go run ./cmd/genkey hmac       # 仅生成 HMAC_SECRET
//
// 输出可直接复制到 .env 文件。
package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

func main() {
	keys := []struct {
		env   string
		bytes int
	}{
		{"JWT_SECRET", 32},
		{"ARGON2_SALT", 16},
		{"AGENT_API_TOKEN_SALT", 32},
		{"HMAC_SECRET", 32},
	}

	if len(os.Args) > 1 {
		// 单独生成某个密钥
		target := os.Args[1]
		for _, k := range keys {
			if k.env == target || "jwt" == target && k.env == "JWT_SECRET" ||
				"hmac" == target && k.env == "HMAC_SECRET" ||
				"argon" == target && k.env == "ARGON2_SALT" ||
				"agent" == target && k.env == "AGENT_API_TOKEN_SALT" {
				val, err := genHex(k.bytes)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("%s=%s\n", k.env, val)
				return
			}
		}
		fmt.Fprintf(os.Stderr, "unknown key: %s (valid: jwt, argon, agent, hmac)\n", target)
		os.Exit(1)
	}

	// 生成所有密钥
	fmt.Println("# Generated secrets - copy to .env")
	fmt.Println("# DO NOT commit .env to version control")
	fmt.Println()
	for _, k := range keys {
		val, err := genHex(k.bytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s: %v\n", k.env, err)
			os.Exit(1)
		}
		fmt.Printf("%s=%s\n", k.env, val)
	}
}

func genHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
