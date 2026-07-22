package config

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// init loads .env file if present. Existing environment variables take
// precedence (godotenv does not override already-set vars), so explicit
// env settings win over .env values.
//
// 因为各服务以子目录（apps/<service>）为 cwd 启动，而 .env 在仓库根，
// 所以需要向上逐级查找直到找到 .env 或到达文件系统根。
// 注意：godotenv.Load() 遇到第一个不存在的文件就返回错误并停止，
// 所以这里手动遍历候选路径，只对实际存在的文件调用 Load。
func init() {
	for _, p := range envCandidatePaths() {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			return
		}
	}
}

// envCandidatePaths 返回从当前工作目录向上逐级查找的 .env 候选路径列表（最多 8 级）。
func envCandidatePaths() []string {
	cwd, err := os.Getwd()
	if err != nil {
		return []string{".env"}
	}
	var candidates []string
	dir := cwd
	for i := 0; i < 8; i++ {
		candidates = append(candidates, filepath.Join(dir, ".env"))
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return candidates
}

type DatabaseConfig struct {
	DSN      string
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type NATSConfig struct {
	URL string
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
}

type JWTConfig struct {
	Secret           string
	AccessTTLSeconds int
	RefreshTTLSeconds int
}

type Config struct {
	AppEnv     string
	LogLevel   string
	Database   DatabaseConfig
	Redis      RedisConfig
	NATS       NATSConfig
	MinIO      MinIOConfig
	JWT        JWTConfig
	ServicePorts map[string]string
}

func Load() *Config {
	return &Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		LogLevel: getEnv("APP_LOG_LEVEL", "debug"),
		Database: DatabaseConfig{
			DSN:      getEnv("POSTGRES_DSN", "postgres://app:app@localhost:5432/airport?sslmode=disable"),
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnvInt("POSTGRES_PORT", 5432),
			User:     getEnv("POSTGRES_USER", "app"),
			Password: getEnv("POSTGRES_PASSWORD", "app"),
			DBName:   getEnv("POSTGRES_DB", "airport"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		NATS: NATSConfig{
			URL: getEnv("NATS_URL", "nats://localhost:4222"),
		},
		MinIO: MinIOConfig{
			Endpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
			UseSSL:    getEnv("MINIO_USE_SSL", "false") == "true",
			Bucket:    getEnv("MINIO_BUCKET", "airport"),
		},
		JWT: JWTConfig{
			Secret:            getEnv("JWT_SECRET", ""),
			AccessTTLSeconds:  getEnvInt("JWT_ACCESS_TTL_SECONDS", 900),
			RefreshTTLSeconds: getEnvInt("JWT_REFRESH_TTL_SECONDS", 604800),
		},
		ServicePorts: map[string]string{
			"api-gateway":        getEnv("API_GATEWAY_PORT", "8080"),
			"identity-service":   getEnv("IDENTITY_SERVICE_PORT", "8081"),
			"node-service":       getEnv("NODE_SERVICE_PORT", "8082"),
			"subscription-service": getEnv("SUBSCRIPTION_SERVICE_PORT", "8083"),
			"traffic-service":    getEnv("TRAFFIC_SERVICE_PORT", "8084"),
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
