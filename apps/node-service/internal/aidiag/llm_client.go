package aidiag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// 错误定义
// ============================================================================

var (
	ErrNotFound        = errors.New("diagnosis session not found")
	ErrNoLLMClient     = errors.New("no LLM client configured")
	ErrNoLogCollector  = errors.New("no log collector configured")
	ErrSessionNotDone  = errors.New("diagnosis session not done yet")
	ErrInvalidSuggestion = errors.New("invalid suggestion index")
)

// ============================================================================
// GLM Client 实现（智谱 GLM-4 / GLM-4.5 / GLM-Flash）
// 智谱 API 兼容 OpenAI Chat Completions 格式
// 文档: https://open.bigmodel.cn/dev/api/llm/glm-4
// ============================================================================

type GLMConfig struct {
	APIKey       string // 从环境变量 LLM_*_API_KEY 读取
	BaseURL      string // GLM 默认 https://open.bigmodel.cn/api/paas/v4；DeepSeek 默认 https://api.deepseek.com
	Model        string // GLM 默认 glm-4-flash；DeepSeek 默认 deepseek-chat
	MaxTokens    int
	Temperature  float32
	ProviderName string // LLM 提供方标识：glm / deepseek
}

func GLMConfigFromEnv() GLMConfig {
	cfg := GLMConfig{
		APIKey:       os.Getenv("LLM_GLM_API_KEY"),
		BaseURL:      os.Getenv("LLM_GLM_BASE_URL"),
		Model:        os.Getenv("LLM_GLM_MODEL"),
		ProviderName: "glm",
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
	}
	if cfg.Model == "" {
		cfg.Model = "glm-4-flash"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 2048
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.2
	}
	return cfg
}

type GLMClient struct {
	cfg        GLMConfig
	httpClient *http.Client
}

// 单次请求超时：覆盖 DeepSeek 生成 2048 tokens 的常见耗时（含 R1 推理路径）。
// 注意：调用方传入的 ctx 可能受网关 120s 超时约束，所以这里再叠一层 110s
// 防止超过网关上限导致 504（保留 10s 余量给持久化 / 日志写入）。
const llmCallTimeout = 110 * time.Second

// 瞬时错误最大重试次数（不含首次调用）
const llmMaxRetries = 1

// 重试退避：1s + jitter，避免雪崩
const llmRetryBaseBackoff = 1 * time.Second

func NewGLMClient(cfg GLMConfig) *GLMClient {
	return &GLMClient{
		cfg: cfg,
		// 不在 http.Client 上设 Timeout，改用每请求 ctx 控制，便于重试时复用连接池
		httpClient: &http.Client{
			Timeout: 0,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *GLMClient) Provider() string {
	if c.cfg.ProviderName != "" {
		return c.cfg.ProviderName
	}
	return "glm"
}

func (c *GLMClient) Chat(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	if c.cfg.APIKey == "" {
		return nil, fmt.Errorf("%s API key not configured (set LLM_DEEPSEEK_API_KEY or LLM_GLM_API_KEY env var)", c.Provider())
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.cfg.MaxTokens
	}
	temp := req.Temperature
	if temp == 0 {
		temp = c.cfg.Temperature
	}

	body := map[string]interface{}{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.UserPrompt},
		},
		"max_tokens":   maxTokens,
		"temperature":  temp,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := c.cfg.BaseURL + "/chat/completions"

	var lastErr error
	for attempt := 0; attempt <= llmMaxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避 + 抖动：1s * 2^(n-1) + rand(0, 200ms)
			backoff := llmRetryBaseBackoff * (1 << (attempt - 1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		// 每次调用单独设置 110s 超时（即使外层 ctx 还活着，也避免单次卡死太久）
		callCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
		resp, err := c.doCall(callCtx, url, bodyBytes)
		cancel()

		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isTransient(err) {
			// 4xx / 解析错误 / 无可选响应等不可重试错误，立即返回
			return nil, err
		}
	}
	return nil, fmt.Errorf("%s call failed after %d attempts: %w", c.Provider(), llmMaxRetries+1, lastErr)
}

// doCall 单次 HTTP 调用
func (c *GLMClient) doCall(ctx context.Context, url string, bodyBytes []byte) (*LLMResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 500 {
		// 5xx 服务端错误，可重试
		return nil, &transientError{msg: fmt.Sprintf("%s API error %d: %s", c.Provider(), resp.StatusCode, string(respBytes))}
	}
	if resp.StatusCode >= 400 {
		// 4xx 客户端错误（鉴权失败 / 限流等），不重试
		return nil, fmt.Errorf("%s API error %d: %s", c.Provider(), resp.StatusCode, string(respBytes))
	}

	var result struct {
		Choices []struct {
			Message      struct{ Content string } `json:"message"`
			FinishReason string                  `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("decode %s response: %w, body: %s", c.Provider(), err, string(respBytes))
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("%s returned no choices", c.Provider())
	}
	return &LLMResponse{
		Content:      result.Choices[0].Message.Content,
		FinishReason: result.Choices[0].FinishReason,
		TokensUsed:   result.Usage.TotalTokens,
	}, nil
}

// transientError 标记可重试的瞬时错误
type transientError struct{ msg string }

func (e *transientError) Error() string { return e.msg }

// isTransient 判断错误是否值得重试
// - 网络错误 / 超时：可重试（连接重置 / 拨号失败 / ctx deadline）
// - 5xx：可重试（服务端临时故障）
// - 4xx / 解析错误：不重试
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	var te *transientError
	if errors.As(err, &te) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// http.Client 返回的 url.Error 在网络层错误时包装底层 error
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// ctx deadline 或连接重置类才重试；其他 url.Error（如 invalid URL）不重试
		if errors.Is(urlErr.Err, context.DeadlineExceeded) || errors.Is(urlErr.Err, io.EOF) {
			return true
		}
	}
	return false
}

// ============================================================================
// DeepSeek 配置（OpenAI 兼容 API）
// 文档: https://api-docs.deepseek.com/
// 模型: deepseek-chat (V3 通用) / deepseek-reasoner (R1 推理)
// ============================================================================

func DeepSeekConfigFromEnv() GLMConfig {
	cfg := GLMConfig{
		APIKey:       os.Getenv("LLM_DEEPSEEK_API_KEY"),
		BaseURL:      os.Getenv("LLM_DEEPSEEK_BASE_URL"),
		Model:        os.Getenv("LLM_DEEPSEEK_MODEL"),
		ProviderName: "deepseek",
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 2048
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.2
	}
	return cfg
}

// LLMFromEnv 根据环境变量自动选择 LLM 提供方
// 优先级：DeepSeek > GLM > nil（无 LLM，诊断会话将返回原始日志摘要）
func LLMFromEnv() LLMClient {
	if key := os.Getenv("LLM_DEEPSEEK_API_KEY"); key != "" {
		return NewGLMClient(DeepSeekConfigFromEnv())
	}
	if key := os.Getenv("LLM_GLM_API_KEY"); key != "" {
		return NewGLMClient(GLMConfigFromEnv())
	}
	return nil
}

// ============================================================================
// Stub LogCollector（未接入 node-agent 日志拉取时的占位实现）
// ============================================================================

type StubLogCollector struct{}

func NewStubLogCollector() *StubLogCollector { return &StubLogCollector{} }

func (s *StubLogCollector) CollectLogs(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID, start, end time.Time) (string, error) {
	return "[stub] log collection not implemented yet. Configure real LogCollector to fetch from node-agent via gRPC stream.", nil
}

func (s *StubLogCollector) CollectMetrics(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID, start, end time.Time) (map[string]interface{}, error) {
	return map[string]interface{}{
		"note":         "stub metrics collector",
		"server_id":    serverID.String(),
		"time_window":  fmt.Sprintf("%s ~ %s", start.Format(time.RFC3339), end.Format(time.RFC3339)),
	}, nil
}
