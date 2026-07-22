package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/middleware"
	"github.com/gin-gonic/gin"
)

func getRequestIDFromContext(ctx context.Context) string {
	if v := ctx.Value(middleware.RequestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func NewSingleHostReverseProxy(targetURL string, logger *slog.Logger) (gin.HandlerFunc, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL %q: %w", targetURL, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
	}

	proxy.Transport = &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		// 拨号控制：避免后端 hang 住时网关被拖死
		// 注意：DialContext 超时必须 < 网关 TimeoutByPath 全局 30s（默认），
		// 否则拨号阶段就把 30s 用完，handler 没机会响应。
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		// TLS 握手超时（虽然内部调用是 http，仍保留以备 https upstream）
		TLSHandshakeTimeout:   5 * time.Second,
		// 等待后端响应头的最长时间：10s 足够 identity/node/subscription/traffic 任一服务返回首字节
		// 超过则认为是后端死锁，触发 ErrorHandler 返回 503
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, proxyErr error) {
		rid := getRequestIDFromContext(r.Context())
		logger.Error("proxy request failed",
			"request_id", rid,
			"target", target.String(),
			"method", r.Method,
			"path", r.URL.Path,
			"error", proxyErr,
		)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"code":%d,"message":"service unavailable","request_id":%q}`,
			config.CodeServiceUnavailable, rid)
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Methods")
		resp.Header.Del("Access-Control-Allow-Headers")
		return nil
	}

	return func(c *gin.Context) {
		start := time.Now()
		rid := middleware.GetRequestID(c)

		logger.Info("proxying request",
			"request_id", rid,
			"target", target.String(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"client_ip", c.ClientIP(),
		)

		proxy.ServeHTTP(c.Writer, c.Request)

		latency := time.Since(start)
		status := c.Writer.Status()
		logger.Info("proxied request completed",
			"request_id", rid,
			"target", target.String(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"latency_ms", float64(latency.Microseconds()) / 1000.0,
		)
	}, nil
}
